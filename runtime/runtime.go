//go:build !wasm

package runtime

import (
	"context"
	"encoding/json"
	"fmt"
	"html"
	"io"
	"net/http"
	"os"
	"os/signal"
	"runtime"
	"runtime/pprof"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	// YAML Parser
	"gopkg.in/yaml.v3"
)

// Global State
var (
	serverPort string

	// Config Map
	configMap   = make(map[string]string)
	configMapMu sync.RWMutex

	// TLS
	tlsCertFile string
	tlsKeyFile  string
	tlsEnabled  bool

	servgateURL string
)

// getCliFlag parses a --flag value from os.Args.
// Returns empty string if not found.
func getCliFlag(name string) string {
	prefix := "--" + name + "="
	flagWithSpace := "--" + name
	for i, arg := range os.Args {
		// --port=9090
		if strings.HasPrefix(arg, prefix) {
			return strings.TrimPrefix(arg, prefix)
		}
		// --port 9090
		if arg == flagWithSpace && i+1 < len(os.Args) {
			return os.Args[i+1]
		}
	}
	return ""
}

func init() {
	metricsGauges.m = make(map[string]float64)
	loadYAMLConfig()

	// Parse customizable Pub/Sub options
	if sizeStr := Config("pubsub.queue_size"); sizeStr != "" {
		if val, err := strconv.Atoi(sizeStr); err == nil && val > 0 {
			pubSubQueueSize = val
		}
	}
	if workersStr := Config("pubsub.workers"); workersStr != "" {
		if val, err := strconv.Atoi(workersStr); err == nil && val > 0 {
			pubSubWorkers = val
		}
	}
	pubSubQueue = make(chan pubSubEvent, pubSubQueueSize)

	// Parse customizable statement cache size
	if valStr := Config("database.stmt_cache_max"); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil && val > 0 {
			stmtCacheMax = val
		}
	}
}

func loadYAMLConfig() {
	// Look for custom config path in:
	// 1. CLI flag: --config <path>
	// 2. Env variable: SERV_CONFIG
	// 3. Profile-based fallback: config.<env>.yml or config.<env>.yaml
	// 4. Default fallback: config.yml or config.yaml
	var configPath string

	for i := 0; i < len(os.Args)-1; i++ {
		if os.Args[i] == "--config" {
			configPath = os.Args[i+1]
			break
		}
	}

	if configPath == "" {
		configPath = os.Getenv("SERV_CONFIG")
	}

	if configPath == "" {
		// Determine the active environment profile
		profile := getCliFlag("env")
		if profile == "" {
			profile = os.Getenv("SERV_ENV")
		}

		if profile != "" {
			ymlPath := fmt.Sprintf("config.%s.yml", profile)
			yamlPath := fmt.Sprintf("config.%s.yaml", profile)
			if _, err := os.Stat(ymlPath); err == nil {
				configPath = ymlPath
			} else if _, err := os.Stat(yamlPath); err == nil {
				configPath = yamlPath
			}
		}
	}

	if configPath == "" {
		if _, err := os.Stat("config.yml"); err == nil {
			configPath = "config.yml"
		} else if _, err := os.Stat("config.yaml"); err == nil {
			configPath = "config.yaml"
		}
	}

	if configPath == "" {
		return // No config file found, fallback to env vars only
	}

	data, err := os.ReadFile(configPath)
	if err != nil {
		LogWarn("Failed to read config file at ", configPath, ": ", err.Error())
		return
	}

	var parsed map[string]interface{}
	err = yaml.Unmarshal(data, &parsed)
	if err != nil {
		LogWarn("Failed to parse YAML config file at ", configPath, ": ", err.Error())
		return
	}

	configMapMu.Lock()
	defer configMapMu.Unlock()
	flattenMap("", parsed)
	LogInfo("Successfully loaded YAML configuration from: ", configPath)
}

