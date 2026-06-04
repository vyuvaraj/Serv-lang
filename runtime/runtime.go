package runtime

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	// SQLite Driver (CGO-free)
	_ "github.com/glebarez/go-sqlite"

	// PostgreSQL Driver
	_ "github.com/lib/pq"

	// Oracle Driver (Pure Go)
	_ "github.com/sijms/go-ora/v2"

	// MongoDB Driver
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"

	// YAML Parser
	"gopkg.in/yaml.v3"

	// Redis client
	"github.com/redis/go-redis/v9"

	// Broker drivers
	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-stomp/stomp/v3"
	"github.com/nats-io/nats.go"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/segmentio/kafka-go"
)

// Global State
var (
	brokerURL    string
	serverPort   string
	routes       = make(map[string]map[string]func(Request) interface{}) // method -> path -> handler
	routesMu     sync.RWMutex

	routeTrie    = make(map[string]*trieNode) // method -> root trie node
	routeTrieMu  sync.RWMutex

	// DB Instance
	dbInstance  *sql.DB
	stmtCache      = make(map[string]*sql.Stmt)
	stmtCacheKeys  []string // ordered keys for LRU eviction
	stmtCacheMax   = 256    // max cached prepared statements
	stmtCacheMu sync.RWMutex

	// MongoDB Instances
	mongoClient *mongo.Client
	mongoDB     *mongo.Database

	// Cache Instance
	redisClient *redis.Client
	ctx         = context.Background()
	localCache   = make(map[string]localCacheEntry)
	localCacheMu sync.RWMutex

	// Broker Connection Instances
	natsClient      *nats.Conn
	mqttConn        mqtt.Client
	amqpConn        *amqp.Connection
	amqpChan        *amqp.Channel
	kafkaBrokerAddr string
	kafkaWriterMap  = make(map[string]*kafka.Writer)
	kafkaWriterMu   sync.Mutex
	stompConn       *stomp.Conn

	// Fallback In-memory Broker
	subscribers   = make(map[string][]func(string))
	subscribersMu sync.RWMutex

	pubSubQueueSize  = 10000
	pubSubWorkers    = 20
	pubSubQueue      chan pubSubEvent
	pubSubWorkerOnce sync.Once

	// Config Map
	configMap   = make(map[string]string)
	configMapMu sync.RWMutex

	// Database query hooks
	beforeQueryHooks   []func(interface{}, interface{}) interface{}
	beforeQueryHooksMu sync.RWMutex
)

// Noop is a no-op sentinel used by generated test files to satisfy the runtime import.
func Noop() {}

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



type localCacheEntry struct {
	value      interface{}
	expiration time.Time
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
	// 3. Fall back: config.yml or config.yaml
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
	Method string            `json:"method"`
	Path   string            `json:"path"`
	Body   string            `json:"body"`
	Params map[string]string `json:"params"`
}

type HTTPResponse struct {
	Status int    `json:"status"`
	Body   string `json:"body"`
}

