package runtime

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
)

func TestTracePropagation(t *testing.T) {
	// Enable Otel for the duration of this test
	otelEnabled = true
	otelService = "test-service"
	defer func() {
		otelEnabled = false
	}()

	// Reset span buffer
	spanBufferMu.Lock()
	spanBuffer = nil
	spanBufferMu.Unlock()

	// Initialize the broker fallback queues so we can publish/subscribe
	pubSubQueue = make(chan pubSubEvent, 100)

	// 1. Start a root request trace
	parentTrace := "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"
	trace := TraceRequest("GET", "/api/data", parentTrace)
	SetActiveTrace(trace)
	defer ClearActiveTrace()

	// 2. Perform DB query (should inherit trace context)
	dbSpanEnd := TraceDB("SELECT", "SELECT * FROM users")
	dbSpanEnd()

	// 3. Perform Cache query (should inherit trace context)
	cacheSpanEnd := TraceCache("GET", "user_1")
	cacheSpanEnd()

	// 4. Test spawned worker context propagation
	var wg sync.WaitGroup
	wg.Add(1)

	// Simulate codegen behavior for spawn
	_spawnTrace := GetActiveTrace()
	go func() {
		defer wg.Done()
		if _spawnTrace != nil {
			SetActiveTrace(_spawnTrace)
			defer ClearActiveTrace()
		}
		_endSpan := TraceSpawn("async_task")
		defer _endSpan()

		// DB call inside goroutine should inherit the same trace ID
		innerEnd := TraceDB("INSERT", "INSERT INTO logs ...")
		innerEnd()
	}()

	wg.Wait()

	// 5. Test Pub/Sub propagation (in-memory)
	wg.Add(1)
	Subscribe("test-topic", func(msg string) {
		defer wg.Done()
		// The active trace inside subscription callback should match the publisher's trace context
		active := GetActiveTrace()
		if active == nil {
			t.Error("Expected active trace in subscription callback, got nil")
			return
		}
		if active.TraceID != "4bf92f3577b34da6a3ce929d0e0e4736" {
			t.Errorf("Expected trace ID 4bf92f3577b34da6a3ce929d0e0e4736, got %s", active.TraceID)
		}

		// Perform Cache call inside callback (should inherit trace context)
		subCacheEnd := TraceCache("SET", "sub_key")
		subCacheEnd()
	})

	Publish("test-topic", "hello")
	wg.Wait()

	// 6. Test outgoing HTTP client propagation
	// Set up mock HTTP server
	mockServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		tp := r.Header.Get("traceparent")
		if tp == "" {
			t.Error("Expected traceparent header in HTTP client request, got none")
		} else {
			parts := strings.Split(tp, "-")
			if len(parts) >= 4 {
				if parts[1] != "4bf92f3577b34da6a3ce929d0e0e4736" {
					t.Errorf("Expected traceparent trace ID 4bf92f3577b34da6a3ce929d0e0e4736, got %s", parts[1])
				}
			} else {
				t.Errorf("Invalid traceparent format: %s", tp)
			}
		}
		w.WriteHeader(http.StatusOK)
		w.Write([]byte(`"OK"`))
	}))
	defer mockServer.Close()

	// Perform HTTPGet helper call
	HTTPGet(mockServer.URL)

	// End root trace
	EndTrace(trace, 200)

	// Verify span relationships in spanBuffer
	spanBufferMu.Lock()
	spans := make([]otelSpan, len(spanBuffer))
	copy(spans, spanBuffer)
	spanBufferMu.Unlock()

	if len(spans) < 8 {
		t.Errorf("Expected at least 8 spans, got %d", len(spans))
	}

	expectedRootTraceID := "4bf92f3577b34da6a3ce929d0e0e4736"
	for _, span := range spans {
		if span.TraceID != expectedRootTraceID {
			t.Errorf("Span %s has mismatched TraceID: expected %s, got %s", span.Name, expectedRootTraceID, span.TraceID)
		}
	}
}
