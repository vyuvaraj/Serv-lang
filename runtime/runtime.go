package runtime

import (
	"bufio"
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"io"
	"log"
	"net/http"
	"os"
	"os/exec"
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
	"github.com/gorilla/websocket"
	"github.com/nats-io/nats.go"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/robfig/cron/v3"
	"github.com/segmentio/kafka-go"
)

type pythonWorker struct {
	cmd    *exec.Cmd
	stdin  io.WriteCloser
	stdout *bufio.Reader
	mutex  sync.Mutex
}

// Global State
var (
	brokerURL    string
	serverPort   string
	routes       = make(map[string]map[string]func(Request) interface{}) // method -> path -> handler
	routesMu     sync.RWMutex

	routeTrie    = make(map[string]*trieNode) // method -> root trie node
	routeTrieMu  sync.RWMutex
	cronInstance *cron.Cron
	cronOnce     sync.Once

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

	// Python interop daemon pool state
	pythonPoolQueue   chan *pythonWorker
	pythonWorkersOnce sync.Once

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

	// Concurrency Semaphores
	semaphores   = make(map[string]chan struct{})
	semaphoresMu sync.Mutex

	// Metrics
	metricsCounters = make(map[string]int64)
	metricsGauges   MapStringFloat
	metricsMu       sync.RWMutex

	// Config Map
	configMap   = make(map[string]string)
	configMapMu sync.RWMutex

	// Database query hooks
	beforeQueryHooks   []func(interface{}, interface{}) interface{}
	beforeQueryHooksMu sync.RWMutex
)

// Noop is a no-op sentinel used by generated test files to satisfy the runtime import.
func Noop() {}

// Collection methods — operate on []interface{} slices

// Filter returns elements where the callback returns true.
func Filter(slice interface{}, callback func(interface{}) interface{}) interface{} {
	items := toInterfaceSlice(slice)
	var result []interface{}
	for _, item := range items {
		val := callback(item)
		if isTruthyVal(val) {
			result = append(result, item)
		}
	}
	return result
}

// Map transforms each element using the callback.
func Map(slice interface{}, callback func(interface{}) interface{}) interface{} {
	items := toInterfaceSlice(slice)
	result := make([]interface{}, len(items))
	for i, item := range items {
		result[i] = callback(item)
	}
	return result
}

// Find returns the first element where callback returns true, or nil.
func Find(slice interface{}, callback func(interface{}) interface{}) interface{} {
	items := toInterfaceSlice(slice)
	for _, item := range items {
		val := callback(item)
		if isTruthyVal(val) {
			return item
		}
	}
	return nil
}

// Reduce accumulates a value by applying callback(accumulator, item) for each element.
func Reduce(slice interface{}, callback func(interface{}, interface{}) interface{}, initial interface{}) interface{} {
	items := toInterfaceSlice(slice)
	acc := initial
	for _, item := range items {
		acc = callback(acc, item)
	}
	return acc
}

// ForEach calls the callback for each element (no return value).
func ForEach(slice interface{}, callback func(interface{}) interface{}) interface{} {
	items := toInterfaceSlice(slice)
	for _, item := range items {
		callback(item)
	}
	return nil
}

// Length returns the length of a slice or string.
func Length(val interface{}) interface{} {
	switch v := val.(type) {
	case []interface{}:
		return len(v)
	case string:
		return len(v)
	case *SafeMap:
		v.mu.RLock()
		defer v.mu.RUnlock()
		return len(v.m)
	case map[string]interface{}:
		return len(v)
	default:
		return 0
	}
}

// Push appends an element to a slice and returns the new slice.
func Push(slice interface{}, elem interface{}) interface{} {
	items := toInterfaceSlice(slice)
	return append(items, elem)
}

// Contains checks if a slice contains an element.
func Contains(slice interface{}, target interface{}) interface{} {
	items := toInterfaceSlice(slice)
	targetStr := fmt.Sprint(target)
	for _, item := range items {
		if fmt.Sprint(item) == targetStr {
			return true
		}
	}
	return false
}

func toInterfaceSlice(val interface{}) []interface{} {
	if val == nil {
		return nil
	}
	if s, ok := val.([]interface{}); ok {
		return s
	}
	return nil
}

func isTruthyVal(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != ""
	default:
		return true
	}
}

// String methods

