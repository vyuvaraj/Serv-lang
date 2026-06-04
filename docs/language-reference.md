# Serv Language Reference

## Program Structure

A Serv program consists of top-level declarations and statements:

```serv
server "8080"                    // Infrastructure
database "sqlite://app.db"       // Database connection
cache "redis://localhost:6379"   // Cache connection
broker "nats://localhost:4222"   // Message broker

// Routes, functions, scheduled tasks, etc.
```

## Variables

```serv
let name = "Alice"               // Type inferred
let age: int = 30                // Explicit type
let { x, y } = point            // Destructuring
let val, err = riskyFunction()   // Multi-return
```

## Types

| Type | Example |
|------|---------|
| `int` | `42` |
| `float` | `3.14` |
| `string` | `"hello"` |
| `bool` | `true`, `false` |
| `nil` | `nil` |
| `[]T` | `[1, 2, 3]` |
| `map` | `{ "key": "value" }` |

### Type Aliases

```serv
type UserID = int
type Email = string
```

## Functions

```serv
// Basic function
fn greet(name) {
    return f"Hello, {name}!"
}

// Typed parameters and return
fn add(a: int, b: int) -> int {
    return a + b
}

// Generic function
fn identity[T](value: T) -> T {
    return value
}

// Generic with constraints
fn max[T: Ordered](a: T, b: T) -> T {
    if a > b { return a }
    return b
}

// Arrow functions (closures)
let double = x => x * 2
let add = fn(a, b) { return a + b }
```

### Generic Constraints

| Constraint | Supports |
|-----------|----------|
| `Comparable` | `==`, `!=` |
| `Ordered` | `<`, `>`, `<=`, `>=` |
| `Numeric` | `+`, `-`, `*`, `/` |
| `Integer` | Integer arithmetic |
| `Float` | Floating point |

## Control Flow

### If/Else

```serv
if condition {
    // ...
} else if other {
    // ...
} else {
    // ...
}
```

### For Loops

```serv
// Range-based
for item in items {
    log.info(item)
}

// Condition-based
for count < 10 {
    count = count + 1
}
```

### Match (Pattern Matching)

```serv
match status {
    "active" => { log.info("Active") }
    "inactive" => { log.info("Inactive") }
    _ => { log.info("Unknown") }
}
```

## Structs

```serv
struct User {
    name: string,
    email: string,
    age: int
}

// Methods
fn User.greet() -> string {
    return f"Hi, I'm {self.name}"
}

// Instantiation
let user = User { name: "Alice", email: "a@test.com", age: 30 }
log.info(user.greet())
```

## Enums

```serv
// Simple (string values)
enum Color { Red, Green, Blue }

// With explicit values
enum HttpStatus {
    OK = 200,
    NotFound = 404,
    ServerError = 500
}
```

## Interfaces

```serv
interface Serializable {
    fn serialize() -> string
    fn deserialize(data: string)
}
```

## HTTP Routes

```serv
route "GET" "/users" (req) {
    return { "users": [] }
}

route "POST" "/users" (req) {
    let body = req.body
    return { "created": true }
}

// With rate limiting
route "GET" "/api/data" (req) limit 100/minute {
    return { "data": "limited" }
}

// With middleware
route "GET" "/protected" (req) use [auth, logging] {
    return { "secret": "data" }
}
```

### Request Object

| Field | Type | Description |
|-------|------|-------------|
| `req.body` | string | Request body (JSON string) |
| `req.method` | string | HTTP method |
| `req.path` | string | URL path |
| `req.params` | map | URL params + headers |

## WebSockets

```serv
ws "/chat" (conn) {
    for true {
        let msg = conn.receive()
        if msg == nil { break }
        conn.send(f"Echo: {msg}")
    }
}
```

## Scheduled Tasks

```serv
// Fixed interval
every 5s {
    log.info("Tick")
}

// Cron expression
cron "0 0 * * *" {
    log.info("Midnight job")
}
```

## Pub/Sub Messaging

```serv
// Subscribe to a topic
subscribe "orders.new" (msg) {
    log.info("New order: ", msg)
}

// Publish a message
publish "notifications" "Order confirmed"
```

## Concurrency

```serv
// Fire and forget
spawn processOrder(order)

// With worker pool limit
spawn(5) heavyTask(data)

// Async/await
let result = await fetchData()
let all = await all([task1(), task2(), task3()])
```

## Error Handling

```serv
try {
    let result = http.get("http://api.example.com/data")
    log.info(result.body)
} catch (err) {
    log.error("Failed: ", err)
}

// Multi-return error handling
let data, err = riskyCall()
if err != nil {
    log.error(err)
}
```

## Middleware

```serv
middleware auth(req) {
    let token = req.params.authorization
    if token == nil {
        return { "error": "Unauthorized", "status": 401 }
    }
}

route "GET" "/protected" (req) use [auth] {
    return { "data": "secret" }
}
```

## Optional Chaining

```serv
let city = user?.address?.city    // nil if any part is nil
```

## Spread Operator

```serv
let defaults = { "timeout": 30, "retries": 3 }
let config = { ...defaults, "timeout": 60 }
```

## Imports & Modules

```serv
// Import a local .srv module
import "models/user.srv"
import { User, Role } from "models/user.srv"

// Import a Go package
import uuid from "github.com/google/uuid"
let id = uuid.New()

// Use stdlib modules
import { ok, notFound } from "../stdlib/response.srv"
```

## External Function Bindings

```serv
// Go package
extern fn generateID() from "go:github.com/google/uuid:NewString"

// Python script
extern fn analyze(data) from "python:./scripts/analyzer.py:analyze"
```

## Testing

```serv
test "math works" {
    let result = add(2, 3)
    assert result == 5
}

test "string methods" {
    assert "hello".toUpper() == "HELLO"
    assert "  hi  ".trim() == "hi"
}
```

## Config Validation

```serv
validate {
    required "db.host",
    required "db.port",
    optional "log.level"
}
```

## Request Validation

```serv
let errors = validate(req.body, {
    "email": "required,email",
    "name": "required,string",
    "age": "int"
})
```

## Migrations

```serv
migration "create_users" {
    db.query("CREATE TABLE users (id INTEGER PRIMARY KEY, name TEXT)")
}
```

## MCP Tools

```serv
tool "calculator" "Performs math operations" (args) {
    let result = args.a + args.b
    return { "result": result }
}
```
