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
	"reflect"
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
	logStructuredWithFields(level, nil, args...)
}

func logStructuredWithFields(level string, fields map[string]interface{}, args ...interface{}) {
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
		for k, v := range fields {
			entry[k] = v
		}
		b, _ := json.Marshal(entry)
		fmt.Println(string(b))
	} else {
		if len(fields) > 0 {
			var pairs []string
			for k, v := range fields {
				pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
			}
			log.Printf("[%s] %s %s", strings.ToUpper(level), msg, strings.Join(pairs, " "))
		} else {
			log.Printf("[%s] %s", strings.ToUpper(level), msg)
		}
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

// ContextLogger holds pre-set fields and emits them with every log call.
// Usage from Serv: let logger = log.with("request_id", id, "service", "auth")
//                  logger.info("request processed")
type ContextLogger struct {
	Fields map[string]interface{}
}

// Info logs at info level with the logger's context fields.
func (cl *ContextLogger) Info(args ...interface{}) interface{} {
	logStructuredWithFields("info", cl.Fields, args...)
	return nil
}

// Warn logs at warn level with the logger's context fields.
func (cl *ContextLogger) Warn(args ...interface{}) interface{} {
	logStructuredWithFields("warn", cl.Fields, args...)
	return nil
}

// Error logs at error level with the logger's context fields.
func (cl *ContextLogger) Error(args ...interface{}) interface{} {
	logStructuredWithFields("error", cl.Fields, args...)
	return nil
}

// Debug logs at debug level with the logger's context fields.
func (cl *ContextLogger) Debug(args ...interface{}) interface{} {
	logStructuredWithFields("debug", cl.Fields, args...)
	return nil
}

// With returns a new ContextLogger with additional fields merged in.
func (cl *ContextLogger) With(args ...interface{}) *ContextLogger {
	merged := make(map[string]interface{})
	for k, v := range cl.Fields {
		merged[k] = v
	}
	for i := 0; i+1 < len(args); i += 2 {
		merged[fmt.Sprint(args[i])] = args[i+1]
	}
	return &ContextLogger{Fields: merged}
}

// LogWith creates a ContextLogger with the given key-value pairs.
// Usage from Serv:
//   log.with("user_id", 123, "action", "login")            — logs at info with fields (legacy)
//   let logger = log.with("request_id", id)                 — returns a reusable logger
//   logger.info("handled request")
func LogWith(args ...interface{}) interface{} {
	fields := make(map[string]interface{})
	for i := 0; i+1 < len(args); i += 2 {
		fields[fmt.Sprint(args[i])] = args[i+1]
	}
	// If odd number of args, last arg is a message — log immediately (legacy behavior)
	if len(args)%2 == 1 {
		msg := fmt.Sprint(args[len(args)-1])
		logStructuredWithFields("info", fields, msg)
		return nil
	}
	// Even number of args: return a ContextLogger for chaining
	return &ContextLogger{Fields: fields}
}

// LogFields creates a ContextLogger from a map of fields.
// Usage from Serv: let logger = log.fields({ request_id: id, service: "auth" })
//                  logger.info("ready")
func LogFields(args ...interface{}) interface{} {
	fields := make(map[string]interface{})
	if len(args) == 1 {
		switch m := args[0].(type) {
		case map[string]interface{}:
			fields = m
		case *SafeMap:
			for k, v := range m.All() {
				fields[k] = v
			}
		}
	}
	return &ContextLogger{Fields: fields}
}

// LogSetLevel changes the runtime log level.
// Usage from Serv: log.setLevel("debug")
func LogSetLevel(args ...interface{}) interface{} {
	if len(args) == 0 {
		return nil
	}
	lvl := strings.ToLower(fmt.Sprint(args[0]))
	switch lvl {
	case "debug", "info", "warn", "error":
		logLevelMu.Lock()
		logLevel = lvl
		logLevelMu.Unlock()
	}
	return nil
}

// LogGetLevel returns the current log level.
// Usage from Serv: let level = log.getLevel()
func LogGetLevel(args ...interface{}) interface{} {
	logLevelMu.RLock()
	defer logLevelMu.RUnlock()
	return logLevel
}

// ContextLoggerInfo calls .Info() on a ContextLogger value.
// Used when codegen encounters: logger.info("msg")
func ContextLoggerInfo(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.Info(args...)
	}
	// Fallback: just log normally
	logStructured("info", args...)
	return nil
}