func StringSplit(s interface{}, sep interface{}) interface{} {
	str := fmt.Sprint(s)
	separator := fmt.Sprint(sep)
	parts := strings.Split(str, separator)
	result := make([]interface{}, len(parts))
	for i, p := range parts {
		result[i] = p
	}
	return result
}

func StringTrim(s interface{}) interface{} {
	return strings.TrimSpace(fmt.Sprint(s))
}

func StringReplace(s interface{}, old interface{}, new interface{}) interface{} {
	return strings.ReplaceAll(fmt.Sprint(s), fmt.Sprint(old), fmt.Sprint(new))
}

func StringStartsWith(s interface{}, prefix interface{}) interface{} {
	return strings.HasPrefix(fmt.Sprint(s), fmt.Sprint(prefix))
}

func StringEndsWith(s interface{}, suffix interface{}) interface{} {
	return strings.HasSuffix(fmt.Sprint(s), fmt.Sprint(suffix))
}

func StringIncludes(s interface{}, substr interface{}) interface{} {
	return strings.Contains(fmt.Sprint(s), fmt.Sprint(substr))
}

func StringToUpper(s interface{}) interface{} {
	return strings.ToUpper(fmt.Sprint(s))
}

func StringToLower(s interface{}) interface{} {
	return strings.ToLower(fmt.Sprint(s))
}

func StringSubstring(s interface{}, start interface{}, args ...interface{}) interface{} {
	str := fmt.Sprint(s)
	startIdx := toInt(start)
	if startIdx < 0 {
		startIdx = 0
	}
	if startIdx >= len(str) {
		return ""
	}
	if len(args) > 0 {
		endIdx := toInt(args[0])
		if endIdx > len(str) {
			endIdx = len(str)
		}
		if endIdx < startIdx {
			return ""
		}
		return str[startIdx:endIdx]
	}
	return str[startIdx:]
}

func StringIndexOf(s interface{}, substr interface{}) interface{} {
	return strings.Index(fmt.Sprint(s), fmt.Sprint(substr))
}

func StringRepeat(s interface{}, count interface{}) interface{} {
	return strings.Repeat(fmt.Sprint(s), toInt(count))
}

func toInt(v interface{}) int {
	switch val := v.(type) {
	case int:
		return val
	case int64:
		return int(val)
	case float64:
		return int(val)
	case string:
		n, _ := strconv.Atoi(val)
		return n
	default:
		n, _ := strconv.Atoi(fmt.Sprint(v))
		return n
	}
}

type localCacheEntry struct {
	value      interface{}
	expiration time.Time
}

type MapStringFloat struct {
	m map[string]float64
	sync.RWMutex
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

// Logging

var (
	logJSON    bool
	logLevel   = "info" // "debug", "info", "warn", "error"
	logLevelMu sync.RWMutex
)

func init() {
	// Check for JSON log mode
	if Config("log.format") == "json" || os.Getenv("LOG_FORMAT") == "json" {
		logJSON = true
	}
	if lvl := Config("log.level"); lvl != "" {
		logLevel = lvl
	} else if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		logLevel = lvl
	}
}

func shouldLog(level string) bool {
	levels := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}
	logLevelMu.RLock()
	defer logLevelMu.RUnlock()
	return levels[level] >= levels[logLevel]
}