func flattenMap(prefix string, val interface{}) {
	switch v := val.(type) {
	case map[string]interface{}:
		for k, child := range v {
			newPrefix := k
			if prefix != "" {
				newPrefix = prefix + "." + k
			}
			flattenMap(newPrefix, child)
		}
	case map[interface{}]interface{}:
		for k, child := range v {
			newPrefix := fmt.Sprint(k)
			if prefix != "" {
				newPrefix = prefix + "." + newPrefix
			}
			flattenMap(newPrefix, child)
		}
	case []interface{}:
		for i, child := range v {
			newPrefix := fmt.Sprintf("%s.[%d]", prefix, i)
			flattenMap(newPrefix, child)
		}
	default:
		configMap[prefix] = fmt.Sprint(v)
	}
}

type Request struct {
	Method  string            `json:"method"`
	Path    string            `json:"path"`
	Body    string            `json:"body"`
	Params  map[string]string `json:"params"`
	Headers map[string]string `json:"headers"`
	Query   map[string]string `json:"query"`
}

type HTTPResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// Config Helper
func Env(key string) string {
	return os.Getenv(key)
}

func EnvSecret(key string) string {
	val := os.Getenv(key)
	RegisterSecret(val)
	return val
}

func Config(key string) string {
	configMapMu.RLock()
	val, exists := configMap[key]
	configMapMu.RUnlock()

	if exists {
		return val
	}
	return os.Getenv(key)
}

func resolveServgateAddr(port string) (string, string) {
	if !strings.HasPrefix(port, "servgate://") {
		return port, ""
	}
	urlStr := strings.TrimPrefix(port, "servgate://")
	localPort := "8085"
	if idx := strings.Index(urlStr, "?"); idx != -1 {
		params := urlStr[idx+1:]
		urlStr = urlStr[:idx]
		for _, p := range strings.Split(params, "&") {
			kv := strings.Split(p, "=")
			if len(kv) == 2 && kv[0] == "port" {
				localPort = kv[1]
			}
		}
	}
	if !strings.HasPrefix(urlStr, "http://") && !strings.HasPrefix(urlStr, "https://") {
		urlStr = "http://" + urlStr
	}
	
	if envPort := os.Getenv("PORT"); envPort != "" {
		localPort = envPort
	}
	return localPort, urlStr
}

// REST HTTP Server
func InitServer(port string) {
	p, gate := resolveServgateAddr(port)
	serverPort = p
	servgateURL = gate
}

func InitServerTLS(port, certFile, keyFile string) {
	p, gate := resolveServgateAddr(port)
	serverPort = p
	servgateURL = gate
	tlsCertFile = certFile
	tlsKeyFile = keyFile
	tlsEnabled = true
}