// ContextLoggerWarn calls .Warn() on a ContextLogger value.
func ContextLoggerWarn(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.Warn(args...)
	}
	logStructured("warn", args...)
	return nil
}

// ContextLoggerError calls .Error() on a ContextLogger value.
func ContextLoggerError(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.Error(args...)
	}
	logStructured("error", args...)
	return nil
}

// ContextLoggerDebug calls .Debug() on a ContextLogger value.
func ContextLoggerDebug(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.Debug(args...)
	}
	logStructured("debug", args...)
	return nil
}

// ContextLoggerWith calls .With() on a ContextLogger value to add more fields.
func ContextLoggerWith(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.With(args...)
	}
	// If not a ContextLogger, create a new one
	return LogWith(args...)
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
	endSpan := TraceHTTPClient("GET", url)
	start := time.Now()
	MetricInc("http_client_requests_total")
	resp, err := http.Get(url)
	if err != nil {
		MetricInc("http_client_errors_total")
		endSpan(0)
		panic(fmt.Sprintf("HTTP GET request failed for %s: %s", url, err.Error()))
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)

	duration := time.Since(start).Seconds()
	MetricGauge("http_client_request_duration_seconds", duration)

	endSpan(resp.StatusCode)
	return HTTPResponse{Status: resp.StatusCode, Body: string(body)}
}

func HTTPPost(url string, body interface{}) HTTPResponse {
	endSpan := TraceHTTPClient("POST", url)
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
		endSpan(0)
		panic(fmt.Sprintf("HTTP POST request failed for %s: %s", url, err.Error()))
	}
	defer resp.Body.Close()
	bodyBytes, _ := io.ReadAll(resp.Body)

	duration := time.Since(start).Seconds()
	MetricGauge("http_client_request_duration_seconds", duration)

	endSpan(resp.StatusCode)
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
			endSpan := TraceScheduler("Every", intervalStr)
			func() {
				defer func() {
					if r := recover(); r != nil {
						MetricInc("scheduler_jobs_failed_total")
						LogError("Recovered in every job: ", r)
					}
				}()
				callback()
			}()
			endSpan()
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
		endSpan := TraceScheduler("Cron", cronExpr)
		func() {
			defer func() {
				if r := recover(); r != nil {
					MetricInc("scheduler_jobs_failed_total")
					LogError("Recovered in cron job: ", r)
				}
			}()
			callback()
		}()
		endSpan()
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
	endSpan := TraceExtern("python:"+scriptPath, funcName)
	defer endSpan()

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

// Atomic operations — thread-safe counters and values

type AtomicValue struct {
	mu    sync.RWMutex
	value interface{}
}

var (
	atomicValues   = make(map[string]*AtomicValue)
	atomicValuesMu sync.Mutex
)

func getOrCreateAtomic(name string) *AtomicValue {
	atomicValuesMu.Lock()
	defer atomicValuesMu.Unlock()
	if av, ok := atomicValues[name]; ok {
		return av
	}
	av := &AtomicValue{value: 0}
	atomicValues[name] = av
	return av
}

// AtomicNew creates a new atomic value with an initial value.
func AtomicNew(name interface{}, initial interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	av.value = initial
	av.mu.Unlock()
	return nil
}

// AtomicInc increments an atomic counter by 1 and returns the new value.
func AtomicInc(name interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	defer av.mu.Unlock()
	switch v := av.value.(type) {
	case int:
		av.value = v + 1
		return av.value
	case int64:
		av.value = v + 1
		return av.value
	default:
		av.value = 1
		return 1
	}
}

// AtomicDec decrements an atomic counter by 1 and returns the new value.
func AtomicDec(name interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	defer av.mu.Unlock()
	switch v := av.value.(type) {
	case int:
		av.value = v - 1
		return av.value
	case int64:
		av.value = v - 1
		return av.value
	default:
		av.value = -1
		return -1
	}
}

// AtomicGet returns the current value of an atomic.
func AtomicGet(name interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.RLock()
	defer av.mu.RUnlock()
	return av.value
}

