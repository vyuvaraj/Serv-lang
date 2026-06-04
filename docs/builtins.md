# Built-in Functions & Objects

Serv provides built-in objects for common service operations. No imports needed.

## log — Structured Logging

```serv
log.info("Server started")
log.warn("Slow query detected")
log.error("Connection failed: ", err)
log.debug("Processing item: ", id)

// Context logger (fields included in every log)
let logger = log.with("service", "auth", "version", "2.0")
logger.info("Request processed")

// Logger from map
let reqLog = log.fields({ "request_id": id, "user": name })
reqLog.error("Failed")

// Runtime level control
log.setLevel("debug")
let level = log.getLevel()
```

**Environment:** `LOG_FORMAT=json` for JSON output, `LOG_LEVEL=debug|info|warn|error`

## db — Database Operations

```serv
database "sqlite://app.db"        // SQLite
database "postgres://user:pass@host/db"  // PostgreSQL
database "mongodb://localhost:27017/mydb"  // MongoDB

// Query (SQL or MongoDB)
let rows = db.query("SELECT * FROM users WHERE active = ?", true)
let result = db.query("INSERT INTO users (name) VALUES (?)", "Alice")

// MongoDB-specific
let page = db.queryPage("users", "{}", 1, 20)
let user = db.findOne("users", "{\"email\": \"a@test.com\"}")
let count = db.count("users", "{\"active\": true}")
let res = db.upsert("users", filter, update)
```

## cache — Caching

```serv
cache "redis://localhost:6379"    // Redis
cache "in-memory"                 // Local (dev/testing)

cache.set("key", value, "60s")   // Set with TTL
let val = cache.get("key")       // Get (nil if expired/missing)
```

## http — HTTP Client

```serv
let resp = http.get("https://api.example.com/data")
// resp.status = 200, resp.body = "..."

let resp = http.post("https://api.example.com/users", body)
```

## json — JSON Operations

```serv
let obj = json.parse("{\"name\": \"Alice\"}")
let str = json.stringify({ "name": "Alice" })
```

## time — Time Operations

```serv
let now = time.now()       // ISO 8601 timestamp
let ts = time.unix()       // Unix timestamp (int)
time.sleep(1000)           // Sleep milliseconds
```

## env — Environment Variables

```serv
let port = env("PORT")     // Read env var (empty string if not set)
```

## config — Configuration

```serv
let host = config("db.host")   // Read from config.yml or env
```

Reads from `config.yml` in the working directory, or maps dotted keys to env vars (`db.host` → `DB_HOST`).

## metric — Metrics

```serv
metric.inc("requests_total")
metric.gauge("active_connections", 42)
```

Exposed at `GET /metrics` endpoint.

## publish / subscribe — Messaging

```serv
publish "topic" "message"

subscribe "topic" (msg) {
    log.info("Received: ", msg)
}
```

## atomic — Atomic Operations

```serv
atomic.new("counter", 0)
atomic.inc("counter")
atomic.dec("counter")
let val = atomic.get("counter")
atomic.set("counter", 100)
atomic.cas("counter", 100, 200)  // Compare-and-swap
```

## channel — Go Channels

```serv
let ch = channel.new("mychan", 10)  // Buffered channel
channel.send("mychan", "data")
let msg = channel.receive("mychan")
let msg = channel.tryReceive("mychan")  // Non-blocking
channel.close("mychan")
```

## registry — Named Function Registry

```serv
registry.set("handler", fn(x) { return x * 2 })
let result = registry.call("handler", 5)  // 10
registry.has("handler")  // true
registry.list()          // ["handler"]
```

## validate — Request Validation

```serv
let errors = validate(req.body, {
    "email": "required,email",
    "name": "required,string",
    "age": "int"
})
// Returns nil if valid, or ["email is required", ...] if invalid
```

**Rules:** `required`, `string`, `int`, `float`, `bool`, `email` — combine with commas.

## String Methods

```serv
"hello world".split(" ")      // ["hello", "world"]
"  hi  ".trim()               // "hi"
"hello".replace("l", "L")     // "heLLo"
"hello".startsWith("he")      // true
"hello".endsWith("lo")        // true
"hello".includes("ell")       // true
"hello".toUpper()             // "HELLO"
"HELLO".toLower()             // "hello"
"hello".substring(1, 3)       // "el"
"hello".indexOf("l")          // 2
"ha".repeat(3)                // "hahaha"
"hello".length()              // 5
```

## Collection Methods

```serv
let items = [1, 2, 3, 4, 5]

items.filter(x => x > 2)        // [3, 4, 5]
items.map(x => x * 2)           // [2, 4, 6, 8, 10]
items.find(x => x == 3)         // 3
items.reduce(fn(a, b) { return a + b }, 0)  // 15
items.forEach(x => log.info(x))
items.contains(3)                // true
items.push(6)                    // [1, 2, 3, 4, 5, 6]
items.length()                   // 5

// Slice expressions
let first3 = items[0:3]          // [1, 2, 3]
let rest = items[2:]             // [3, 4, 5]
let head = items[:2]             // [1, 2]
```