func logStructured(level string, args ...interface{}) {
	if !shouldLog(level) {
		return
	}
	msg := fmt.Sprint(args...)
	if logJSON {
		entry := map[string]interface{}{
			"level":     level,
			"message":   msg,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		b, _ := json.Marshal(entry)
		fmt.Println(string(b))
	} else {
		log.Printf("[%s] %s", strings.ToUpper(level), msg)
	}
}

func LogInfo(args ...interface{}) {
	logStructured("info", args...)
}

func LogWarn(args ...interface{}) {
	logStructured("warn", args...)
}

func LogError(args ...interface{}) {
	logStructured("error", args...)
}

func LogDebug(args ...interface{}) {
	logStructured("debug", args...)
}

// LogWith logs a message with structured context fields (key-value pairs).
// Usage from Serv: log.with("user_id", 123, "action", "login")
func LogWith(args ...interface{}) interface{} {
	if len(args) < 2 {
		return nil
	}
	// Last arg is the message, preceding pairs are key-value context
	msg := fmt.Sprint(args[len(args)-1])
	fields := make(map[string]interface{})
	for i := 0; i+1 < len(args)-1; i += 2 {
		fields[fmt.Sprint(args[i])] = args[i+1]
	}

	if logJSON {
		entry := map[string]interface{}{
			"level":     "info",
			"message":   msg,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		for k, v := range fields {
			entry[k] = v
		}
		b, _ := json.Marshal(entry)
		fmt.Println(string(b))
	} else {
		var pairs []string
		for k, v := range fields {
			pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
		}
		log.Printf("[INFO] %s %s", msg, strings.Join(pairs, " "))
	}
	return nil
}

// Metrics
func MetricInc(name string) {
	metricsMu.Lock()
	defer metricsMu.Unlock()
	metricsCounters[name]++
}

func MetricGauge(name string, val float64) {
	metricsGauges.Lock()
	defer metricsGauges.Unlock()
	metricsGauges.m[name] = val
}

// HTTP Client
func HTTPGet(url string) HTTPResponse {
	start := time.Now()
	MetricInc("http_client_requests_total")
	resp, err := http.Get(url)
	if err != nil {
		MetricInc("http_client_errors_total")
		panic(fmt.Sprintf("HTTP GET request failed for %s: %s", url, err.Error()))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	duration := time.Since(start).Seconds()
	MetricGauge("http_client_request_duration_seconds", duration)

	return HTTPResponse{Status: resp.StatusCode, Body: string(body)}
}

func HTTPPost(url string, body interface{}) HTTPResponse {
	start := time.Now()
	MetricInc("http_client_requests_total")

	var buf bytes.Buffer
	if strBody, ok := body.(string); ok {
		buf.WriteString(strBody)
	} else {
		json.NewEncoder(&buf).Encode(body)
	}

	resp, err := http.Post(url, "application/json", &buf)
	if err != nil {
		MetricInc("http_client_errors_total")
		panic(fmt.Sprintf("HTTP POST request failed for %s: %s", url, err.Error()))
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)

	duration := time.Since(start).Seconds()
	MetricGauge("http_client_request_duration_seconds", duration)

	return HTTPResponse{Status: resp.StatusCode, Body: string(bodyBytes)}
}

// Scheduler: Every
func Every(intervalStr string, callback func()) {
	duration, err := time.ParseDuration(intervalStr)
	if err != nil {
		if secs, err2 := fmt.Sscanf(intervalStr, "%d", &duration); err2 == nil && secs > 0 {
			duration = duration * time.Second
		} else {
			LogError("Invalid interval: ", intervalStr, " error: ", err)
			return
		}
	}

	go func() {
		ticker := time.NewTicker(duration)
		for range ticker.C {
			start := time.Now()
			MetricInc("scheduler_jobs_executed_total")
			func() {
				defer func() {
					if r := recover(); r != nil {
						MetricInc("scheduler_jobs_failed_total")
						LogError("Recovered in every job: ", r)
					}
				}()
				callback()
			}()
			durationSecs := time.Since(start).Seconds()
			MetricGauge("scheduler_job_duration_seconds", durationSecs)
		}
	}()
}

// Scheduler: Cron
func Cron(cronExpr string, callback func()) {
	cronOnce.Do(func() {
		cronInstance = cron.New(cron.WithSeconds())
		cronInstance.Start()
	})

	_, err := cronInstance.AddFunc(cronExpr, func() {
		start := time.Now()
		MetricInc("scheduler_jobs_executed_total")
		func() {
			defer func() {
				if r := recover(); r != nil {
					MetricInc("scheduler_jobs_failed_total")
					LogError("Recovered in cron job: ", r)
				}
			}()
			callback()
		}()
		durationSecs := time.Since(start).Seconds()
		MetricGauge("scheduler_job_duration_seconds", durationSecs)
	})
	if err != nil {
		LogError("Failed to register cron expression: ", cronExpr, " error: ", err)
	}
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

// Await runs a function asynchronously and blocks until it returns.
func Await(fn func() interface{}) interface{} {
	ch := make(chan interface{}, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- nil
			}
		}()
		ch <- fn()
	}()
	return <-ch
}

// AwaitAll runs multiple functions concurrently and returns all results as []interface{}.
func AwaitAll(fns []func() interface{}) interface{} {
	results := make([]interface{}, len(fns))
	var wg sync.WaitGroup
	wg.Add(len(fns))
	for i, fn := range fns {
		go func(idx int, f func() interface{}) {
			defer wg.Done()
			defer func() {
				if r := recover(); r != nil {
					results[idx] = nil
				}
			}()
			results[idx] = f()
		}(i, fn)
	}
	wg.Wait()
	return results
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

// WebSocket support

type WSConn struct {
	conn   *websocket.Conn
	mu     sync.Mutex
}

func (w *WSConn) Send(msg interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var data []byte
	if str, ok := msg.(string); ok {
		data = []byte(str)
	} else {
		data, _ = json.Marshal(msg)
	}
	w.conn.WriteMessage(websocket.TextMessage, data)
}

func (w *WSConn) Receive() interface{} {
	_, message, err := w.conn.ReadMessage()
	if err != nil {
		return nil
	}
	return string(message)
}

func (w *WSConn) Close() {
	w.conn.Close()
}

var (
	wsHandlers   = make(map[string]func(*WSConn))
	wsHandlersMu sync.RWMutex
	upgrader     = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func AddWebSocket(path string, handler func(*WSConn)) {
	wsHandlersMu.Lock()
	wsHandlers[path] = handler
	wsHandlersMu.Unlock()
	LogInfo("Registered WebSocket endpoint: ", path)
}

type MCPTool struct {
	Name        string
	Description string
	Handler     func(interface{}) interface{}
}

var (
	mcpTools   = make(map[string]MCPTool)
	mcpToolsMu sync.RWMutex
)

func AddMCPTool(name, description string, handler func(interface{}) interface{}) {
	mcpToolsMu.Lock()
	defer mcpToolsMu.Unlock()
	mcpTools[name] = MCPTool{
		Name:        name,
		Description: description,
		Handler:     handler,
	}
}

func InvokeMCPToolForTesting(name string, args interface{}) interface{} {
	mcpToolsMu.RLock()
	t, ok := mcpTools[name]
	mcpToolsMu.RUnlock()
	if !ok {
		panic("tool not found: " + name)
	}
	return t.Handler(args)
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func startMCPServer() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendRPCError(nil, -32700, "Parse error")
			continue
		}
		handleMCPRequest(req)
	}
}

func sendRPCError(id interface{}, code int, message string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	b, _ := json.Marshal(resp)
	fmt.Println(string(b))
}

func handleMCPRequest(req jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "Serv-MCP",
					"version": "1.0.0",
				},
			},
		}
		b, _ := json.Marshal(resp)
		fmt.Println(string(b))

	case "notifications/initialized":
		// Notification, no reply

	case "tools/list":
		mcpToolsMu.RLock()
		toolsList := []map[string]interface{}{}
		for _, t := range mcpTools {
			toolsList = append(toolsList, map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			})
		}
		mcpToolsMu.RUnlock()

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": toolsList,
			},
		}
		b, _ := json.Marshal(resp)
		fmt.Println(string(b))

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendRPCError(req.ID, -32602, "Invalid params")
			return
		}

		mcpToolsMu.RLock()
		tool, exists := mcpTools[params.Name]
		mcpToolsMu.RUnlock()

		if !exists {
			sendRPCError(req.ID, -32601, "Tool not found: "+params.Name)
			return
		}

		// Run tool handler
		res := tool.Handler(params.Arguments)

		// Convert result to standard MCP content
		var text string
		if resStr, ok := res.(string); ok {
			text = resStr
		} else {
			b, _ := json.Marshal(res)
			text = string(b)
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": text,
					},
				},
			},
		}
		b, _ := json.Marshal(resp)
		fmt.Println(string(b))

	default:
		sendRPCError(req.ID, -32601, "Method not found: "+req.Method)
	}
}

