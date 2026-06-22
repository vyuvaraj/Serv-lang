//go:build !wasm

package runtime

import (
	"fmt"
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
				// Dynamic role guard parsing
				if strings.HasPrefix(name, "auth.role(") && strings.HasSuffix(name, ")") {
					roleVal := strings.TrimPrefix(name, "auth.role(")
					roleVal = strings.TrimSuffix(roleVal, ")")
					roleVal = strings.Trim(roleVal, `"'`) // remove quotes
					
					userRoles := req.Params["auth_role"]
					if userRoles == "" {
						userRoles = req.Params["auth_roles"]
					}
					if userRoles == "" {
						userRoles = req.Params["auth_realm_access.roles"]
					}
					
					found := false
					for _, r := range strings.Split(userRoles, ",") {
						if strings.TrimSpace(r) == roleVal {
							found = true
							break
						}
					}
					if !found {
						middlewareRegistryMu.RUnlock()
						return map[string]interface{}{
							"status": 403,
							"error":  "Forbidden",
							"code":   "ERR_FORBIDDEN",
							"message": fmt.Sprintf("Missing required role: %s", roleVal),
						}
					}
					continue
				}

				// Dynamic scope guard parsing
				if strings.HasPrefix(name, "auth.scope(") && strings.HasSuffix(name, ")") {
					scopeVal := strings.TrimPrefix(name, "auth.scope(")
					scopeVal = strings.TrimSuffix(scopeVal, ")")
					scopeVal = strings.Trim(scopeVal, `"'`) // remove quotes
					
					userScopes := req.Params["auth_scope"]
					if userScopes == "" {
						userScopes = req.Params["auth_scp"]
					}
					if userScopes == "" {
						userScopes = req.Params["auth_scopes"]
					}
					
					found := false
					sep := ","
					if strings.Contains(userScopes, " ") && !strings.Contains(userScopes, ",") {
						sep = " "
					}
					for _, s := range strings.Split(userScopes, sep) {
						if strings.TrimSpace(s) == scopeVal {
							found = true
							break
						}
					}
					if !found {
						middlewareRegistryMu.RUnlock()
						return map[string]interface{}{
							"status": 403,
							"error":  "Forbidden",
							"code":   "ERR_FORBIDDEN",
							"message": fmt.Sprintf("Missing required scope: %s", scopeVal),
						}
					}
					continue
				}

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
	fullPath  string
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
	curr.fullPath = path
}

func matchRoute(method, path string) (func(Request) interface{}, map[string]string, *routeRateLimiter, string) {
	routeTrieMu.RLock()
	root, ok := routeTrie[method]
	routeTrieMu.RUnlock()
	if !ok {
		return nil, nil, nil, ""
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
			return nil, nil, nil, ""
		}
	}
	return curr.handler, params, curr.limiter, curr.fullPath
}

var (
	corsEnabled   bool
	corsOrigins   []string
	corsOriginsMu sync.RWMutex

	globalIPRateLimitEnabled bool
	globalIPRateLimiter      *ipRateLimiter
)

type ipRateLimiter struct {
	rate        int
	period      string
	limiters    map[string]*routeRateLimiter
	limitersMu  sync.RWMutex
}

func newIPRateLimiter(rate int, period string) *ipRateLimiter {
	return &ipRateLimiter{
		rate:     rate,
		period:   period,
		limiters: make(map[string]*routeRateLimiter),
	}
}

func (ipl *ipRateLimiter) getLimiter(ip string) *routeRateLimiter {
	ipl.limitersMu.RLock()
	lim, exists := ipl.limiters[ip]
	ipl.limitersMu.RUnlock()
	if exists {
		return lim
	}
	ipl.limitersMu.Lock()
	defer ipl.limitersMu.Unlock()
	lim, exists = ipl.limiters[ip]
	if !exists {
		lim = newRouteRateLimiter(ipl.rate, ipl.period)
		ipl.limiters[ip] = lim
	}
	return lim
}

func EnableCORS(origins []string) {
	corsOriginsMu.Lock()
	defer corsOriginsMu.Unlock()
	corsEnabled = true
	corsOrigins = origins
}

func SetGlobalIPRateLimit(rate int, period string) {
	globalIPRateLimitEnabled = true
	globalIPRateLimiter = newIPRateLimiter(rate, period)
}

func GetGlobalIPRateLimiterStub(ip string) *routeRateLimiter {
	if globalIPRateLimiter == nil {
		return nil
	}
	return globalIPRateLimiter.getLimiter(ip)
}

func (rl *routeRateLimiter) AllowStub() bool {
	return rl.allow()
}

func MatchRouteStub(method, path string) (func(Request) interface{}, map[string]string, interface{}, string) {
	h, p, l, pat := matchRoute(method, path)
	return h, p, l, pat
}
