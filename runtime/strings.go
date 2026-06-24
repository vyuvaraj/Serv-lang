package runtime

import (
	"fmt"
	"strconv"
	"strings"
)

// String method implementations for Serv's built-in string operations.

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

func StringTrim(s interface{}) string {
	return strings.TrimSpace(fmt.Sprint(s))
}

func StringReplace(s interface{}, old interface{}, new interface{}) string {
	return strings.ReplaceAll(fmt.Sprint(s), fmt.Sprint(old), fmt.Sprint(new))
}

func StringStartsWith(s interface{}, prefix interface{}) bool {
	return strings.HasPrefix(fmt.Sprint(s), fmt.Sprint(prefix))
}

func StringEndsWith(s interface{}, suffix interface{}) bool {
	return strings.HasSuffix(fmt.Sprint(s), fmt.Sprint(suffix))
}

func StringIncludes(s interface{}, substr interface{}) bool {
	return strings.Contains(fmt.Sprint(s), fmt.Sprint(substr))
}

func StringToUpper(s interface{}) string {
	return strings.ToUpper(fmt.Sprint(s))
}

func StringToLower(s interface{}) string {
	return strings.ToLower(fmt.Sprint(s))
}

func StringSubstring(s interface{}, start interface{}, args ...interface{}) string {
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

func StringIndexOf(s interface{}, substr interface{}) int {
	return strings.Index(fmt.Sprint(s), fmt.Sprint(substr))
}

func StringRepeat(s interface{}, count interface{}) string {
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