// AtomicSet sets the value of an atomic.
func AtomicSet(name interface{}, value interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	av.value = value
	av.mu.Unlock()
	return nil
}

// AtomicCAS performs a compare-and-swap. Returns true if swapped.
func AtomicCAS(name interface{}, expected interface{}, newValue interface{}) interface{} {
	key := fmt.Sprint(name)
	av := getOrCreateAtomic(key)
	av.mu.Lock()
	defer av.mu.Unlock()
	if fmt.Sprint(av.value) == fmt.Sprint(expected) {
		av.value = newValue
		return true
	}
	return false
}

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

// Registry — generic named function map for dynamic dispatch.
// Supports registering functions by name and calling them dynamically.
// Use cases: job schedulers, event handlers, plugin systems, command dispatch.

var (
	registryFuncs   = make(map[string]interface{})
	registryFuncsMu sync.RWMutex
)

// RegistrySet registers a function by name.
// Usage: registry.set("batch_processing", executeBatchProcessing)
func RegistrySet(name interface{}, handler interface{}) interface{} {
	key := fmt.Sprint(name)
	registryFuncsMu.Lock()
	registryFuncs[key] = handler
	registryFuncsMu.Unlock()
	LogInfo("Registry: registered handler '", key, "'")
	return nil
}

// RegistryCall invokes a registered function by name with the given arguments.
// Usage: registry.call("batch_processing", payload, idempotencyKey)
func RegistryCall(name interface{}, args ...interface{}) interface{} {
	key := fmt.Sprint(name)
	registryFuncsMu.RLock()
	handler, exists := registryFuncs[key]
	registryFuncsMu.RUnlock()

	if !exists {
		LogError("Registry: handler not found: '", key, "'")
		return nil
	}

	// Call the handler based on its type
	switch fn := handler.(type) {
	case func(interface{}) interface{}:
		if len(args) >= 1 {
			return fn(args[0])
		}
		return fn(nil)
	case func(interface{}, interface{}) interface{}:
		var a, b interface{}
		if len(args) >= 1 {
			a = args[0]
		}
		if len(args) >= 2 {
			b = args[1]
		}
		return fn(a, b)
	case func(interface{}, interface{}, interface{}) interface{}:
		var a, b, c interface{}
		if len(args) >= 1 {
			a = args[0]
		}
		if len(args) >= 2 {
			b = args[1]
		}
		if len(args) >= 3 {
			c = args[2]
		}
		return fn(a, b, c)
	default:
		LogError("Registry: handler '", key, "' has unsupported signature")
		return nil
	}
}

// RegistryList returns all registered handler names.
// Usage: let handlers = registry.list()
func RegistryList() interface{} {
	registryFuncsMu.RLock()
	defer registryFuncsMu.RUnlock()
	names := make([]interface{}, 0, len(registryFuncs))
	for k := range registryFuncs {
		names = append(names, k)
	}
	return names
}

// RegistryHas checks if a handler is registered.
// Usage: let exists = registry.has("batch_processing")
func RegistryHas(name interface{}) interface{} {
	key := fmt.Sprint(name)
	registryFuncsMu.RLock()
	_, exists := registryFuncs[key]
	registryFuncsMu.RUnlock()
	return exists
}

// CronNext computes the next execution time for a cron expression.
// Returns Unix timestamp (seconds) of the next occurrence.
// Usage: let nextTime = cron.next("0 */30 * * *")
func CronNext(cronExpr interface{}) interface{} {
	expr := fmt.Sprint(cronExpr)
	fields := strings.Fields(expr)
	var schedule cron.Schedule
	var err error

	if len(fields) == 6 {
		parser6 := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err = parser6.Parse(expr)
	} else {
		parser5 := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err = parser5.Parse(expr)
	}

	if err != nil {
		LogError("CronNext: invalid cron expression '", expr, "': ", err.Error())
		return 0
	}
	next := schedule.Next(time.Now())
	return next.Unix()
}

