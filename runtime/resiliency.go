package runtime

import (
	"errors"
	"fmt"
	"sync"
	"time"
)

// Circuit breaker states
type CBState int

const (
	StateClosed CBState = iota
	StateOpen
	StateHalfOpen
)

type CircuitBreaker struct {
	mu           sync.RWMutex
	state        CBState
	failures     int
	threshold    int
	timeout      time.Duration
	lastStateChg time.Time
}

var (
	breakers   = make(map[string]*CircuitBreaker)
	breakersMu sync.Mutex
)

func getBreaker(name string) *CircuitBreaker {
	breakersMu.Lock()
	defer breakersMu.Unlock()
	cb, exists := breakers[name]
	if !exists {
		cb = &CircuitBreaker{
			state:     StateClosed,
			threshold: 3,                // open after 3 consecutive failures
			timeout:   2 * time.Second,   // try recovery after 2 seconds
		}
		breakers[name] = cb
	}
	return cb
}

// ResilientCall wraps function execution with retries, timeout, and circuit breaker logic.
func ResilientCall(fnName string, fn func() interface{}, retries int, timeoutStr string, hasCB bool) interface{} {
	var timeout time.Duration
	if timeoutStr != "" {
		var err error
		timeout, err = time.ParseDuration(timeoutStr)
		if err != nil {
			// If it's a raw integer without unit, default to seconds
			if val, convErr := strconvAtoi(timeoutStr); convErr == nil {
				timeout = time.Duration(val) * time.Second
			}
		}
	}

	cb := getBreaker(fnName)

	executeWithTimeout := func() (interface{}, error) {
		if timeout > 0 {
			resChan := make(chan interface{}, 1)
			errChan := make(chan error, 1)

			go func() {
				defer func() {
					if r := recover(); r != nil {
						errChan <- fmt.Errorf("panic: %v", r)
					}
				}()
				resChan <- fn()
			}()

			select {
			case res := <-resChan:
				if err, isErr := isErrorValue(res); isErr {
					return nil, err
				}
				return res, nil
			case err := <-errChan:
				return nil, err
			case <-time.After(timeout):
				return nil, errors.New("timeout exceeded")
			}
		} else {
			var res interface{}
			var err error
			func() {
				defer func() {
					if r := recover(); r != nil {
						err = fmt.Errorf("panic: %v", r)
					}
				}()
				res = fn()
			}()
			if err != nil {
				return nil, err
			}
			if errVal, isErr := isErrorValue(res); isErr {
				return nil, errVal
			}
			return res, nil
		}
	}

	var lastErr error
	for attempt := 0; attempt <= retries; attempt++ {
		if hasCB {
			cb.mu.Lock()
			if cb.state == StateOpen && time.Since(cb.lastStateChg) > cb.timeout {
				cb.state = StateHalfOpen
				cb.lastStateChg = time.Now()
			}

			if cb.state == StateOpen {
				cb.mu.Unlock()
				return [2]interface{}{nil, errors.New("circuit breaker is open")}
			}
			cb.mu.Unlock()
		}

		res, err := executeWithTimeout()
		if err == nil {
			if hasCB {
				cb.mu.Lock()
				if cb.state == StateHalfOpen {
					cb.state = StateClosed
					cb.failures = 0
					cb.lastStateChg = time.Now()
				} else {
					cb.failures = 0
				}
				cb.mu.Unlock()
			}
			return res
		}

		lastErr = err
		if hasCB {
			cb.mu.Lock()
			cb.failures++
			if cb.failures >= cb.threshold {
				cb.state = StateOpen
				cb.lastStateChg = time.Now()
			}
			cb.mu.Unlock()
		}

		if attempt < retries {
			time.Sleep(10 * time.Millisecond)
		}
	}

	return [2]interface{}{nil, lastErr}
}

func strconvAtoi(s string) (int, error) {
	var res int
	for _, c := range s {
		if c >= '0' && c <= '9' {
			res = res*10 + int(c-'0')
		} else {
			return 0, fmt.Errorf("invalid int: %s", s)
		}
	}
	return res, nil
}

func isErrorValue(v interface{}) (error, bool) {
	if v == nil {
		return nil, false
	}
	if err, ok := v.(error); ok {
		return err, true
	}
	if arr, ok := v.([2]interface{}); ok {
		if err, ok := arr[1].(error); ok && err != nil {
			return err, true
		}
	}
	if m, ok := v.(map[string]interface{}); ok {
		if errVal, exists := m["error"]; exists && errVal != nil {
			return fmt.Errorf("%v", errVal), true
		}
	}
	if sm, ok := v.(*SafeMap); ok {
		if errVal := sm.Get("error"); errVal != nil {
			return fmt.Errorf("%v", errVal), true
		}
	}
	return nil, false
}