// Config Helper
func Env(key string) string {
	return os.Getenv(key)
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

// Message Broker Connections
func InitBroker(url string) {
	brokerURL = url
	LogInfo("Initializing broker: ", url)

	if strings.HasPrefix(url, "nats://") {
		var err error
		natsClient, err = nats.Connect(url)
		if err != nil {
			LogWarn("Failed to connect to NATS broker: ", err, " - Falling back to in-memory broker")
		} else {
			LogInfo("Connected to NATS broker successfully")
		}
	} else if strings.HasPrefix(url, "mqtt://") || strings.HasPrefix(url, "tcp://") {
		opts := mqtt.NewClientOptions().AddBroker(url)
		mqttConn = mqtt.NewClient(opts)
		if token := mqttConn.Connect(); token.Wait() && token.Error() != nil {
			LogWarn("Failed to connect to MQTT broker: ", token.Error(), " - Falling back to in-memory broker")
			mqttConn = nil
		} else {
			LogInfo("Connected to MQTT broker successfully")
		}
	} else if strings.HasPrefix(url, "amqp://") {
		var err error
		amqpConn, err = amqp.Dial(url)
		if err != nil {
			LogWarn("Failed to connect to AMQP/RabbitMQ: ", err, " - Falling back to in-memory broker")
		} else {
			amqpChan, err = amqpConn.Channel()
			if err != nil {
				LogWarn("Failed to open AMQP channel: ", err)
				amqpConn.Close()
				amqpConn = nil
			} else {
				LogInfo("Connected to AMQP/RabbitMQ broker successfully")
			}
		}
	} else if strings.HasPrefix(url, "kafka://") {
		kafkaBrokerAddr = strings.TrimPrefix(url, "kafka://")
		LogInfo("Targeting Kafka Broker Address: ", kafkaBrokerAddr)
	} else if strings.HasPrefix(url, "activemq://") || strings.HasPrefix(url, "stomp://") {
		addr := strings.TrimPrefix(strings.TrimPrefix(url, "activemq://"), "stomp://")
		var err error
		stompConn, err = stomp.Dial("tcp", addr)
		if err != nil {
			LogWarn("Failed to connect to ActiveMQ over STOMP: ", err, " - Falling back to in-memory broker")
		} else {
			LogInfo("Connected to ActiveMQ/STOMP successfully")
		}
	}
}

func Subscribe(topic string, callback func(string)) {
	LogInfo("Registering subscription for topic: ", topic)

	if natsClient != nil {
		_, err := natsClient.Subscribe(topic, func(m *nats.Msg) {
			callback(string(m.Data))
		})
		if err == nil {
			return
		}
	}

	if mqttConn != nil {
		token := mqttConn.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			callback(string(msg.Payload()))
		})
		if token.Wait() && token.Error() == nil {
			return
		}
	}

	if amqpChan != nil {
		_, err1 := amqpChan.QueueDeclare(topic, false, false, false, false, nil)
		msgs, err2 := amqpChan.Consume(topic, "", true, false, false, false, nil)
		if err1 == nil && err2 == nil {
			go func() {
				for d := range msgs {
					callback(string(d.Body))
				}
			}()
			return
		}
	}

	if kafkaBrokerAddr != "" {
		r := kafka.NewReader(kafka.ReaderConfig{
			Brokers:  []string{kafkaBrokerAddr},
			Topic:    topic,
			GroupID:  "serv-consumer-group",
			MinBytes: 10,
			MaxBytes: 10e6,
		})
		go func() {
			defer r.Close()
			for {
				m, err := r.ReadMessage(context.Background())
				if err != nil {
					break
				}
				callback(string(m.Value))
			}
		}()
		return
	}

	if stompConn != nil {
		sub, err := stompConn.Subscribe(topic, stomp.AckAuto)
		if err == nil {
			go func() {
				defer sub.Unsubscribe()
				for {
					msg := <-sub.C
					if msg.Err != nil {
						break
					}
					callback(string(msg.Body))
				}
			}()
			return
		}
	}

	// In-memory fallback Pub/Sub
	subscribersMu.Lock()
	subscribers[topic] = append(subscribers[topic], callback)
	subscribersMu.Unlock()
}

func Publish(topic string, msg interface{}) {
	endSpan := TracePubSub("Publish", topic)
	defer endSpan()

	MetricInc("broker_messages_published_total")
	var msgStr string
	if str, ok := msg.(string); ok {
		msgStr = str
	} else {
		b, _ := json.Marshal(msg)
		msgStr = string(b)
	}

	// 1. NATS Publish
	if natsClient != nil {
		if err := natsClient.Publish(topic, []byte(msgStr)); err == nil {
			return
		}
	}

	// 2. MQTT Publish
	if mqttConn != nil {
		token := mqttConn.Publish(topic, 0, false, msgStr)
		if token.Wait() && token.Error() == nil {
			return
		}
	}

	// 3. AMQP Publish
	if amqpChan != nil {
		_, err := amqpChan.QueueDeclare(topic, false, false, false, false, nil)
		if err == nil {
			amqpChan.PublishWithContext(context.Background(), "", topic, false, false, amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(msgStr),
			})
			return
		}
	}

	// 4. Kafka Publish
	if kafkaBrokerAddr != "" {
		kafkaWriterMu.Lock()
		w, exists := kafkaWriterMap[topic]
		if !exists {
			w = &kafka.Writer{
				Addr:     kafka.TCP(kafkaBrokerAddr),
				Topic:    topic,
				Balancer: &kafka.LeastBytes{},
			}
			kafkaWriterMap[topic] = w
		}
		kafkaWriterMu.Unlock()
		if err := w.WriteMessages(context.Background(), kafka.Message{Value: []byte(msgStr)}); err == nil {
			return
		}
	}

	// 5. ActiveMQ STOMP Publish
	if stompConn != nil {
		if err := stompConn.Send(topic, "text/plain", []byte(msgStr)); err == nil {
			return
		}
	}

	// 6. In-memory Fallback
	startPubSubWorkers()
	subscribersMu.RLock()
	subs := subscribers[topic]
	subscribersMu.RUnlock()

	for _, callback := range subs {
		select {
		case pubSubQueue <- pubSubEvent{callback: callback, payload: msgStr}:
		default:
			// If queue is completely full, spawn a temporary goroutine fallback to avoid dropping events
			go executeCallbackSafe(callback, msgStr)
		}
	}
}

