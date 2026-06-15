package runtime

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"
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

// JSON native support
func JSONParse(dataVal interface{}) interface{} {
	data := fmt.Sprint(dataVal)
	var val interface{}
	err := json.Unmarshal([]byte(data), &val)
	if err != nil {
		return [2]interface{}{nil, fmt.Sprintf("JSON parse error: %s", err.Error())}
	}
	return ToSafeValue(val)
}

func JSONStringify(val interface{}) string {
	b, err := json.Marshal(val)
	if err != nil {
		return ""
	}
	return string(b)
}

// JSONParseSafe is kept for backward compatibility — now just calls JSONParse directly.
func JSONParseSafe(dataVal interface{}) interface{} {
	return JSONParse(dataVal)
}

// WASM helpers for Serverless Compute
func WasmReadInput() interface{} {
	data, err := io.ReadAll(os.Stdin)
	if err != nil {
		return ""
	}
	return string(data)
}

func WasmWriteOutput(data interface{}) interface{} {
	str := fmt.Sprint(data)
	_, _ = os.Stdout.WriteString(str)
	return nil
}

// Noop is a no-op sentinel used by generated test files to satisfy the runtime import.
func Noop() {}
