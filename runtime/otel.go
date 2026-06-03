package runtime

import (
	"bytes"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"math/rand"
	"net/http"
	"os"
	"strings"
	"sync"
	"time"
)

// OpenTelemetry support for Serv services.
// Enabled by setting OTEL_ENDPOINT or otel.endpoint config.
// Exports traces and metrics via OTLP/HTTP (JSON).

var (
	otelEnabled  bool
	otelEndpoint string
	otelService  string
	otelMu       sync.RWMutex

	// Batch spans for export
	spanBuffer   []otelSpan
	spanBufferMu sync.Mutex
	spanFlushInterval = 5 * time.Second
	maxBatchSize = 100
)

// otelSpan represents a single trace span.
type otelSpan struct {
	TraceID    string                 `json:"traceId"`
	SpanID     string                 `json:"spanId"`
	ParentID   string                 `json:"parentSpanId,omitempty"`
	Name       string                 `json:"name"`
	Kind       int                    `json:"kind"` // 1=internal, 2=server, 3=client
	StartTime  int64                  `json:"startTimeUnixNano"`
	EndTime    int64                  `json:"endTimeUnixNano"`
	Attributes map[string]interface{} `json:"attributes,omitempty"`
	Status     int                    `json:"status"` // 0=unset, 1=ok, 2=error
}

func initOtel() {
	endpoint := os.Getenv("OTEL_ENDPOINT")
	if endpoint == "" {
		endpoint = Config("otel.endpoint")
	}
	if endpoint == "" {
		return
	}

	otelEndpoint = strings.TrimSuffix(endpoint, "/")
	otelEnabled = true

	otelService = os.Getenv("OTEL_SERVICE_NAME")
	if otelService == "" {
		otelService = Config("otel.service")
	}
	if otelService == "" {
		otelService = "serv-service"
	}

	// Start background span flusher
	go otelFlushLoop()

	LogInfo("OpenTelemetry enabled: endpoint=", otelEndpoint, " service=", otelService)
}

// OtelEnabled returns whether OpenTelemetry tracing is active.
func OtelEnabled() bool {
	return otelEnabled
}

// TraceRequest creates a span for an HTTP request and returns trace context.
// Called automatically by the HTTP handler for every route.
func TraceRequest(method, path string, parentTrace string) *RequestTrace {
	if !otelEnabled {
		return &RequestTrace{StartTime: time.Now()}
	}

	traceID := parentTrace
	parentSpan := ""
	if traceID == "" {
		traceID = generateTraceID()
	} else {
		// Parse W3C traceparent: 00-traceId-parentSpanId-flags
		parts := strings.Split(parentTrace, "-")
		if len(parts) >= 4 {
			traceID = parts[1]
			parentSpan = parts[2]
		}
	}

	spanID := generateSpanID()

	return &RequestTrace{
		TraceID:   traceID,
		SpanID:    spanID,
		ParentID:  parentSpan,
		Method:    method,
		Path:      path,
		StartTime: time.Now(),
	}
}

// EndTrace completes a request trace span and queues it for export.
func EndTrace(rt *RequestTrace, statusCode int) {
	if !otelEnabled || rt == nil {
		return
	}

	span := otelSpan{
		TraceID:   rt.TraceID,
		SpanID:    rt.SpanID,
		ParentID:  rt.ParentID,
		Name:      fmt.Sprintf("%s %s", rt.Method, rt.Path),
		Kind:      2, // SERVER
		StartTime: rt.StartTime.UnixNano(),
		EndTime:   time.Now().UnixNano(),
		Attributes: map[string]interface{}{
			"http.method":      rt.Method,
			"http.route":       rt.Path,
			"http.status_code": statusCode,
			"service.name":     otelService,
		},
		Status: 1, // OK
	}

	if statusCode >= 400 {
		span.Status = 2 // ERROR
	}

	spanBufferMu.Lock()
	spanBuffer = append(spanBuffer, span)
	if len(spanBuffer) >= maxBatchSize {
		batch := spanBuffer
		spanBuffer = nil
		spanBufferMu.Unlock()
		go exportSpans(batch)
	} else {
		spanBufferMu.Unlock()
	}
}

// Traceparent returns the W3C traceparent header value for propagation.
func Traceparent(rt *RequestTrace) string {
	if rt == nil || rt.TraceID == "" {
		return ""
	}
	return fmt.Sprintf("00-%s-%s-01", rt.TraceID, rt.SpanID)
}

// RequestTrace holds in-flight trace context for a request.
type RequestTrace struct {
	TraceID   string
	SpanID    string
	ParentID  string
	Method    string
	Path      string
	StartTime time.Time
}

// --- Internal helpers ---

func otelFlushLoop() {
	ticker := time.NewTicker(spanFlushInterval)
	defer ticker.Stop()

	for range ticker.C {
		spanBufferMu.Lock()
		if len(spanBuffer) == 0 {
			spanBufferMu.Unlock()
			continue
		}
		batch := spanBuffer
		spanBuffer = nil
		spanBufferMu.Unlock()

		exportSpans(batch)
	}
}

func exportSpans(spans []otelSpan) {
	if len(spans) == 0 {
		return
	}

	// Build OTLP/HTTP JSON payload
	payload := map[string]interface{}{
		"resourceSpans": []map[string]interface{}{
			{
				"resource": map[string]interface{}{
					"attributes": []map[string]interface{}{
						{"key": "service.name", "value": map[string]interface{}{"stringValue": otelService}},
					},
				},
				"scopeSpans": []map[string]interface{}{
					{
						"scope": map[string]interface{}{"name": "serv-runtime"},
						"spans": buildSpanPayload(spans),
					},
				},
			},
		},
	}

	body, err := json.Marshal(payload)
	if err != nil {
		LogDebug("OTEL export marshal error: ", err)
		return
	}

	url := otelEndpoint + "/v1/traces"
	resp, err := http.Post(url, "application/json", bytes.NewReader(body))
	if err != nil {
		LogDebug("OTEL export error: ", err)
		return
	}
	resp.Body.Close()

	if resp.StatusCode >= 300 {
		LogDebug("OTEL export failed: status=", resp.StatusCode)
	}
}

func buildSpanPayload(spans []otelSpan) []map[string]interface{} {
	result := make([]map[string]interface{}, 0, len(spans))
	for _, s := range spans {
		span := map[string]interface{}{
			"traceId":            s.TraceID,
			"spanId":             s.SpanID,
			"name":              s.Name,
			"kind":              s.Kind,
			"startTimeUnixNano": fmt.Sprintf("%d", s.StartTime),
			"endTimeUnixNano":   fmt.Sprintf("%d", s.EndTime),
			"status":            map[string]interface{}{"code": s.Status},
		}
		if s.ParentID != "" {
			span["parentSpanId"] = s.ParentID
		}
		if len(s.Attributes) > 0 {
			attrs := make([]map[string]interface{}, 0)
			for k, v := range s.Attributes {
				attrs = append(attrs, map[string]interface{}{
					"key":   k,
					"value": map[string]interface{}{"stringValue": fmt.Sprint(v)},
				})
			}
			span["attributes"] = attrs
		}
		result = append(result, span)
	}
	return result
}

func generateTraceID() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

func generateSpanID() string {
	b := make([]byte, 8)
	rand.Read(b)
	return hex.EncodeToString(b)
}