// REST HTTP Server
func InitServer(port string) {
	serverPort = port
}

var (
	tlsCertFile string
	tlsKeyFile  string
	tlsEnabled  bool
)

func InitServerTLS(port, certFile, keyFile string) {
	serverPort = port
	tlsCertFile = certFile
	tlsKeyFile = keyFile
	tlsEnabled = true
}

type routeRateLimiter struct {
	limitRate   int
	limitPeriod time.Duration
	tokensMutex sync.Mutex
	tokens      float64
	lastRefill  time.Time
}

func newRouteRateLimiter(rate int, period string) *routeRateLimiter {
	var dur time.Duration
	switch period {
	case "s":
		dur = time.Second
	case "m":
		dur = time.Minute
	case "h":
		dur = time.Hour
	default:
		dur = time.Second
	}
	return &routeRateLimiter{
		limitRate:   rate,
		limitPeriod: dur,
		tokens:      float64(rate),
		lastRefill:  time.Now(),
	}
}

func (rl *routeRateLimiter) allow() bool {
	rl.tokensMutex.Lock()
	defer rl.tokensMutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	rl.lastRefill = now

	refillRate := float64(rl.limitRate) / float64(rl.limitPeriod)
	rl.tokens += float64(elapsed) * refillRate
	if rl.tokens > float64(rl.limitRate) {
		rl.tokens = float64(rl.limitRate)
	}

	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}
	return false
}

func AddRoute(method, path string, limitRate int, limitPeriod string, handler func(Request) interface{}) {
	routesMu.Lock()
	if _, ok := routes[method]; !ok {
		routes[method] = make(map[string]func(Request) interface{})
	}
	routes[method][path] = handler
	routesMu.Unlock()

	var limiter *routeRateLimiter
	if limitRate > 0 {
		limiter = newRouteRateLimiter(limitRate, limitPeriod)
	}

	insertRoute(method, path, limiter, handler)
	LogInfo("Registered route: ", method, " ", path)
}

// Middleware registry
var (
	middlewareRegistry   = make(map[string]func(Request) interface{})
	middlewareRegistryMu sync.RWMutex
)

// RegisterMiddleware registers a named middleware function.
func RegisterMiddleware(name string, handler func(Request) interface{}) {
	middlewareRegistryMu.Lock()
	defer middlewareRegistryMu.Unlock()
	middlewareRegistry[name] = handler
	LogInfo("Registered middleware: ", name)
}

// AddRouteWithMiddleware registers a route with a middleware chain.
// Middlewares are executed in order before the handler.
// If any middleware returns non-nil, that response is sent and the handler is skipped.
func AddRouteWithMiddleware(method, path string, limitRate int, limitPeriod string, middlewareNames []string, handler func(Request) interface{}) {
	wrappedHandler := func(req Request) interface{} {
		// Execute middleware chain
		middlewareRegistryMu.RLock()
		for _, name := range middlewareNames {
			mw, exists := middlewareRegistry[name]
			if !exists {
				LogWarn("Middleware not found: ", name)
				continue
			}
			result := mw(req)
			if result != nil {
				middlewareRegistryMu.RUnlock()
				return result // short-circuit: middleware returned a response
			}
		}
		middlewareRegistryMu.RUnlock()

		// All middlewares passed, execute handler
		return handler(req)
	}

	AddRoute(method, path, limitRate, limitPeriod, wrappedHandler)
}

type Migration struct {
	Name string
	Func func()
}

var (
	migrations   []Migration
	migrationsMu sync.Mutex
)

func RegisterMigration(name string, f func()) {
	migrationsMu.Lock()
	defer migrationsMu.Unlock()
	migrations = append(migrations, Migration{Name: name, Func: f})
}