func StartServer() interface{} {
	for _, arg := range os.Args {
		if arg == "--mcp" {
			startMCPServer()
			return nil
		}
	}

	if os.Getenv("SERV_PROFILE") == "true" {
		cpuFile, err := os.Create("cpu.pprof")
		if err == nil {
			pprof.StartCPUProfile(cpuFile)
			LogInfo("CPU Profiling started. Output: cpu.pprof")
		}

		c := make(chan os.Signal, 1)
		signal.Notify(c, os.Interrupt, syscall.SIGTERM)
		go func() {
			<-c
			pprof.StopCPUProfile()
			if cpuFile != nil {
				cpuFile.Close()
			}
			memFile, err := os.Create("mem.pprof")
			if err == nil {
				pprof.WriteHeapProfile(memFile)
				memFile.Close()
				LogInfo("Memory profile written. Output: mem.pprof")
			}
			os.Exit(0)
		}()
	}

	RunMigrations()
	initOtel()

	// Port resolution priority: --port flag > PORT env > config("server.port") > source declaration
	if cliPort := getCliFlag("port"); cliPort != "" {
		serverPort = cliPort
	} else if envPort := os.Getenv("PORT"); envPort != "" {
		serverPort = envPort
	} else if cfgPort := Config("server.port"); cfgPort != "" {
		serverPort = cfgPort
	}

	if serverPort == "" {
		serverPort = "2112"
		LogInfo("No server port specified, starting metrics server on default port 2112")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", handleMetrics)
	mux.HandleFunc("/health", handleHealth)
	mux.HandleFunc("/ready", handleReady)

	// WebSocket endpoints
	wsHandlersMu.RLock()
	for wsPath, wsHandler := range wsHandlers {
		handler := wsHandler // capture for closure
		mux.HandleFunc(wsPath, func(w http.ResponseWriter, r *http.Request) {
			conn, err := upgrader.Upgrade(w, r, nil)
			if err != nil {
				LogError("WebSocket upgrade failed: ", err)
				return
			}
			wsConn := &WSConn{conn: conn}
			go handler(wsConn)
		})
	}
	wsHandlersMu.RUnlock()

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// 1. CORS check
		if corsEnabled {
			origin := r.Header.Get("Origin")
			allowed := false
			corsOriginsMu.RLock()
			for _, o := range corsOrigins {
				if o == "*" || o == origin {
					allowed = true
					break
				}
			}
			corsOriginsMu.RUnlock()

			if allowed {
				w.Header().Set("Access-Control-Allow-Origin", origin)
				w.Header().Set("Access-Control-Allow-Methods", "GET, POST, PUT, DELETE, PATCH, OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Request-ID")
				w.Header().Set("Access-Control-Max-Age", "86400")
			}

			if r.Method == "OPTIONS" {
				w.WriteHeader(http.StatusNoContent)
				return
			}
		}

		// 2. Global per-IP rate limiting
		if globalIPRateLimitEnabled && globalIPRateLimiter != nil {
			clientIP := r.Header.Get("X-Forwarded-For")
			if clientIP == "" {
				clientIP = r.Header.Get("X-Real-IP")
			}
			if clientIP == "" {
				if idx := strings.LastIndex(r.RemoteAddr, ":"); idx != -1 {
					clientIP = r.RemoteAddr[:idx]
				} else {
					clientIP = r.RemoteAddr
				}
			}
			lim := globalIPRateLimiter.getLimiter(clientIP)
			if lim != nil && !lim.allow() {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusTooManyRequests)
				json.NewEncoder(w).Encode(map[string]interface{}{
					"status": 429,
					"error":  "Too Many Requests",
				})
				return
			}
		}

		parentTrace := r.Header.Get("traceparent")
		var traceID string
		if parentTrace != "" {
			parts := strings.Split(parentTrace, "-")
			if len(parts) >= 2 {
				traceID = parts[1]
			}
		}

		handler, params, limiter, pattern := matchRoute(r.Method, r.URL.Path)

		if handler == nil {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":    "Not Found",
				"code":     "ERR_ROUTE_NOT_FOUND",
				"trace_id": traceID,
			})
			return
		}

		if limiter != nil && !limiter.allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"error":    "Too Many Requests",
				"code":     "ERR_RATE_LIMIT_EXCEEDED",
				"trace_id": traceID,
			})
			return
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		rawBody := string(bodyBytes)
		
		// 3. Input Sanitization
		sanitizedBody := sanitizeJSON(rawBody)
		
		sanitizedParams := make(map[string]string)
		for k, v := range params {
			sanitizedParams[k] = html.EscapeString(v)
		}

		req := Request{
			Method:  r.Method,
			Path:    r.URL.Path,
			Body:    sanitizedBody,
			Params:  sanitizedParams,
			Headers: make(map[string]string),
			Query:   make(map[string]string),
		}

		// Merge query parameters into params (path/query params take priority)
		for key, values := range r.URL.Query() {
			escapedVal := html.EscapeString(values[0])
			req.Query[key] = escapedVal
			if _, exists := req.Params[key]; !exists {
				req.Params[key] = escapedVal
			}
		}

		// Merge headers into params (lowercase, path/query params take priority)
		for key, values := range r.Header {
			escapedVal := html.EscapeString(values[0])
			req.Headers[key] = escapedVal
			lowerKey := strings.ToLower(key)
			req.Headers[lowerKey] = escapedVal
			if _, exists := req.Params[lowerKey]; !exists {
				req.Params[lowerKey] = escapedVal
			}
		}

		// OpenTelemetry: start request span
		trace := TraceRequest(r.Method, r.URL.Path, parentTrace)
		SetActiveTrace(trace)

		start := time.Now()
		MetricInc("http_server_requests_total", "method", r.Method, "route", pattern)

		res := handler(req)
		ClearActiveTrace()

		duration := time.Since(start).Seconds()
		MetricGauge("http_server_request_duration_seconds", duration, "method", r.Method, "route", pattern)

		statusCode := 200
		w.Header().Set("Content-Type", "application/json")
		// Propagate trace context in response
		if tp := Traceparent(trace); tp != "" {
			w.Header().Set("traceparent", tp)
		}

		if ch, ok := res.(chan interface{}); ok {
			flusher, isFlusher := w.(http.Flusher)
			w.Header().Set("Content-Type", "text/event-stream")
			w.Header().Set("Cache-Control", "no-cache")
			w.Header().Set("Connection", "keep-alive")
			w.Header().Set("X-Accel-Buffering", "no")
			w.WriteHeader(http.StatusOK)
			if isFlusher {
				flusher.Flush()
			}
			for item := range ch {
				var payload string
				if str, ok := item.(string); ok {
					payload = str
				} else {
					b, _ := json.Marshal(item)
					payload = string(b)
				}
				fmt.Fprintf(w, "data: %s\n\n", payload)
				if isFlusher {
					flusher.Flush()
				}
			}
			EndTrace(trace, statusCode)
			return
		}

		if resMap, ok := res.(map[string]interface{}); ok {
			if s, hasStatus := resMap["status"]; hasStatus {
				if code, ok := s.(int); ok && code >= 400 {
					statusCode = code
				}
			}
			json.NewEncoder(w).Encode(resMap)
		} else if resStr, ok := res.(string); ok {
			w.Write([]byte(resStr))
		} else {
			json.NewEncoder(w).Encode(res)
		}


		// OpenTelemetry: end request span
		EndTrace(trace, statusCode)
	})

	srv := &http.Server{
		Addr:    ":" + serverPort,
		Handler: mux,
	}

	// Graceful shutdown on SIGINT/SIGTERM
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-shutdownCh
		LogInfo("Shutdown signal received, draining connections...")

		shutdownCtx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()

		if err := srv.Shutdown(shutdownCtx); err != nil {
			LogError("HTTP server shutdown error: ", err)
		}

		// Stop cron scheduler
		if cronInstance != nil {
			cronInstance.Stop()
		}

		// Close database connections
		stmtCacheMu.Lock()
		for _, stmt := range stmtCache {
			stmt.Close()
		}
		stmtCacheMu.Unlock()
		if dbInstance != nil {
			dbInstance.Close()
		}
		if mongoClient != nil {
			mongoClient.Disconnect(context.Background())
		}

		// Close broker connections
		if natsClient != nil {
			natsClient.Close()
		}
		if mqttConn != nil {
			mqttConn.Disconnect(250)
		}
		if amqpChan != nil {
			amqpChan.Close()
		}
		if amqpConn != nil {
			amqpConn.Close()
		}
		kafkaWriterMu.Lock()
		for _, w := range kafkaWriterMap {
			w.Close()
		}
		kafkaWriterMu.Unlock()
		if stompConn != nil {
			stompConn.Disconnect()
		}

		// Close Redis
		if redisClient != nil {
			redisClient.Close()
		}

		// Kill Python workers
		shutdownPythonWorkers()

		LogInfo("Shutdown complete")
	}()

	LogInfo("Serv service listening on port ", serverPort)
	if servgateURL != "" {
		go announceRoutesToServGate()
	}

	if tlsEnabled {
		LogInfo("TLS enabled with cert=", tlsCertFile, " key=", tlsKeyFile)
		if err := srv.ListenAndServeTLS(tlsCertFile, tlsKeyFile); err != nil && err != http.ErrServerClosed {
			LogError("Web server TLS error: ", err)
		}
	} else {
		if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			LogError("Web server error: ", err)
		}
	}
	return nil
}

