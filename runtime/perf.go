package runtime

import (
	"fmt"
	"reflect"
	"strings"
)

// Performance-optimized runtime helpers.
// These replace inline closures in generated code for better performance.

// GetField extracts a named field from a value using reflection.
func GetField(obj interface{}, field string) interface{} {
	if obj == nil {
		return nil
	}
	if sm, ok := obj.(*SafeMap); ok {
		return sm.Get(field)
	}
	if m, ok := obj.(map[string]interface{}); ok {
		return m[field]
	}
	if m, ok := obj.(map[string]string); ok {
		if val, exists := m[field]; exists {
			return val
		}
		return nil
	}
	v := reflect.ValueOf(obj)
	if v.Kind() == reflect.Ptr {
		v = v.Elem()
	}
	if v.Kind() != reflect.Struct {
		return nil
	}
	capitalized := strings.ToUpper(field[:1]) + field[1:]
	f := v.FieldByName(capitalized)
	if f.IsValid() {
		return f.Interface()
	}
	f = v.FieldByName(field)
	if f.IsValid() {
		return f.Interface()
	}
	return nil
}

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
			case "%": if bv != 0 { return av % bv }
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
			case "%": if bv != 0 { return av % bv }
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

// Bitwise performs bitwise operations on two interface{} values.
func Bitwise(a, b interface{}, op string) interface{} {
	ai := toInt(a)
	bi := toInt(b)
	switch op {
	case "&":
		return ai & bi
	case "|":
		return ai | bi
	case "^":
		return ai ^ bi
	case "<<":
		if bi >= 0 {
			return ai << uint(bi)
		}
		return 0
	case ">>":
		if bi >= 0 {
			return ai >> uint(bi)
		}
		return 0
	}
	return 0
}

// ToMap converts an interface{} to map[string]interface{} for map iteration.
func ToMap(v interface{}) map[string]interface{} {
	if v == nil {
		return nil
	}
	switch m := v.(type) {
	case map[string]interface{}:
		return m
	case *SafeMap:
		return m.All()
	case map[string]string:
		result := make(map[string]interface{}, len(m))
		for k, val := range m {
			result[k] = val
		}
		return result
	}
	return nil
}

// Slice extracts a sub-slice from an interface{} value.
func Slice(v interface{}, start interface{}, end interface{}) interface{} {
	if v == nil {
		return nil
	}
	switch arr := v.(type) {
	case []interface{}:
		s := 0
		e := len(arr)
		if start != nil {
			s = toInt(start)
			if s < 0 { s = 0 }
			if s > len(arr) { s = len(arr) }
		}
		if end != nil {
			e = toInt(end)
			if e < 0 { e = 0 }
			if e > len(arr) { e = len(arr) }
		}
		return arr[s:e]
	case string:
		s := 0
		e := len(arr)
		if start != nil {
			s = toInt(start)
			if s < 0 { s = 0 }
			if s > len(arr) { s = len(arr) }
		}
		if end != nil {
			e = toInt(end)
			if e < 0 { e = 0 }
			if e > len(arr) { e = len(arr) }
		}
		return arr[s:e]
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
	case map[string]string:
		if val, ok := v[field]; ok {
			return val
		}
		return nil
	default:
		return GetField(obj, field)
	}
	return nil
}

// MergeMaps merges multiple maps into a single map[string]interface{}.
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

// TryCall executes a function and returns its result.
// If the function panics, returns nil (error is swallowed).
// If the function returns a [2]interface{} tuple, returns only the first value
// (the error can be extracted separately by the caller).
// Used by the ? operator for error propagation.
func TryCall(fn func() interface{}) interface{} {
	var result interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				result = nil
			}
		}()
		result = fn()
	}()
	// If result is a tuple, extract value
	if tuple, ok := result.([2]interface{}); ok {
		return tuple[0]
	}
	return result
}

// TryCallWithError executes a function and returns (value, error).
// Used internally for the ? operator to detect and propagate errors.
func TryCallWithError(fn func() interface{}) (interface{}, interface{}) {
	var result interface{}
	var errVal interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				errVal = fmt.Sprint(r)
			}
		}()
		result = fn()
	}()
	if errVal != nil {
		return nil, errVal
	}
	// If result is a tuple [value, error], unpack
	if tuple, ok := result.([2]interface{}); ok {
		return tuple[0], tuple[1]
	}
	return result, nil
}

// Negate negates a numeric value (unary minus on interface{}).
func Negate(v interface{}) interface{} {
	switch val := v.(type) {
	case int:
		return -val
	case int64:
		return -val
	case float64:
		return -val
	default:
		return 0
	}
}

// IndexAccess retrieves an item from a list or map by index/key.
func IndexAccess(val interface{}, index interface{}) interface{} {
	if val == nil || index == nil {
		return nil
	}
	switch v := val.(type) {
	case []interface{}:
		idx := toInt(index)
		if idx >= 0 && idx < len(v) {
			return v[idx]
		}
	case *SafeMap:
		return v.Get(fmt.Sprint(index))
	case map[string]interface{}:
		return v[fmt.Sprint(index)]
	}
	return nil
}
