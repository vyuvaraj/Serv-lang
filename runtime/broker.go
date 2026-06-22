//go:build !wasm

package runtime

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"

	mqtt "github.com/eclipse/paho.mqtt.golang"
	"github.com/go-stomp/stomp/v3"
	"github.com/nats-io/nats.go"
	amqp "github.com/rabbitmq/amqp091-go"
	"github.com/redis/go-redis/v9"
	"github.com/segmentio/kafka-go"
)

// Broker global state
var (
	brokerURL string

	// Broker Connection Instances
	natsClient      *nats.Conn
	mqttConn        mqtt.Client
	amqpConn        *amqp.Connection
	amqpChan        *amqp.Channel
	kafkaBrokerAddr string
	kafkaWriterMap  = make(map[string]*kafka.Writer)
	kafkaWriterMu   sync.Mutex
	stompConn       *stomp.Conn
	redisBrokerClient     *redis.Client

	// Fallback In-memory Broker
	subscribers   = make(map[string][]func(string))
	subscribersMu sync.RWMutex

	pubSubQueueSize  = 10000
	pubSubWorkers    = 20
	pubSubQueue      chan pubSubEvent
	pubSubWorkerOnce sync.Once
)

type pubSubEvent struct {
	callback    func(string)
	payload     string
	traceparent string
}

type BrokerEnvelope struct {
	Traceparent string `json:"_traceparent,omitempty"`
	Payload     string `json:"payload"`
}

func extractTraceAndPayload(msgData string) (string, string) {
	var env BrokerEnvelope
	if strings.HasPrefix(msgData, "{") && strings.Contains(msgData, `"payload"`) {
		if err := json.Unmarshal([]byte(msgData), &env); err == nil {
			return env.Traceparent, env.Payload
		}
	}
	return "", msgData
}

func handleIncomingMessage(traceparentHeader string, rawPayload string, callback func(string), topic string) {
	tp, payload := extractTraceAndPayload(rawPayload)
	if tp == "" && traceparentHeader != "" {
		tp = traceparentHeader
	}

	var trace *RequestTrace
	if tp != "" {
		trace = TraceRequest("Subscribe", topic, tp)
	} else {
		trace = TraceRequest("Subscribe", topic, "")
	}
	SetActiveTrace(trace)
	defer ClearActiveTrace()
	defer EndTrace(trace, 200)

	endSpan := TracePubSub("Subscribe", topic)
	defer endSpan()

	executeCallbackSafe(callback, payload)
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
	} else if strings.HasPrefix(url, "activemq://") || strings.HasPrefix(url, "stomp://") || strings.HasPrefix(url, "servqueue://") {
		addr := url
		var login, passcode string

		if strings.HasPrefix(url, "servqueue://") {
			addr = strings.TrimPrefix(url, "servqueue://")
			// Parse username:password if present
			if strings.Contains(addr, "@") {
				parts := strings.SplitN(addr, "@", 2)
				creds := parts[0]
				addr = parts[1]
				if strings.Contains(creds, ":") {
					credParts := strings.SplitN(creds, ":", 2)
					login = credParts[0]
					passcode = credParts[1]
				} else {
					login = creds
				}
			}
		} else {
			addr = strings.TrimPrefix(strings.TrimPrefix(url, "activemq://"), "stomp://")
		}

		var err error
		var opts []func(*stomp.Conn) error
		if login != "" || passcode != "" {
			opts = append(opts, stomp.ConnOpt.Login(login, passcode))
		}

		stompConn, err = stomp.Dial("tcp", addr, opts...)
		if err != nil {
			LogWarn("Failed to connect to STOMP/ServQueue broker: ", err, " - Falling back to in-memory broker")
		} else {
			LogInfo("Connected to STOMP/ServQueue broker successfully at ", addr)
		}
	} else if strings.HasPrefix(url, "redis://") || strings.HasPrefix(url, "redis-stream://") {
		addr := url
		if strings.HasPrefix(url, "redis://") {
			addr = strings.TrimPrefix(url, "redis://")
		} else {
			addr = strings.TrimPrefix(url, "redis-stream://")
		}
		var password string
		if strings.Contains(addr, "@") {
			parts := strings.SplitN(addr, "@", 2)
			creds := parts[0]
			addr = parts[1]
			if strings.Contains(creds, ":") {
				password = strings.SplitN(creds, ":", 2)[1]
			} else {
				password = creds
			}
		}
		redisBrokerClient = redis.NewClient(&redis.Options{
			Addr:     addr,
			Password: password,
			DB:       0,
		})
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		defer cancel()
		if err := redisBrokerClient.Ping(ctx).Err(); err != nil {
			LogWarn("Failed to connect to Redis broker: ", err, " - Falling back to in-memory broker")
			redisBrokerClient = nil
		} else {
			LogInfo("Connected to Redis broker successfully at ", addr)
		}
	}
}