// CronSleepUntilNext sleeps until the next occurrence of the cron expression.
// Returns the Unix timestamp when it woke up.
// Supports both 5-field (min hour dom month dow) and 6-field (sec min hour dom month dow).
// Usage: cron.sleepUntilNext("0 */30 * * *")
func CronSleepUntilNext(cronExpr interface{}) interface{} {
	expr := fmt.Sprint(cronExpr)

	// Count fields to determine format
	fields := strings.Fields(expr)
	var schedule cron.Schedule
	var err error

	if len(fields) == 6 {
		// 6-field: second minute hour dom month dow
		parser6 := cron.NewParser(cron.Second | cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err = parser6.Parse(expr)
	} else {
		// 5-field: minute hour dom month dow
		parser5 := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow)
		schedule, err = parser5.Parse(expr)
	}

	if err != nil {
		LogError("CronSleepUntilNext: invalid cron expression '", expr, "': ", err.Error())
		time.Sleep(60 * time.Second)
		return time.Now().Unix()
	}

	next := schedule.Next(time.Now())
	sleepDuration := time.Until(next)
	LogDebug("CronSleepUntilNext: expr='", expr, "' next=", next.Format(time.RFC3339), " sleeping ", sleepDuration.String())
	if sleepDuration > 0 {
		time.Sleep(sleepDuration)
	}
	return time.Now().Unix()
}

// SpawnWithTimeout runs a function with a timeout. Returns result or nil on timeout.
func SpawnWithTimeout(timeoutMs interface{}, fn func() interface{}) interface{} {
	timeout := time.Duration(toInt(timeoutMs)) * time.Millisecond
	ch := make(chan interface{}, 1)
	go func() {
		defer func() {
			if r := recover(); r != nil {
				ch <- nil
			}
		}()
		ch <- fn()
	}()
	select {
	case result := <-ch:
		return result
	case <-time.After(timeout):
		return nil
	}
}

// Channel operations — Go channels exposed to Serv

// ChannelNew creates a buffered channel with the given capacity.
// Usage: let ch = channel.new(100)
func ChannelNew(capacity interface{}) interface{} {
	cap := toInt(capacity)
	if cap <= 0 {
		cap = 1
	}
	return make(chan interface{}, cap)
}

// ChannelSend sends a value to a channel. Blocks if channel is full.
// Usage: channel.send(ch, value)
func ChannelSend(ch interface{}, value interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		c <- value
	}
	return nil
}

// ChannelReceive receives a value from a channel. Blocks until a value is available.
// Usage: let value = channel.receive(ch)
func ChannelReceive(ch interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		val, ok := <-c
		if !ok {
			return nil // channel closed
		}
		return val
	}
	return nil
}

// ChannelTryReceive attempts to receive without blocking. Returns nil if nothing available.
// Usage: let value = channel.tryReceive(ch)
func ChannelTryReceive(ch interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		select {
		case val, ok := <-c:
			if !ok {
				return nil
			}
			return val
		default:
			return nil
		}
	}
	return nil
}

// ChannelTrySend attempts to send without blocking. Returns true if sent, false if full.
// Usage: let sent = channel.trySend(ch, value)
func ChannelTrySend(ch interface{}, value interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		select {
		case c <- value:
			return true
		default:
			return false
		}
	}
	return false
}

// ChannelClose closes a channel. Receivers will get nil after all buffered values are consumed.
// Usage: channel.close(ch)
func ChannelClose(ch interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		close(c)
	}
	return nil
}

