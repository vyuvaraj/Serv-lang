package runtime

import (
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
	"strconv"
	"strings"
	"sync"
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
	"github.com/robfig/cron/v3"
	"github.com/segmentio/kafka-go"
)

// Global State
var (
	brokerURL    string
	serverPort   string
	routes       = make(map[string]map[string]func(Request) interface{}) // method -> path -> handler
	routesMu     sync.RWMutex
	cronInstance *cron.Cron
	cronOnce     sync.Once

	// DB Instance
	dbInstance *sql.DB

	// MongoDB Instances
	mongoClient *mongo.Client
	mongoDB     *mongo.Database

	// Cache Instance
	redisClient *redis.Client
	ctx         = context.Background()
	localCache  = make(map[string]localCacheEntry)
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
)

// Noop is a no-op sentinel used by generated test files to satisfy the runtime import.
func Noop() {}

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
func LogInfo(args ...interface{}) {
	log.Printf("[INFO] %s", fmt.Sprint(args...))
}

func LogWarn(args ...interface{}) {
	log.Printf("[WARN] %s", fmt.Sprint(args...))
}

func LogError(args ...interface{}) {
	log.Printf("[ERROR] %s", fmt.Sprint(args...))
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

func AddRoute(method, path string, handler func(Request) interface{}) {
	routesMu.Lock()
	defer routesMu.Unlock()

	if _, ok := routes[method]; !ok {
		routes[method] = make(map[string]func(Request) interface{})
	}
	routes[method][path] = handler
	LogInfo("Registered route: ", method, " ", path)
}

func StartServer() {
	if serverPort == "" {
		serverPort = "2112"
		LogInfo("No server port specified, starting metrics server on default port 2112")
	}

	mux := http.NewServeMux()
	mux.HandleFunc("/metrics", handleMetrics)

	mux.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		routesMu.RLock()
		methodRoutes, exists := routes[r.Method]
		var handler func(Request) interface{}
		if exists {
			handler = methodRoutes[r.URL.Path]
		}
		routesMu.RUnlock()

		if handler == nil {
			http.NotFound(w, r)
			return
		}

		bodyBytes, _ := io.ReadAll(r.Body)
		req := Request{
			Method: r.Method,
			Path:   r.URL.Path,
			Body:   string(bodyBytes),
			Params: make(map[string]string),
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

	LogInfo("Serv service listening on port ", serverPort)
	if err := srv.ListenAndServe(); err != nil && err != http.ErrServerClosed {
		LogError("Web server error: ", err)
	}
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

// Call Python Script for extern mappings
func CallPython(scriptPath string, funcName string, args ...interface{}) interface{} {
	argsJSON, err := json.Marshal(args)
	if err != nil {
		LogError("Failed to marshal python arguments: ", err)
		return nil
	}

	wrapperCode := fmt.Sprintf(`
import sys
import json
import importlib.util

spec = importlib.util.spec_from_file_location("module", %q)
module = importlib.util.module_from_spec(spec)
spec.loader.exec_module(module)

fn = getattr(module, %q)
args = json.loads(%q)

result = fn(*args)
print(json.dumps({"result": result}))
`, scriptPath, funcName, string(argsJSON))

	cmd := exec.Command("python", "-c", wrapperCode)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	err = cmd.Run()
	if err != nil {
		LogError("Python script execution failed: ", stderr.String(), " error: ", err)
		return nil
	}

	var res struct {
		Result interface{} `json:"result"`
	}
	if err := json.Unmarshal(stdout.Bytes(), &res); err != nil {
		return stdout.String()
	}

	return res.Result
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

func DBQuery(query string, args ...interface{}) interface{} {
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

	queryLower := strings.ToLower(strings.TrimSpace(query))
	if strings.HasPrefix(queryLower, "insert") || strings.HasPrefix(queryLower, "update") ||
		strings.HasPrefix(queryLower, "delete") || strings.HasPrefix(queryLower, "create") ||
		strings.HasPrefix(queryLower, "replace") {
		res, err := dbInstance.Exec(query, args...)
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

	rows, err := dbInstance.Query(query, args...)
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