func Sleep(ms interface{}) interface{} {
	var val int
	switch v := ms.(type) {
	case int:
		val = v
	case int64:
		val = int(v)
	case float64:
		val = int(v)
	case string:
		val, _ = strconv.Atoi(v)
	default:
		val, _ = strconv.Atoi(fmt.Sprint(v))
	}
	time.Sleep(time.Duration(val) * time.Millisecond)
	return nil
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

		start := time.Now()
		MetricInc("http_server_requests_total")

		res := handler(req)

		duration := time.Since(start).Seconds()
		MetricGauge("http_server_request_duration_seconds", duration)

		w.Header().Set("Content-Type", "application/json")
		if resMap, ok := res.(map[string]interface{}); ok {
			json.NewEncoder(w).Encode(resMap)
		} else if resStr, ok := res.(string); ok {
			w.Write([]byte(resStr))
		} else {
			json.NewEncoder(w).Encode(res)
		}
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

func initPythonDaemonPool() {
	pythonWorkersOnce.Do(func() {
		workersCount := 4
		if valStr := Config("python.workers"); valStr != "" {
			if val, err := strconv.Atoi(valStr); err == nil && val > 0 {
				workersCount = val
			}
		}

		pythonPoolQueue = make(chan *pythonWorker, workersCount)

		for i := 0; i < workersCount; i++ {
			worker := spawnPythonWorker()
			if worker == nil {
				panic("Failed to start Python worker during pool initialization")
			}
			pythonPoolQueue <- worker
		}
	})
}

// Call Python Script for extern mappings using the persistent daemon pool
func CallPython(scriptPath string, funcName string, args ...interface{}) interface{} {
	initPythonDaemonPool()

	worker := <-pythonPoolQueue
	defer func() {
		pythonPoolQueue <- worker
	}()

	worker.mutex.Lock()
	defer worker.mutex.Unlock()

	// Health check: if the process has exited, respawn it
	if worker.cmd.ProcessState != nil || !isProcessAlive(worker.cmd) {
		LogWarn("Python worker died, respawning...")
		newWorker := spawnPythonWorker()
		if newWorker != nil {
			worker.cmd = newWorker.cmd
			worker.stdin = newWorker.stdin
			worker.stdout = newWorker.stdout
		} else {
			LogError("Failed to respawn Python worker")
			return nil
		}
	}

	reqPayload := map[string]interface{}{
		"script_path": scriptPath,
		"func_name":   funcName,
		"args":        args,
	}

	payloadBytes, err := json.Marshal(reqPayload)
	if err != nil {
		LogError("Failed to marshal Python daemon request: ", err)
		return nil
	}

	_, err = worker.stdin.Write(append(payloadBytes, '\n'))
	if err != nil {
		// Write failed — worker is likely dead, try respawn once
		LogWarn("Python worker write failed, respawning: ", err)
		newWorker := spawnPythonWorker()
		if newWorker == nil {
			LogError("Failed to respawn Python worker after write error")
			return nil
		}
		worker.cmd = newWorker.cmd
		worker.stdin = newWorker.stdin
		worker.stdout = newWorker.stdout

		// Retry the write
		_, err = worker.stdin.Write(append(payloadBytes, '\n'))
		if err != nil {
			LogError("Failed to write to respawned Python worker: ", err)
			return nil
		}
	}

	line, err := worker.stdout.ReadBytes('\n')
	if err != nil {
		LogError("Failed to read response from Python daemon: ", err)
		return nil
	}

	var res struct {
		Result interface{} `json:"result"`
		Error  string      `json:"error"`
	}

	if err := json.Unmarshal(line, &res); err != nil {
		LogError("Failed to unmarshal Python daemon response: ", err)
		return string(line)
	}

	if res.Error != "" {
		LogError("Python daemon execution error: ", res.Error)
		return nil
	}

	return res.Result
}

// isProcessAlive checks if the underlying process is still running.
func isProcessAlive(cmd *exec.Cmd) bool {
	if cmd == nil || cmd.Process == nil {
		return false
	}
	// On Unix, sending signal 0 checks if process exists.
	// On Windows, Process.Signal is not fully supported, so we check ProcessState.
	if cmd.ProcessState != nil {
		return false
	}
	return true
}

// spawnPythonWorker creates a single new Python daemon worker.
func spawnPythonWorker() *pythonWorker {
	daemonCode := `
import sys
import json
import importlib.util

modules = {}

while True:
    line = sys.stdin.readline()
    if not line:
        break
    try:
        req = json.loads(line)
        script_path = req["script_path"]
        func_name = req["func_name"]
        args = req["args"]

        if script_path not in modules:
            spec = importlib.util.spec_from_file_location("module", script_path)
            module = importlib.util.module_from_spec(spec)
            spec.loader.exec_module(module)
            modules[script_path] = module
        else:
            module = modules[script_path]

        fn = getattr(module, func_name)
        res = fn(*args)
        print(json.dumps({"result": res}))
        sys.stdout.flush()
    except Exception as e:
        print(json.dumps({"error": str(e)}))
        sys.stdout.flush()
`
	cmd := exec.Command("python", "-u", "-c", daemonCode)
	stdin, err := cmd.StdinPipe()
	if err != nil {
		LogError("Failed to create stdin pipe for Python worker: ", err)
		return nil
	}
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		LogError("Failed to create stdout pipe for Python worker: ", err)
		return nil
	}
	stdout := bufio.NewReader(stdoutPipe)

	if err := cmd.Start(); err != nil {
		LogError("Failed to start Python worker: ", err)
		return nil
	}

	return &pythonWorker{
		cmd:    cmd,
		stdin:  stdin,
		stdout: stdout,
	}
}