// ChannelLen returns the number of elements currently buffered in the channel.
// Usage: let pending = channel.len(ch)
func ChannelLen(ch interface{}) interface{} {
	if c, ok := ch.(chan interface{}); ok {
		return len(c)
	}
	return 0
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

// All returns a copy of all key-value pairs in the SafeMap.
func (s *SafeMap) All() map[string]interface{} {
	s.mu.RLock()
	defer s.mu.RUnlock()
	cp := make(map[string]interface{}, len(s.m))
	for k, v := range s.m {
		cp[k] = v
	}
	return cp
}

// GetField extracts a named field from a value using reflection.
// Supports struct pointer fields (capitalized) and falls back to lowercase matches.
// Used by destructuring: let { name, email } = user
func GetField(obj interface{}, field string) interface{} {
	if obj == nil {
		return nil
	}
	// Try SafeMap
	if sm, ok := obj.(*SafeMap); ok {
		return sm.Get(field)
	}
	// Try plain map
	if m, ok := obj.(map[string]interface{}); ok {
		return m[field]
	}
	// Use reflection for struct field access
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	// Try capitalized field name first (Go convention)
	capitalized := strings.ToUpper(field[:1]) + field[1:]
	f := v.FieldByName(capitalized)
	if f.IsValid() {
		return f.Interface()
	}
	// Try exact name
	f = v.FieldByName(field)
	if f.IsValid() {
		return f.Interface()
	}
	return nil
}

// --- Performance-optimized runtime helpers ---
// These replace inline closures in generated code for better performance.

// Equal compares two interface{} values for equality without fmt.Sprintf allocation.
func Equal(a, b interface{}) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok { return av == bv }
		if bv, ok := b.(int64); ok { return int64(av) == bv }
		if bv, ok := b.(float64); ok { return float64(av) == bv }
	case int64:
		if bv, ok := b.(int64); ok { return av == bv }
		if bv, ok := b.(int); ok { return av == int64(bv) }
	case float64:
		if bv, ok := b.(float64); ok { return av == bv }
		if bv, ok := b.(int); ok { return av == float64(bv) }
	case string:
		if bv, ok := b.(string); ok { return av == bv }
	case bool:
		if bv, ok := b.(bool); ok { return av == bv }
	}
	// Fallback: compare string representations
	return fmt.Sprint(a) == fmt.Sprint(b)
}

// Compare performs ordered comparison (<, >, <=, >=) on two interface{} values.
func Compare(a, b interface{}, op string) bool {
	switch av := a.(type) {
	case int:
		bv, ok := b.(int)
		if !ok { return false }
		switch op {
		case "<": return av < bv
		case ">": return av > bv
		case "<=": return av <= bv
		case ">=": return av >= bv
		}
	case int64:
		bv, ok := b.(int64)
		if !ok { return false }
		switch op {
		case "<": return av < bv
		case ">": return av > bv
		case "<=": return av <= bv
		case ">=": return av >= bv
		}
	case float64:
		bv, ok := b.(float64)
		if !ok { return false }
		switch op {
		case "<": return av < bv
		case ">": return av > bv
		case "<=": return av <= bv
		case ">=": return av >= bv
		}
	case string:
		bv, ok := b.(string)
		if !ok { return false }
		switch op {
		case "<": return av < bv
		case ">": return av > bv
		case "<=": return av <= bv
		case ">=": return av >= bv
		}
	}
	return false
}

// Arith performs arithmetic on two interface{} values.
func Arith(a, b interface{}, op string) interface{} {
	switch av := a.(type) {
	case int:
		if bv, ok := b.(int); ok {
			switch op {
			case "+": return av + bv
			case "-": return av - bv
			case "*": return av * bv
			case "/": if bv != 0 { return av / bv }
			}
		}
		if bv, ok := b.(float64); ok {
			switch op {
			case "+": return float64(av) + bv
			case "-": return float64(av) - bv
			case "*": return float64(av) * bv
			case "/": if bv != 0 { return float64(av) / bv }
			}
		}
	case int64:
		if bv, ok := b.(int64); ok {
			switch op {
			case "+": return av + bv
			case "-": return av - bv
			case "*": return av * bv
			case "/": if bv != 0 { return av / bv }
			}
		}
	case float64:
		if bv, ok := b.(float64); ok {
			switch op {
			case "+": return av + bv
			case "-": return av - bv
			case "*": return av * bv
			case "/": if bv != 0 { return av / bv }
			}
		}
		if bv, ok := b.(int); ok {
			switch op {
			case "+": return av + float64(bv)
			case "-": return av - float64(bv)
			case "*": return av * float64(bv)
			case "/": if bv != 0 { return av / float64(bv) }
			}
		}
	case string:
		if op == "+" {
			if bv, ok := b.(string); ok { return av + bv }
			return av + fmt.Sprint(b)
		}
	}
	return nil
}

// MemberAccess retrieves a field from a dynamic object (Request, SafeMap, map, struct).
func MemberAccess(obj interface{}, field string) interface{} {
	if obj == nil {
		return nil
	}
	switch v := obj.(type) {
	case Request:
		switch field {
		case "body", "Body": return v.Body
		case "method", "Method": return v.Method
		case "path", "Path": return v.Path
		case "params", "Params": return v.Params
		}
	case HTTPResponse:
		switch field {
		case "body", "Body": return v.Body
		case "status", "Status": return v.Status
		}
	case *SafeMap:
		return v.Get(field)
	case map[string]interface{}:
		return v[field]
	default:
		return GetField(obj, field)
	}
	return nil
}