func announceRoutesToServGate() {
	time.Sleep(500 * time.Millisecond)
	
	routesMu.RLock()
	defer routesMu.RUnlock()
	
	authToken := os.Getenv("SERV_GATE_AUTH_TOKEN")
	if authToken == "" {
		authToken = "gateway-secret-token"
	}
	
	host := os.Getenv("SERV_HOST")
	if host == "" {
		host = "localhost"
	}
	port := serverPort
	if extPort := os.Getenv("SERV_EXTERNAL_PORT"); extPort != "" {
		port = extPort
	}
	targetBase := fmt.Sprintf("http://%s:%s", host, port)
	if tlsEnabled {
		targetBase = fmt.Sprintf("https://%s:%s", host, port)
	}
	
	client := &http.Client{Timeout: 3 * time.Second}
	
	for _, pathMap := range routes {
		for path := range pathMap {
			prefix := path
			if idx := strings.Index(prefix, ":"); idx != -1 {
				prefix = prefix[:idx]
			}
			prefix = "/" + strings.Trim(prefix, "/")
			
			payload := map[string]interface{}{
				"prefix": prefix,
				"target": targetBase,
			}
			
			bodyBytes, err := json.Marshal(payload)
			if err != nil {
				continue
			}
			
			req, err := http.NewRequest("POST", servgateURL+"/api/routes", strings.NewReader(string(bodyBytes)))
			if err != nil {
				continue
			}
			req.Header.Set("Content-Type", "application/json")
			if authToken != "" {
				req.Header.Set("Authorization", "Bearer "+authToken)
			}
			
			resp, err := client.Do(req)
			if err != nil {
				LogWarn("Failed to self-announce route ", prefix, " to ServGate: ", err.Error())
				continue
			}
			resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				LogInfo("Successfully self-announced route ", prefix, " to ServGate")
			} else {
				LogWarn("ServGate route announcement returned status ", resp.StatusCode)
			}
		}
	}
}

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	// Standard Go Runtime Metrics
	var m runtime.MemStats
	runtime.ReadMemStats(&m)
	fmt.Fprintf(w, "go_goroutines %d\n", runtime.NumGoroutine())
	fmt.Fprintf(w, "go_mem_sys_bytes %d\n", m.Sys)
	fmt.Fprintf(w, "go_mem_alloc_bytes %d\n", m.Alloc)
	fmt.Fprintf(w, "go_mem_heap_alloc_bytes %d\n", m.HeapAlloc)
	fmt.Fprintf(w, "go_uptime_seconds %f\n", time.Since(startTime).Seconds())

	metricsMu.RLock()
	for k, v := range metricsCounters {
		if idx := strings.Index(k, "{"); idx != -1 {
			fmt.Fprintf(w, "%s_total%s %d\n", k[:idx], k[idx:], v)
		} else {
			fmt.Fprintf(w, "%s_total %d\n", k, v)
		}
	}
	metricsMu.RUnlock()

	metricsGauges.RLock()
	for k, v := range metricsGauges.m {
		fmt.Fprintf(w, "%s %f\n", k, v)
	}
	metricsGauges.RUnlock()
}

