//go:build !wasm

package runtime

import (
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"
)

var (
	logJSON    bool
	logLevel   = "info" // "debug", "info", "warn", "error"
	logLevelMu sync.RWMutex
)

func init() {
	// Check for JSON log mode
	if Config("log.format") == "json" || os.Getenv("LOG_FORMAT") == "json" {
		logJSON = true
	}
	if lvl := Config("log.level"); lvl != "" {
		logLevel = lvl
	} else if lvl := os.Getenv("LOG_LEVEL"); lvl != "" {
		logLevel = lvl
	}
}

func shouldLog(level string) bool {
	levels := map[string]int{"debug": 0, "info": 1, "warn": 2, "error": 3}
	logLevelMu.RLock()
	defer logLevelMu.RUnlock()
	return levels[level] >= levels[logLevel]
}

func logStructured(level string, args ...interface{}) {
	logStructuredWithFields(level, nil, args...)
}

func logStructuredWithFields(level string, fields map[string]interface{}, args ...interface{}) {
	if !shouldLog(level) {
		return
	}
	msg := fmt.Sprint(args...)
	if logJSON {
		entry := map[string]interface{}{
			"level":     level,
			"message":   msg,
			"timestamp": time.Now().Format(time.RFC3339),
		}
		for k, v := range fields {
			entry[k] = v
		}
		b, _ := json.Marshal(entry)
		fmt.Println(string(b))
	} else {
		if len(fields) > 0 {
			var pairs []string
			for k, v := range fields {
				pairs = append(pairs, fmt.Sprintf("%s=%v", k, v))
			}
			log.Printf("[%s] %s %s", strings.ToUpper(level), msg, strings.Join(pairs, " "))
		} else {
			log.Printf("[%s] %s", strings.ToUpper(level), msg)
		}
	}
}

func LogInfo(args ...interface{}) {
	logStructured("info", args...)
}

func LogWarn(args ...interface{}) {
	logStructured("warn", args...)
}

func LogError(args ...interface{}) {
	logStructured("error", args...)
}

func LogDebug(args ...interface{}) {
	logStructured("debug", args...)
}

// ContextLogger holds pre-set fields and emits them with every log call.
// Usage from Serv: let logger = log.with("request_id", id, "service", "auth")
//
//	logger.info("request processed")
type ContextLogger struct {
	Fields map[string]interface{}
}

// Info logs at info level with the logger's context fields.
func (cl *ContextLogger) Info(args ...interface{}) interface{} {
	logStructuredWithFields("info", cl.Fields, args...)
	return nil
}

// Warn logs at warn level with the logger's context fields.
func (cl *ContextLogger) Warn(args ...interface{}) interface{} {
	logStructuredWithFields("warn", cl.Fields, args...)
	return nil
}

// Error logs at error level with the logger's context fields.
func (cl *ContextLogger) Error(args ...interface{}) interface{} {
	logStructuredWithFields("error", cl.Fields, args...)
	return nil
}

// Debug logs at debug level with the logger's context fields.
func (cl *ContextLogger) Debug(args ...interface{}) interface{} {
	logStructuredWithFields("debug", cl.Fields, args...)
	return nil
}

// With returns a new ContextLogger with additional fields merged in.
func (cl *ContextLogger) With(args ...interface{}) *ContextLogger {
	merged := make(map[string]interface{})
	for k, v := range cl.Fields {
		merged[k] = v
	}
	for i := 0; i+1 < len(args); i += 2 {
		merged[fmt.Sprint(args[i])] = args[i+1]
	}
	return &ContextLogger{Fields: merged}
}

// LogWith creates a ContextLogger with the given key-value pairs.
// Usage from Serv:
//
//	log.with("user_id", 123, "action", "login")            — logs at info with fields (legacy)
//	let logger = log.with("request_id", id)                 — returns a reusable logger
//	logger.info("handled request")
func LogWith(args ...interface{}) interface{} {
	fields := make(map[string]interface{})
	for i := 0; i+1 < len(args); i += 2 {
		fields[fmt.Sprint(args[i])] = args[i+1]
	}
	// If odd number of args, last arg is a message — log immediately (legacy behavior)
	if len(args)%2 == 1 {
		msg := fmt.Sprint(args[len(args)-1])
		logStructuredWithFields("info", fields, msg)
		return nil
	}
	// Even number of args: return a ContextLogger for chaining
	return &ContextLogger{Fields: fields}
}

// LogFields creates a ContextLogger from a map of fields.
// Usage from Serv: let logger = log.fields({ request_id: id, service: "auth" })
//
//	logger.info("ready")
func LogFields(args ...interface{}) interface{} {
	fields := make(map[string]interface{})
	if len(args) == 1 {
		switch m := args[0].(type) {
		case map[string]interface{}:
			fields = m
		case *SafeMap:
			for k, v := range m.All() {
				fields[k] = v
			}
		}
	}
	return &ContextLogger{Fields: fields}
}

// LogSetLevel changes the runtime log level.
// Usage from Serv: log.setLevel("debug")
func LogSetLevel(args ...interface{}) interface{} {
	if len(args) == 0 {
		return nil
	}
	lvl := strings.ToLower(fmt.Sprint(args[0]))
	switch lvl {
	case "debug", "info", "warn", "error":
		logLevelMu.Lock()
		logLevel = lvl
		logLevelMu.Unlock()
	}
	return nil
}

// LogGetLevel returns the current log level.
// Usage from Serv: let level = log.getLevel()
func LogGetLevel(args ...interface{}) interface{} {
	logLevelMu.RLock()
	defer logLevelMu.RUnlock()
	return logLevel
}

// ContextLoggerInfo calls .Info() on a ContextLogger value.
// Used when codegen encounters: logger.info("msg")
func ContextLoggerInfo(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.Info(args...)
	}
	// Fallback: just log normally
	logStructured("info", args...)
	return nil
}

// ContextLoggerWarn calls .Warn() on a ContextLogger value.
func ContextLoggerWarn(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.Warn(args...)
	}
	logStructured("warn", args...)
	return nil
}

// ContextLoggerError calls .Error() on a ContextLogger value.
func ContextLoggerError(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.Error(args...)
	}
	logStructured("error", args...)
	return nil
}

// ContextLoggerDebug calls .Debug() on a ContextLogger value.
func ContextLoggerDebug(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.Debug(args...)
	}
	logStructured("debug", args...)
	return nil
}

// ContextLoggerWith calls .With() on a ContextLogger value to add more fields.
func ContextLoggerWith(logger interface{}, args ...interface{}) interface{} {
	if cl, ok := logger.(*ContextLogger); ok {
		return cl.With(args...)
	}
	// If not a ContextLogger, create a new one
	return LogWith(args...)
}

