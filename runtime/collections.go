package runtime

import (
	"encoding/json"
	"fmt"
	"reflect"
	"sync"
)

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
	if val == nil {
		return 0
	}
	switch v := val.(type) {
	case string:
		return len(v)
	case *SafeMap:
		v.mu.RLock()
		defer v.mu.RUnlock()
		return len(v.m)
	case map[string]interface{}:
		return len(v)
	}
	rv := reflect.ValueOf(val)
	if rv.Kind() == reflect.Slice || rv.Kind() == reflect.Array {
		return rv.Len()
	}
	return 0
}

// Push appends an element to a slice and returns the new slice.
func Push(slice interface{}, elem interface{}) interface{} {
	items := toInterfaceSlice(slice)
	return append(items, elem)
}

// Contains checks if a slice contains an element.
func Contains(slice interface{}, target interface{}) bool {
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