// MergeMaps merges multiple maps into a single map[string]interface{}.
// Later maps override earlier ones. Supports map[string]interface{}, *SafeMap, and interface{} values.
// Usage from Serv: { ...defaults, ...overrides, extra: "value" }
func MergeMaps(maps ...interface{}) map[string]interface{} {
	result := make(map[string]interface{})
	for _, m := range maps {
		switch v := m.(type) {
		case map[string]interface{}:
			for k, val := range v {
				result[k] = val
			}
		case *SafeMap:
			for k, val := range v.All() {
				result[k] = val
			}
		}
	}
	return result
}

// ValidateConfig checks that all required config keys are present at startup.
// If any key is missing, it logs an error and exits the process immediately.
// Usage from Serv: validate { required "db.host", required "db.port" }
func ValidateConfig(requiredKeys []string) {
	var missing []string
	for _, key := range requiredKeys {
		val := Config(key)
		if val == "" {
			// Also check environment variable equivalent: db.host -> DB_HOST
			envKey := strings.ToUpper(strings.ReplaceAll(key, ".", "_"))
			if os.Getenv(envKey) == "" {
				missing = append(missing, key)
			}
		}
	}
	if len(missing) > 0 {
		log.Fatalf("[FATAL] Config validation failed. Missing required keys: %v\n"+
			"Set them via config file or environment variables (e.g., %s -> %s)",
			missing, missing[0], strings.ToUpper(strings.ReplaceAll(missing[0], ".", "_")))
	}
}

// ValidateBody validates a JSON request body against a schema map.
// Schema values define rules: "required", "string", "int", "float", "bool", "email", "required,email"
// Returns nil if valid, or a []interface{} of error message strings.
// Usage from Serv: let errors = validate(req.body, { name: "required", email: "required,email", age: "int" })
func ValidateBody(args ...interface{}) interface{} {
	if len(args) < 2 {
		return []interface{}{"validate requires (body, schema) arguments"}
	}

	// Parse body
	bodyStr := fmt.Sprint(args[0])
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
		return []interface{}{"invalid JSON body: " + err.Error()}
	}

	// Parse schema
	schema := make(map[string]string)
	switch s := args[1].(type) {
	case map[string]interface{}:
		for k, v := range s {
			schema[k] = fmt.Sprint(v)
		}
	case *SafeMap:
		for k, v := range s.All() {
			schema[k] = fmt.Sprint(v)
		}
	default:
		return []interface{}{"schema must be a map"}
	}

	var errors []interface{}
	for field, rules := range schema {
		ruleList := strings.Split(rules, ",")
		val, exists := body[field]

		for _, rule := range ruleList {
			rule = strings.TrimSpace(rule)
			switch rule {
			case "required":
				if !exists || val == nil || val == "" {
					errors = append(errors, fmt.Sprintf("%s is required", field))
				}
			case "string":
				if exists && val != nil {
					if _, ok := val.(string); !ok {
						errors = append(errors, fmt.Sprintf("%s must be a string", field))
					}
				}
			case "int":
				if exists && val != nil {
					switch val.(type) {
					case float64: // JSON numbers are float64
						// ok
					case int, int64:
						// ok
					default:
						errors = append(errors, fmt.Sprintf("%s must be an integer", field))
					}
				}
			case "float":
				if exists && val != nil {
					if _, ok := val.(float64); !ok {
						errors = append(errors, fmt.Sprintf("%s must be a number", field))
					}
				}
			case "bool":
				if exists && val != nil {
					if _, ok := val.(bool); !ok {
						errors = append(errors, fmt.Sprintf("%s must be a boolean", field))
					}
				}
			case "email":
				if exists && val != nil {
					s := fmt.Sprint(val)
					if !strings.Contains(s, "@") || !strings.Contains(s, ".") {
						errors = append(errors, fmt.Sprintf("%s must be a valid email", field))
					}
				}
			}
		}
	}

	if len(errors) == 0 {
		return nil
	}
	return errors
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