func Subscribe(topic string, callback func(string)) {
	LogInfo("Registering subscription for topic: ", topic)

	if natsClient != nil {
		_, err := natsClient.Subscribe(topic, func(m *nats.Msg) {
			var tp string
			if m.Header != nil {
				tp = m.Header.Get("traceparent")
			}
			handleIncomingMessage(tp, string(m.Data), callback, topic)
		})
		if err == nil {
			return
		}
	}

	if mqttConn != nil {
		token := mqttConn.Subscribe(topic, 0, func(client mqtt.Client, msg mqtt.Message) {
			handleIncomingMessage("", string(msg.Payload()), callback, topic)
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
					var tp string
					if d.Headers != nil {
						if val, ok := d.Headers["traceparent"].(string); ok {
							tp = val
						}
					}
					handleIncomingMessage(tp, string(d.Body), callback, topic)
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
				var tp string
				for _, h := range m.Headers {
					if h.Key == "traceparent" {
						tp = string(h.Value)
						break
					}
				}
				handleIncomingMessage(tp, string(m.Value), callback, topic)
			}
		}()
		return
	}

	if stompConn != nil {
		if strings.HasPrefix(brokerURL, "servqueue://") {
			go RegisterServQueueDLQ(topic, topic+".dlq")
		}

		sub, err := stompConn.Subscribe(topic, stomp.AckAuto)
		if err == nil {
			go func() {
				defer sub.Unsubscribe()
				for {
					msg := <-sub.C
					if msg.Err != nil {
						break
					}
					var tp string
					if msg.Header != nil {
						tp = msg.Header.Get("traceparent")
					}
					handleIncomingMessage(tp, string(msg.Body), callback, topic)
				}
			}()
			return
		}
	}

	if redisBrokerClient != nil {
		go func() {
			lastID := "$"
			for {
				ctx := context.Background()
				streams, err := redisBrokerClient.XRead(ctx, &redis.XReadArgs{
					Streams: []string{topic, lastID},
					Count:   10,
					Block:   0,
				}).Result()

				if err != nil {
					time.Sleep(1 * time.Second)
					continue
				}

				for _, stream := range streams {
					for _, msg := range stream.Messages {
						lastID = msg.ID
						payloadVal, _ := msg.Values["payload"].(string)
						traceparentVal, _ := msg.Values["traceparent"].(string)
						handleIncomingMessage(traceparentVal, payloadVal, callback, topic)
					}
				}
			}
		}()
		return
	}

	// In-memory fallback Pub/Sub
	subscribersMu.Lock()
	subscribers[topic] = append(subscribers[topic], callback)
	subscribersMu.Unlock()
}