func RunMigrations() interface{} {
	if dbInstance == nil {
		return nil
	}

	_, err := dbInstance.Exec("CREATE TABLE IF NOT EXISTS schema_migrations (version TEXT PRIMARY KEY)")
	if err != nil {
		LogWarn("Failed to create schema_migrations table: ", err.Error())
		return nil
	}

	rows, err := dbInstance.Query("SELECT version FROM schema_migrations")
	if err != nil {
		LogWarn("Failed to query schema_migrations: ", err.Error())
		return nil
	}
	defer rows.Close()

	executed := make(map[string]bool)
	for rows.Next() {
		var version string
		if err := rows.Scan(&version); err == nil {
			executed[version] = true
		}
	}

	migrationsMu.Lock()
	defer migrationsMu.Unlock()

	for _, m := range migrations {
		if !executed[m.Name] {
			LogInfo("Running database migration: ", m.Name)
			m.Func()
			_, err := dbInstance.Exec("INSERT INTO schema_migrations (version) VALUES (?)", m.Name)
			if err != nil {
				panic(fmt.Sprintf("Failed to record execution of migration %s: %s", m.Name, err.Error()))
			}
			LogInfo("Migration successful: ", m.Name)
		}
	}
	return nil
}

func StartServer() interface{} {
	for _, arg := range os.Args {
		if arg == "--mcp" {
			startMCPServer()
			return nil
		}
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
		handler, params, limiter := matchRoute(r.Method, r.URL.Path)

		if handler == nil {
			http.NotFound(w, r)
			return
		}

		if limiter != nil && !limiter.allow() {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusTooManyRequests)
			json.NewEncoder(w).Encode(map[string]interface{}{
				"status": 429,
				"error":  "Too Many Requests",
			})
			return
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		req := Request{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   string(bodyBytes),
			Params: params,
		}

		// OpenTelemetry: start request span
		parentTrace := r.Header.Get("traceparent")
		trace := TraceRequest(r.Method, r.URL.Path, parentTrace)

		start := time.Now()
		MetricInc("http_server_requests_total")

		res := handler(req)

		duration := time.Since(start).Seconds()
		MetricGauge("http_server_request_duration_seconds", duration)

		statusCode := 200
		w.Header().Set("Content-Type", "application/json")
		// Propagate trace context in response
		if tp := Traceparent(trace); tp != "" {
			w.Header().Set("traceparent", tp)
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

func handleMetrics(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "text/plain")

	metricsMu.RLock()
	for k, v := range metricsCounters {
		fmt.Fprintf(w, "%s_total %d\n", k, v)
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

// Pagination support for MongoDB queries

// DBQueryPage executes a paginated MongoDB find query.
// Usage: db.queryPage("collection", filter, page, pageSize)
func DBQueryPage(collection string, args ...interface{}) interface{} {
	endSpan := TraceDB("queryPage", collection)
	defer endSpan()

	if mongoDB == nil {
		panic("MongoDB not initialized for paginated queries")
	}

	var filter interface{} = bson.M{}
	page := 0
	pageSize := 20

	if len(args) >= 1 && args[0] != nil {
		filterStr, ok := args[0].(string)
		if ok && strings.TrimSpace(filterStr) != "" {
			var f interface{}
			if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
				filter = f
			}
		} else if !ok {
			filter = args[0]
		}
	}
	if len(args) >= 2 {
		page = toInt(args[1])
	}
	if len(args) >= 3 {
		pageSize = toInt(args[2])
		if pageSize > 100 {
			pageSize = 100
		}
	}

	coll := mongoDB.Collection(collection)

	// Count total
	total, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		panic(fmt.Sprintf("MongoDB count error: %s", err.Error()))
	}

	// Find with skip/limit
	opts := options.Find().SetSkip(int64(page * pageSize)).SetLimit(int64(pageSize))
	cursor, err := coll.Find(ctx, filter, opts)
	if err != nil {
		panic(fmt.Sprintf("MongoDB find error: %s", err.Error()))
	}
	defer cursor.Close(ctx)

	var results []interface{}
	for cursor.Next(ctx) {
		var row map[string]interface{}
		if err := cursor.Decode(&row); err == nil {
			results = append(results, ToSafeValue(row))
		}
	}

	return map[string]interface{}{
		"data":     results,
		"total":    total,
		"page":     page,
		"pageSize": pageSize,
		"pages":    (total + int64(pageSize) - 1) / int64(pageSize),
	}
}

// DBFindOne finds a single document matching the filter.
// Usage: db.findOne("collection", filter)
func DBFindOne(collection string, args ...interface{}) interface{} {
	endSpan := TraceDB("findOne", collection)
	defer endSpan()

	if mongoDB == nil {
		panic("MongoDB not initialized")
	}

	var filter interface{} = bson.M{}
	if len(args) >= 1 && args[0] != nil {
		filterStr, ok := args[0].(string)
		if ok && strings.TrimSpace(filterStr) != "" {
			var f interface{}
			if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
				filter = f
			}
		} else if !ok {
			filter = args[0]
		}
	}

	coll := mongoDB.Collection(collection)
	var result map[string]interface{}
	err := coll.FindOne(ctx, filter).Decode(&result)
	if err != nil {
		if err.Error() == "mongo: no documents in result" {
			return nil
		}
		panic(fmt.Sprintf("MongoDB findOne error: %s", err.Error()))
	}
	return ToSafeValue(result)
}

// DBCount counts documents matching a filter.
// Usage: db.count("collection", filter)
func DBCount(collection string, args ...interface{}) interface{} {
	endSpan := TraceDB("count", collection)
	defer endSpan()
	if mongoDB == nil {
		panic("MongoDB not initialized")
	}

	var filter interface{} = bson.M{}
	if len(args) >= 1 && args[0] != nil {
		filterStr, ok := args[0].(string)
		if ok && strings.TrimSpace(filterStr) != "" {
			var f interface{}
			if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
				filter = f
			}
		} else if !ok {
			filter = args[0]
		}
	}

	coll := mongoDB.Collection(collection)
	count, err := coll.CountDocuments(ctx, filter)
	if err != nil {
		panic(fmt.Sprintf("MongoDB count error: %s", err.Error()))
	}
	return count
}

