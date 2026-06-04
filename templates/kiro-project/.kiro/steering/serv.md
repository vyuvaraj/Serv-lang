---
inclusion: always
---

# Serv Language Guide

## What is Serv?
Serv is a compiled language for building services, APIs, schedulers, and event-driven apps. It compiles to native binaries via Go code generation.

## File Extension
- Source files: `.srv`

## Commands
```bash
serv build app.srv -o app.exe    # Compile to binary
serv run app.srv                 # Compile & run
serv run app.srv --watch         # Hot-reload on changes
serv test app.srv                # Run tests
serv test --cover app.srv        # Run tests with coverage
serv lint app.srv                # Check for errors/warnings
serv fmt app.srv                 # Format code (4-space indent)
```

## Syntax Quick Reference

### Infrastructure
```serv
server "8080"
database "sqlite://app.db"
cache "redis://localhost:6379"
broker "nats://localhost:4222"
```

### Variables & Types
```serv
let name = "Alice"               // inferred string
let age: int = 30                // explicit type
let email: string? = nil         // optional (nullable)
let { x, y } = point            // destructuring
let val, err = riskyCall()       // multi-return
```

### Functions
```serv
fn add(a: int, b: int) -> int {
    return a + b
}

fn greet(name: string?) -> string {
    if name == nil { return "Hello, stranger" }
    return f"Hello, {name}"
}

// Arrow functions
let double = x => x * 2
```

### Routes
```serv
route "GET" "/users" (req) {
    return { "users": [] }
}

route "POST" "/users" (req) {
    let body = req.body
    return { "created": true }
}

// With rate limiting and middleware
route "GET" "/api/data" (req) limit 100/minute use [auth] {
    return { "data": "protected" }
}
```

### Scheduled Tasks
```serv
every 5s { log.info("tick") }
cron "0 0 * * *" { log.info("midnight") }
```

### Pub/Sub
```serv
subscribe "orders.new" (msg) { log.info("Order: ", msg) }
publish "notifications" "Order confirmed"
```

### Error Handling
```serv
// ? operator (recommended)
let data = fetchData()?

// Multi-return
let val, err = db.query("SELECT * FROM users")
if err != nil { log.error(err) }

// Try/catch
try {
    let result = http.get("https://api.example.com")
} catch (err) {
    log.error("Failed: ", err)
}
```

### Structs & Methods
```serv
struct User {
    name: string,
    email: string?,
    age: int
}

fn User.greet() -> string {
    return f"Hi, I'm {self.name}"
}
```

### Control Flow
```serv
if x > 0 { ... } else { ... }

for item in items { ... }
for key, value in config { ... }
for count > 0 { count -= 1 }

match status {
    "active" => { ... }
    "inactive" => { ... }
    _ => { ... }
}
```

### Testing
```serv
beforeEach { reset() }

test "math works" {
    assert add(2, 3) == 5
}

test "with timeout" timeout 5s {
    assert fetchData() != nil
}
```

## Built-in Objects
- `log.info/warn/error/debug()` — structured logging
- `db.query(sql, ...args)` — database queries
- `cache.get(key)` / `cache.set(key, val, ttl)` — caching
- `http.get(url)` / `http.post(url, body)` — HTTP client
- `json.parse(str)` / `json.stringify(obj)` — JSON
- `time.now()` / `time.unix()` / `time.sleep(ms)` — time
- `env("KEY")` / `config("dotted.key")` — configuration
- `validate(body, schema)` — request validation
- `atomic.new/inc/dec/get/set/cas` — atomic operations
- `channel.new/send/receive/close` — Go channels
- `metric.inc/gauge` — metrics (exposed at /metrics)

## Imports
```serv
import { ok, notFound } from "stdlib/response"
import { requireAuth } from "stdlib/auth"
import uuid from "github.com/google/uuid"
```

## Formatting Rules
- 4-space indentation
- Blank line between top-level declarations
- Always run `serv fmt` before committing

## Type System
- Gradual typing: annotations are optional but recommended
- `T?` for nullable types: `string?` allows nil
- `T | U` for union types: `int | error`
- The compiler warns on type mismatches when annotations are present