func RegisterServQueueDLQ(topic string, dlqTopic string) {
	var queueAddr = "http://localhost:8082"
	if raw := os.Getenv("SERVVERSE_DISCOVERY"); raw != "" {
		var manifest struct {
			Queue string `json:"queue"`
		}
		if json.Unmarshal([]byte(raw), &manifest) == nil && manifest.Queue != "" {
			queueAddr = manifest.Queue
		} else {
			if data, err := os.ReadFile(raw); err == nil {
				if json.Unmarshal(data, &manifest) == nil && manifest.Queue != "" {
					queueAddr = manifest.Queue
				}
			}
		}
	} else {
		addr := brokerURL
		if strings.HasPrefix(addr, "servqueue://") {
			addr = strings.TrimPrefix(addr, "servqueue://")
			if strings.Contains(addr, "@") {
				parts := strings.SplitN(addr, "@", 2)
				addr = parts[1]
			}
			host := addr
			if strings.Contains(addr, ":") {
				host = strings.SplitN(addr, ":", 2)[0]
			}
			queueAddr = "http://" + host + ":8082"
		}
	}

	url := strings.TrimSuffix(queueAddr, "/") + "/api/topics/" + topic + "/dlq"
	
	reqBody, err := json.Marshal(map[string]string{"dlq_topic": dlqTopic})
	if err != nil {
		LogWarn("Failed to marshal DLQ registration payload:", err)
		return
	}

	req, err := http.NewRequest("POST", url, bytes.NewReader(reqBody))
	if err != nil {
		LogWarn("Failed to create HTTP request for DLQ registration:", err)
		return
	}
	req.Header.Set("Content-Type", "application/json")
	
	apiToken := "secret-token"
	if raw := os.Getenv("SERVVERSE_DISCOVERY"); raw != "" {
		var manifest struct {
			AuthToken string `json:"auth_token"`
		}
		if json.Unmarshal([]byte(raw), &manifest) == nil && manifest.AuthToken != "" {
			apiToken = manifest.AuthToken
		}
	}
	req.Header.Set("Authorization", "Bearer "+apiToken)

	client := &http.Client{Timeout: 5 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		LogWarn("Failed to register DLQ on ServQueue via HTTP:", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		bodyBytes, _ := io.ReadAll(resp.Body)
		LogWarn("ServQueue DLQ registration returned status ", resp.StatusCode, ": ", string(bodyBytes))
	} else {
		LogInfo("Successfully registered DLQ ", dlqTopic, " for topic ", topic, " on ServQueue")
	}
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

	var traceparentVal string
	if active := GetActiveTrace(); active != nil {
		traceparentVal = Traceparent(active)
	}

	// 1. NATS Publish
	if natsClient != nil {
		m := &nats.Msg{
			Subject: topic,
			Data:    []byte(msgStr),
		}
		if traceparentVal != "" {
			m.Header = make(nats.Header)
			m.Header.Set("traceparent", traceparentVal)
		}
		if err := natsClient.PublishMsg(m); err == nil {
			return
		}
	}

	// 2. MQTT Publish - wrap payload in BrokerEnvelope if trace is active
	var mqttPayload = msgStr
	if traceparentVal != "" {
		env := BrokerEnvelope{
			Traceparent: traceparentVal,
			Payload:     msgStr,
		}
		if eb, err := json.Marshal(env); err == nil {
			mqttPayload = string(eb)
		}
	}
	if mqttConn != nil {
		token := mqttConn.Publish(topic, 0, false, mqttPayload)
		if token.Wait() && token.Error() == nil {
			return
		}
	}

	// 3. AMQP Publish
	if amqpChan != nil {
		_, err := amqpChan.QueueDeclare(topic, false, false, false, false, nil)
		if err == nil {
			headers := amqp.Table{}
			if traceparentVal != "" {
				headers["traceparent"] = traceparentVal
			}
			amqpChan.PublishWithContext(context.Background(), "", topic, false, false, amqp.Publishing{
				ContentType: "text/plain",
				Body:        []byte(msgStr),
				Headers:     headers,
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
		var headers []kafka.Header
		if traceparentVal != "" {
			headers = append(headers, kafka.Header{
				Key:   "traceparent",
				Value: []byte(traceparentVal),
			})
		}
		if err := w.WriteMessages(context.Background(), kafka.Message{
			Value:   []byte(msgStr),
			Headers: headers,
		}); err == nil {
			return
		}
	}

	// 5. ActiveMQ STOMP Publish
	if stompConn != nil {
		var err error
		if traceparentVal != "" {
			err = stompConn.Send(topic, "text/plain", []byte(msgStr), stomp.SendOpt.Header("traceparent", traceparentVal))
		} else {
			err = stompConn.Send(topic, "text/plain", []byte(msgStr))
		}
		if err == nil {
			return
		}
	}

	// 6. Redis Publish
	if redisBrokerClient != nil {
		values := map[string]interface{}{
			"payload": msgStr,
		}
		if traceparentVal != "" {
			values["traceparent"] = traceparentVal
		}
		ctx := context.Background()
		err := redisBrokerClient.XAdd(ctx, &redis.XAddArgs{
			Stream: topic,
			Values: values,
		}).Err()
		if err == nil {
			return
		}
	}

	// 7. In-memory Fallback
	startPubSubWorkers()
	subscribersMu.RLock()
	subs := subscribers[topic]
	subscribersMu.RUnlock()

	for _, callback := range subs {
		select {
		case pubSubQueue <- pubSubEvent{callback: callback, payload: msgStr, traceparent: traceparentVal}:
		default:
			// If queue is completely full, spawn a temporary goroutine fallback to avoid dropping events
			go handleIncomingMessage(traceparentVal, msgStr, callback, topic)
		}
	}
}

func startPubSubWorkers() {
	pubSubWorkerOnce.Do(func() {
		for i := 0; i < pubSubWorkers; i++ {
			go func() {
				for event := range pubSubQueue {
					handleIncomingMessage(event.traceparent, event.payload, event.callback, "")
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