// DBUpsert inserts or updates a document.
// Usage: db.upsert("collection", filter, update)
func DBUpsert(collection string, args ...interface{}) interface{} {
	endSpan := TraceDB("upsert", collection)
	defer endSpan()
	if mongoDB == nil {
		panic("MongoDB not initialized")
	}
	if len(args) < 2 {
		panic("db.upsert requires filter and update arguments")
	}

	var filter interface{} = bson.M{}
	var update interface{}

	// Parse filter
	filterStr, ok := args[0].(string)
	if ok {
		var f interface{}
		if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
			filter = f
		}
	} else {
		filter = args[0]
	}

	// Parse update
	updateStr, ok := args[1].(string)
	if ok {
		var u interface{}
		if err := json.Unmarshal([]byte(updateStr), &u); err == nil {
			update = u
		}
	} else {
		update = args[1]
	}

	coll := mongoDB.Collection(collection)
	opts := options.Update().SetUpsert(true)
	result, err := coll.UpdateOne(ctx, filter, update, opts)
	if err != nil {
		panic(fmt.Sprintf("MongoDB upsert error: %s", err.Error()))
	}

	return map[string]interface{}{
		"matched_count":  result.MatchedCount,
		"modified_count": result.ModifiedCount,
		"upserted_id":   fmt.Sprint(result.UpsertedID),
	}
}

// Helper to configure connection pool from YAML config or Env
func configureDBPool(db *sql.DB) {
	maxOpen := 25
	maxIdle := 25
	lifetime := 5 * time.Minute

	if valStr := Config("database.max_open_conns"); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil && val > 0 {
			maxOpen = val
		}
	}
	if valStr := Config("database.max_idle_conns"); valStr != "" {
		if val, err := strconv.Atoi(valStr); err == nil && val > 0 {
			maxIdle = val
		}
	}
	if valStr := Config("database.conn_max_lifetime"); valStr != "" {
		if dur, err := time.ParseDuration(valStr); err == nil && dur > 0 {
			lifetime = dur
		}
	}

	db.SetMaxOpenConns(maxOpen)
	db.SetMaxIdleConns(maxIdle)
	db.SetConnMaxLifetime(lifetime)
}

