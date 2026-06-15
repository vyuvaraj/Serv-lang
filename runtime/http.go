//go:build !wasm

package runtime

import (
	"strings"
	"sync"
	"time"
)

// HTTP routing global state
var (
	routes   = make(map[string]map[string]func(Request) interface{}) // method -> path -> handler
	routesMu sync.RWMutex

	routeTrie   = make(map[string]*trieNode) // method -> root trie node
	routeTrieMu sync.RWMutex

	// Middleware registry
	middlewareRegistry   = make(map[string]func(Request) interface{})
	middlewareRegistryMu sync.RWMutex
)

// Rate limiter per route
type routeRateLimiter struct {
	limitRate   int
	limitPeriod time.Duration
	tokensMutex sync.Mutex
	tokens      float64
	lastRefill  time.Time
}

func newRouteRateLimiter(rate int, period string) *routeRateLimiter {
	var dur time.Duration
	switch period {
	case "s":
		dur = time.Second
	case "m":
		dur = time.Minute
	case "h":
		dur = time.Hour
	default:
		dur = time.Second
	}
	return &routeRateLimiter{
		limitRate:   rate,
		limitPeriod: dur,
		tokens:      float64(rate),
		lastRefill:  time.Now(),
	}
}

func (rl *routeRateLimiter) allow() bool {
	rl.tokensMutex.Lock()
	defer rl.tokensMutex.Unlock()

	now := time.Now()
	elapsed := now.Sub(rl.lastRefill)
	rl.lastRefill = now

	refillRate := float64(rl.limitRate) / float64(rl.limitPeriod)
	rl.tokens += float64(elapsed) * refillRate
	if rl.tokens > float64(rl.limitRate) {
		rl.tokens = float64(rl.limitRate)
	}

	if rl.tokens >= 1.0 {
		rl.tokens -= 1.0
		return true
	}
	return false
}

func AddRoute(method, path string, limitRate int, limitPeriod string, handler func(Request) interface{}) {
	routesMu.Lock()
	if _, ok := routes[method]; !ok {
		routes[method] = make(map[string]func(Request) interface{})
	}
	routes[method][path] = handler
	routesMu.Unlock()

	var limiter *routeRateLimiter
	if limitRate > 0 {
		limiter = newRouteRateLimiter(limitRate, limitPeriod)
	}

	insertRoute(method, path, limiter, handler)
	LogInfo("Registered route: ", method, " ", path)
}

// RegisterMiddleware registers a named middleware function.
func RegisterMiddleware(name string, handler func(Request) interface{}) {
	middlewareRegistryMu.Lock()
	defer middlewareRegistryMu.Unlock()
	middlewareRegistry[name] = handler
	LogInfo("Registered middleware: ", name)
}

// AddRouteWithMiddleware registers a route with a middleware chain.
// Middlewares are executed in order before the handler.
// If any middleware returns non-nil, that response is sent and the handler is skipped.
func AddRouteWithMiddleware(method, path string, limitRate int, limitPeriod string, middlewareNames []string, handler func(Request) interface{}) {
	wrappedHandler := func(req Request) interface{} {
		// Execute middleware chain
		middlewareRegistryMu.RLock()
		for _, name := range middlewareNames {
			mw, exists := middlewareRegistry[name]
			if !exists {
				LogWarn("Middleware not found: ", name)
				continue
			}
			result := mw(req)
			if result != nil {
				middlewareRegistryMu.RUnlock()
				return result // short-circuit: middleware returned a response
			}
		}
		middlewareRegistryMu.RUnlock()

		// All middlewares passed, execute handler
		return handler(req)
	}

	AddRoute(method, path, limitRate, limitPeriod, wrappedHandler)
}

// Trie-based route matching
type trieNode struct {
	children  map[string]*trieNode
	handler   func(Request) interface{}
	isParam   bool
	paramName string
	limiter   *routeRateLimiter
}

func newTrieNode() *trieNode {
	return &trieNode{children: make(map[string]*trieNode)}
}

func insertRoute(method, path string, limiter *routeRateLimiter, handler func(Request) interface{}) {
	routeTrieMu.Lock()
	defer routeTrieMu.Unlock()

	root, ok := routeTrie[method]
	if !ok {
		root = newTrieNode()
		routeTrie[method] = root
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	curr := root
	for _, part := range parts {
		if part == "" {
			continue
		}
		isParam := strings.HasPrefix(part, ":")
		paramName := ""
		childKey := part
		if isParam {
			paramName = strings.TrimPrefix(part, ":")
			childKey = ":"
		}

		child, ok := curr.children[childKey]
		if !ok {
			child = newTrieNode()
			child.isParam = isParam
			child.paramName = paramName
			curr.children[childKey] = child
		}
		curr = child
	}
	curr.handler = handler
	curr.limiter = limiter
}

func matchRoute(method, path string) (func(Request) interface{}, map[string]string, *routeRateLimiter) {
	routeTrieMu.RLock()
	root, ok := routeTrie[method]
	routeTrieMu.RUnlock()
	if !ok {
		return nil, nil, nil
	}

	parts := strings.Split(strings.Trim(path, "/"), "/")
	params := make(map[string]string)
	curr := root

	for _, part := range parts {
		if part == "" {
			continue
		}
		if child, ok := curr.children[part]; ok {
			curr = child
		} else if child, ok := curr.children[":"]; ok {
			params[child.paramName] = part
			curr = child
		} else {
			return nil, nil, nil
		}
	}
	return curr.handler, params, curr.limiter
}