// shutdownPythonWorkers terminates all Python daemon workers.
func shutdownPythonWorkers() {
	if pythonPoolQueue == nil {
		return
	}
	// Drain the pool and kill each worker
	for {
		select {
		case worker := <-pythonPoolQueue:
			if worker.stdin != nil {
				worker.stdin.Close()
			}
			if worker.cmd != nil && worker.cmd.Process != nil {
				worker.cmd.Process.Kill()
				worker.cmd.Wait()
			}
		default:
			return
		}
	}
}

// JSON native support
func JSONParse(dataVal interface{}) interface{} {
	data := fmt.Sprint(dataVal)
	var val interface{}
	err := json.Unmarshal([]byte(data), &val)
	if err != nil {
		panic(fmt.Sprintf("JSON parse error: %s", err.Error()))
	}
	return ToSafeValue(val)
}

func JSONStringify(val interface{}) string {
	b, err := json.Marshal(val)
	if err != nil {
		panic(fmt.Sprintf("JSON stringify error: %s", err.Error()))
	}
	return string(b)
}

// Rate Limiting Semaphores
func AcquireSemaphore(id string, limit int) {
	semaphoresMu.Lock()
	sem, exists := semaphores[id]
	if !exists {
		sem = make(chan struct{}, limit)
		semaphores[id] = sem
	}
	semaphoresMu.Unlock()

	sem <- struct{}{}
}