// SQLite, PostgreSQL, Oracle, and MongoDB Database Integrations
func InitDB(connStr string) {
	if strings.HasPrefix(connStr, "sqlite://") {
		dbPath := strings.TrimPrefix(connStr, "sqlite://")
		var err error
		dbInstance, err = sql.Open("sqlite", dbPath)
		if err != nil {
			panic(fmt.Sprintf("Failed to open SQLite database %s: %s", dbPath, err.Error()))
		}
		configureDBPool(dbInstance)
		LogInfo("Connected to SQLite database: ", dbPath)
	} else if strings.HasPrefix(connStr, "postgres://") || strings.HasPrefix(connStr, "postgresql://") {
		var err error
		dbInstance, err = sql.Open("postgres", connStr)
		if err != nil {
			panic(fmt.Sprintf("Failed to open PostgreSQL database: %s", err.Error()))
		}
		configureDBPool(dbInstance)
		LogInfo("Connected to PostgreSQL database successfully")
	} else if strings.HasPrefix(connStr, "oracle://") {
		var err error
		dbInstance, err = sql.Open("oracle", connStr)
		if err != nil {
			panic(fmt.Sprintf("Failed to open Oracle database: %s", err.Error()))
		}
		configureDBPool(dbInstance)
		LogInfo("Connected to Oracle database successfully")
	} else if strings.HasPrefix(connStr, "mongodb://") {
		clientOptions := options.Client().ApplyURI(connStr)
		var err error
		mongoClient, err = mongo.Connect(ctx, clientOptions)
		if err != nil {
			panic(fmt.Sprintf("Failed to connect to MongoDB: %s", err.Error()))
		}
		err = mongoClient.Ping(ctx, nil)
		if err != nil {
			LogWarn("Failed to ping MongoDB (offline/unreachable): ", err.Error())
		}
		dbName := "serv_db"
		parts := strings.Split(connStr, "/")
		if len(parts) > 3 {
			dbAndOpts := parts[len(parts)-1]
			optParts := strings.Split(dbAndOpts, "?")
			if optParts[0] != "" {
				dbName = optParts[0]
			}
		}
		mongoDB = mongoClient.Database(dbName)
		LogInfo("Connected to MongoDB successfully. Target Database: ", dbName)
	} else {
		panic(fmt.Sprintf("Unsupported database scheme in connection string: %s", connStr))
	}
}

func getCachedStmt(query string) (*sql.Stmt, error) {
	stmtCacheMu.RLock()
	stmt, exists := stmtCache[query]
	stmtCacheMu.RUnlock()
	if exists {
		// Promote to most-recently-used (move to end of keys list)
		stmtCacheMu.Lock()
		for i, k := range stmtCacheKeys {
			if k == query {
				stmtCacheKeys = append(stmtCacheKeys[:i], stmtCacheKeys[i+1:]...)
				stmtCacheKeys = append(stmtCacheKeys, query)
				break
			}
		}
		stmtCacheMu.Unlock()
		return stmt, nil
	}

	stmtCacheMu.Lock()
	defer stmtCacheMu.Unlock()
	// Double-check after acquiring write lock
	if stmt, exists = stmtCache[query]; exists {
		return stmt, nil
	}

	stmt, err := dbInstance.Prepare(query)
	if err != nil {
		return nil, err
	}

	// LRU eviction: if cache is full, close and remove the least-recently-used entry
	if len(stmtCacheKeys) >= stmtCacheMax {
		oldest := stmtCacheKeys[0]
		stmtCacheKeys = stmtCacheKeys[1:]
		if oldStmt, ok := stmtCache[oldest]; ok {
			oldStmt.Close()
			delete(stmtCache, oldest)
		}
	}

	stmtCache[query] = stmt
	stmtCacheKeys = append(stmtCacheKeys, query)
	return stmt, nil
}

func AddBeforeQueryHook(hook func(interface{}, interface{}) interface{}) {
	beforeQueryHooksMu.Lock()
	defer beforeQueryHooksMu.Unlock()
	beforeQueryHooks = append(beforeQueryHooks, hook)
}

func DBQuery(query string, args ...interface{}) interface{} {
	endSpan := TraceDB("query", query)
	defer endSpan()

	// Trigger beforeQuery hooks
	beforeQueryHooksMu.RLock()
	for _, hook := range beforeQueryHooks {
		hook(query, args)
	}
	beforeQueryHooksMu.RUnlock()
	isMongoAction := false
	if mongoDB != nil {
		q := strings.ToLower(strings.TrimSpace(query))
		if q == "find" || q == "insert" || q == "insertone" || q == "update" || q == "updateone" || q == "delete" || q == "deleteone" || q == "count" {
			isMongoAction = true
		}
	}

	if isMongoAction {
		return runMongoQuery(query, args...)
	}

	if dbInstance == nil {
		panic("Database is not initialized. Declare database 'sqlite://...', 'postgres://...', or 'oracle://...' first.")
	}

	stmt, err := getCachedStmt(query)
	if err != nil {
		panic(fmt.Sprintf("Failed to prepare database statement: %s", err.Error()))
	}

	queryLower := strings.ToLower(strings.TrimSpace(query))
	if strings.HasPrefix(queryLower, "insert") || strings.HasPrefix(queryLower, "update") ||
		strings.HasPrefix(queryLower, "delete") || strings.HasPrefix(queryLower, "create") ||
		strings.HasPrefix(queryLower, "replace") {
		res, err := stmt.ExecContext(ctx, args...)
		if err != nil {
			panic(fmt.Sprintf("Database exec error: %s", err.Error()))
		}
		lastInsertID, _ := res.LastInsertId()
		rowsAffected, _ := res.RowsAffected()
		return map[string]interface{}{
			"last_insert_id": lastInsertID,
			"rows_affected":  rowsAffected,
		}
	}

	rows, err := stmt.QueryContext(ctx, args...)
	if err != nil {
		panic(fmt.Sprintf("Database query error: %s", err.Error()))
	}
	defer rows.Close()

	columns, err := rows.Columns()
	if err != nil {
		panic(err.Error())
	}

	var results []interface{}
	for rows.Next() {
		values := make([]interface{}, len(columns))
		valuePtrs := make([]interface{}, len(columns))
		for i := range columns {
			valuePtrs[i] = &values[i]
		}

		if err := rows.Scan(valuePtrs...); err != nil {
			panic(err.Error())
		}

		row := NewSafeMap()
		for i, col := range columns {
			val := values[i]
			if b, ok := val.([]byte); ok {
				row.Set(col, string(b))
			} else {
				row.Set(col, val)
			}
		}
		results = append(results, row)
	}
	return results
}

