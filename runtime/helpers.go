package runtime

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"sync"
	"time"
)

// Metrics
var (
	metricsCounters = make(map[string]int64)
	metricsGauges   MapStringFloat
	metricsMu       sync.RWMutex
)

type MapStringFloat struct {
	m map[string]float64
	sync.RWMutex
}

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

// Safe variants that return [2]interface{}{value, error} tuples for multi-return support.
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
