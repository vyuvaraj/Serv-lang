package runtime

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
)

// ValidateConfig checks that all required config keys are present at startup.
func ValidateConfig(requiredKeys []string) {
	var missing []string
	for _, key := range requiredKeys {
		val := Config(key)
		if val == "" {
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
func ValidateBody(args ...interface{}) interface{} {
	if len(args) < 2 {
		return []interface{}{"validate requires (body, schema) arguments"}
	}

	bodyStr := fmt.Sprint(args[0])
	var body map[string]interface{}
	if err := json.Unmarshal([]byte(bodyStr), &body); err != nil {
		return []interface{}{"invalid JSON body: " + err.Error()}
	}

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
					case float64:
					case int, int64:
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