func runMongoQuery(action string, args ...interface{}) interface{} {
	if len(args) < 1 {
		panic("MongoDB query requires collection name as the first argument, e.g. db.query(\"find\", \"users\", \"{}\")")
	}
	collName, ok := args[0].(string)
	if !ok {
		panic("MongoDB collection name must be a string")
	}

	collection := mongoDB.Collection(collName)

	var filter interface{} = bson.M{}
	if len(args) > 1 {
		filterStr, ok := args[1].(string)
		if ok {
			if strings.TrimSpace(filterStr) != "" {
				var f interface{}
				if err := json.Unmarshal([]byte(filterStr), &f); err == nil {
					filter = f
				} else {
					filter = bson.M{"_id": filterStr}
				}
			}
		} else {
			filter = args[1]
		}
	}

	actionLower := strings.ToLower(strings.TrimSpace(action))
	switch actionLower {
	case "find":
		cursor, err := collection.Find(ctx, filter)
		if err != nil {
			panic(fmt.Sprintf("MongoDB Find error: %s", err.Error()))
		}
		defer cursor.Close(ctx)
		var results []interface{}
		for cursor.Next(ctx) {
			var row map[string]interface{}
			if err := cursor.Decode(&row); err == nil {
				if oid, ok := row["_id"].(interface{ String() string }); ok {
					row["_id"] = oid.String()
				}
				results = append(results, ToSafeValue(row))
			}
		}
		return results

	case "insert", "insertone":
		res, err := collection.InsertOne(ctx, filter)
		if err != nil {
			panic(fmt.Sprintf("MongoDB Insert error: %s", err.Error()))
		}
		return map[string]interface{}{
			"inserted_id": fmt.Sprint(res.InsertedID),
		}

	case "update", "updateone":
		if len(args) < 3 {
			panic("MongoDB update requires update document as the third argument")
		}
		var update interface{}
		updateStr, ok := args[2].(string)
		if ok {
			var u interface{}
			if err := json.Unmarshal([]byte(updateStr), &u); err == nil {
				update = u
			} else {
				panic("MongoDB update document is invalid JSON")
			}
		} else {
			update = args[2]
		}

		res, err := collection.UpdateOne(ctx, filter, update)
		if err != nil {
			panic(fmt.Sprintf("MongoDB Update error: %s", err.Error()))
		}
		return map[string]interface{}{
			"matched_count":  res.MatchedCount,
			"modified_count": res.ModifiedCount,
		}

	case "delete", "deleteone":
		res, err := collection.DeleteOne(ctx, filter)
		if err != nil {
			panic(fmt.Sprintf("MongoDB Delete error: %s", err.Error()))
		}
		return map[string]interface{}{
			"deleted_count": res.DeletedCount,
		}

	case "count":
		count, err := collection.CountDocuments(ctx, filter)
		if err != nil {
			panic(fmt.Sprintf("MongoDB Count error: %s", err.Error()))
		}
		return count

	default:
		panic(fmt.Sprintf("Unsupported MongoDB action: %s. Supported actions: find, insert, update, delete, count", action))
	}
}

// Safe variants that return [2]interface{}{value, error} tuples for multi-return support.
// These are used when Serv code uses: let result, err = db.querySafe(...)