func ReleaseSemaphore(id string) {
	semaphoresMu.Lock()
	sem, exists := semaphores[id]
	semaphoresMu.Unlock()
	if exists {
		<-sem
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
		return stmt, nil
	}

	stmtCacheMu.Lock()
	defer stmtCacheMu.Unlock()
	if stmt, exists = stmtCache[query]; exists {
		return stmt, nil
	}

	stmt, err := dbInstance.Prepare(query)
	if err != nil {
		return nil, err
	}

	// LRU eviction: if cache is full, close and remove the oldest entry
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

func HTTPGetSafe(url string) interface{} {
	var result interface{}
	var errVal interface{}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				errVal = fmt.Sprint(rec)
			}
		}()
		result = HTTPGet(url)
	}()
	return [2]interface{}{result, errVal}
}

func HTTPPostSafe(url string, body interface{}) interface{} {
	var result interface{}
	var errVal interface{}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				errVal = fmt.Sprint(rec)
			}
		}()
		result = HTTPPost(url, body)
	}()
	return [2]interface{}{result, errVal}
}

func JSONParseSafe(dataVal interface{}) interface{} {
	var result interface{}
	var errVal interface{}
	func() {
		defer func() {
			if rec := recover(); rec != nil {
				errVal = fmt.Sprint(rec)
			}
		}()
		result = JSONParse(dataVal)
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

// SafeMap implements a thread-safe map using a sync.RWMutex
type SafeMap struct {
	mu sync.RWMutex
	m  map[string]interface{}
}

func NewSafeMap() *SafeMap {
	return &SafeMap{m: make(map[string]interface{})}
}

func NewSafeMapFromMap(m map[string]interface{}) *SafeMap {
	if m == nil {
		m = make(map[string]interface{})
	}
	return &SafeMap{m: m}
}

func (s *SafeMap) Set(key string, val interface{}) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.m[key] = val
}

func (s *SafeMap) Get(key string) interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return s.m[key]
}

func (s *SafeMap) Delete(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	delete(s.m, key)
}

func (s *SafeMap) MarshalJSON() ([]byte, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return json.Marshal(s.m)
}

func ToSafeValue(val interface{}) interface{} {
	switch v := val.(type) {
	case map[string]interface{}:
		sm := NewSafeMap()
		for k, valItem := range v {
			sm.Set(k, ToSafeValue(valItem))
		}
		return sm
	case []interface{}:
		res := make([]interface{}, len(v))
		for i, valItem := range v {
			res[i] = ToSafeValue(valItem)
		}
		return res
	default:
		return v
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