func handleHealth(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	status := map[string]interface{}{
		"status": "healthy",
		"uptime": time.Since(startTime).String(),
	}
	json.NewEncoder(w).Encode(status)
}

func handleReady(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")

	// Check database connectivity
	dbReady := true
	if dbInstance != nil {
		if err := dbInstance.Ping(); err != nil {
			dbReady = false
		}
	}

	// Check MongoDB connectivity
	mongoReady := true
	if mongoClient != nil {
		if err := mongoClient.Ping(context.Background(), nil); err != nil {
			mongoReady = false
		}
	}

	ready := dbReady && mongoReady
	status := map[string]interface{}{
		"ready":    ready,
		"database": dbReady,
		"mongodb":  mongoReady,
	}

	if ready {
		w.WriteHeader(http.StatusOK)
	} else {
		w.WriteHeader(http.StatusServiceUnavailable)
	}
	json.NewEncoder(w).Encode(status)
}

var startTime = time.Now()

func sanitizeJSON(bodyStr string) string {
	if bodyStr == "" {
		return ""
	}
	var data interface{}
	if err := json.Unmarshal([]byte(bodyStr), &data); err != nil {
		// Not JSON or invalid, return html escaped raw string
		return html.EscapeString(bodyStr)
	}
	sanitized := sanitizeInterface(data)
	b, _ := json.Marshal(sanitized)
	return string(b)
}

func sanitizeInterface(val interface{}) interface{} {
	switch v := val.(type) {
	case string:
		return html.EscapeString(v)
	case map[string]interface{}:
		res := make(map[string]interface{})
		for k, child := range v {
			res[k] = sanitizeInterface(child)
		}
		return res
	case []interface{}:
		res := make([]interface{}, len(v))
		for i, child := range v {
			res[i] = sanitizeInterface(child)
		}
		return res
	default:
		return v
	}
}