func DBQuerySafe(query string, args ...interface{}) interface{} {
	var result interface{}
	var errVal interface{}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				errVal = fmt.Sprint(rec)
			}
		}()
		result = DBQuery(query, args...)
	}()
	return [2]interface{}{result, errVal}
}

// Redis & In-Memory Cache
func InitCache(connStr string) {
	if strings.HasPrefix(connStr, "redis://") {
		opt, err := redis.ParseURL(connStr)
		if err != nil {
			panic(fmt.Sprintf("Invalid Redis URL: %s", err.Error()))
		}
		redisClient = redis.NewClient(opt)
		LogInfo("Connected to Redis cache: ", connStr)
	} else {
		LogInfo("Initialized in-memory cache fallback")
	}
}

func CacheSet(key string, value interface{}, durationStr string) {
	endSpan := TraceCache("SET", key)
	defer endSpan()

	duration, err := time.ParseDuration(durationStr)
	if err != nil {
		duration = 10 * time.Minute // default fallback
	}

	if redisClient != nil {
		b, _ := json.Marshal(value)
		err := redisClient.Set(ctx, key, string(b), duration).Err()
		if err != nil {
			panic(fmt.Sprintf("Redis SET error: %s", err.Error()))
		}
	} else {
		localCacheMu.Lock()
		localCache[key] = localCacheEntry{
			value:      value,
			expiration: time.Now().Add(duration),
		}
		localCacheMu.Unlock()
	}
}

func CacheGet(key string) interface{} {
	endSpan := TraceCache("GET", key)
	defer endSpan()

	if redisClient != nil {
		val, err := redisClient.Get(ctx, key).Result()
		if err == redis.Nil {
			return nil
		} else if err != nil {
			panic(fmt.Sprintf("Redis GET error: %s", err.Error()))
		}
		var parsed interface{}
		if err := json.Unmarshal([]byte(val), &parsed); err == nil {
			return parsed
		}
		return val
	} else {
		localCacheMu.RLock()
		entry, exists := localCache[key]
		localCacheMu.RUnlock()

		if !exists {
			return nil
		}
		if time.Now().After(entry.expiration) {
			localCacheMu.Lock()
			delete(localCache, key)
			localCacheMu.Unlock()
			return nil
		}
		return entry.value
	}
}

type pubSubEvent struct {
	callback func(string)
	payload  string
}

func startPubSubWorkers() {
	pubSubWorkerOnce.Do(func() {
		for i := 0; i < pubSubWorkers; i++ {
			go func() {
				for event := range pubSubQueue {
					executeCallbackSafe(event.callback, event.payload)
				}
			}()
		}
	})
}

func executeCallbackSafe(callback func(string), payload string) {
	defer func() {
		if r := recover(); r != nil {
			LogError("Recovered in subscribe callback: ", r)
		}
	}()
	MetricInc("broker_messages_received_total")
	callback(payload)
}

type trieNode struct {
	children  map[string]*trieNode
	handler   func(Request) interface{}
	isParam   bool
	paramName string
	limiter   *routeRateLimiter
}

func newTrieNode() *trieNode {
	return &trieNode{children: make(map[string]*trieNode)}
}

func insertRoute(method, path string, limiter *routeRateLimiter, handler func(Request) interface{}) {
	routeTrieMu.Lock()
	defer routeTrieMu.Unlock()

	root, ok := routeTrie[method]
	if !ok {
		root = newTrieNode()
		routeTrie[method] = root
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	curr := root
	for _, part := range parts {
		if part == "" {
			continue
		}
		isParam := strings.HasPrefix(part, ":")
		paramName := ""
		childKey := part
		if isParam {
			paramName = strings.TrimPrefix(part, ":")
			childKey = ":"
		}

		child, ok := curr.children[childKey]
		if !ok {
			child = newTrieNode()
			child.isParam = isParam
			child.paramName = paramName
			curr.children[childKey] = child
		}
		curr = child
	}
	curr.handler = handler
	curr.limiter = limiter
}

func matchRoute(method, path string) (func(Request) interface{}, map[string]string, *routeRateLimiter) {
	routeTrieMu.RLock()
	root, ok := routeTrie[method]
	routeTrieMu.RUnlock()
	if !ok {
		return nil, nil, nil
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	params := make(map[string]string)
	curr := root

	for _, part := range parts {
		if part == "" {
			continue
		}
		if child, ok := curr.children[part]; ok {
			curr = child
		} else if child, ok := curr.children[":"]; ok {
			params[child.paramName] = part
			curr = child
		} else {
			return nil, nil, nil
		}
	}
	return curr.handler, params, curr.limiter
}
