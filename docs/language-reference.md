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
| `T?` | Optional (nullable) type |
| `T \| U` | Union type |

### Type Aliases

```serv
type UserID = int
type Email = string
```

### Optional Types (Null Safety)

Types suffixed with `?` allow `nil` values. Without `?`, assigning `nil` is a compile error.

```serv
let name: string = "Alice"     // Cannot be nil
let email: string? = nil       // OK — optional type

fn findUser(id: int) -> User? {
    let row = db.query("SELECT * FROM users WHERE id = ?", id)
    if row == nil { return nil }
    return User { name: row.name }
}
```

**Compile error example:**
```serv
let x: int = nil   // error: cannot assign nil to non-optional type 'int' (use 'int?' to allow nil)
```

### Union Types

Union types allow a value to be one of several types:

```serv
fn divide(a: int, b: int) -> int | error {
    if b == 0 {
        return "division by zero"
    }
    return a / b
}

fn process(input: string | int) {
    log.info(input)
}
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

// Key-value iteration (maps)
for key, value in config {
    log.info(f"{key} = {value}")
}

// Condition-based
for count < 10 {
    count += 1
}
```

### Break & Continue

```serv
for item in items {
    if item == nil { continue }
    if item == "stop" { break }
    log.info(item)
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

## Operators

### Arithmetic

| Operator | Description | Example |
|----------|-------------|---------|
| `+` | Addition / concatenation | `a + b` |
| `-` | Subtraction | `a - b` |
| `*` | Multiplication | `a * b` |
| `/` | Division | `a / b` |
| `%` | Modulo (remainder) | `a % b` |

### Compound Assignment

```serv
let count = 0
count += 1       // count = count + 1
count -= 1       // count = count - 1
count *= 2       // count = count * 2
count /= 2       // count = count / 2
count %= 3       // count = count % 3
```

### Bitwise Operators

| Operator | Description | Example |
|----------|-------------|---------|
| `&` | Bitwise AND | `a & b` |
| `\|` | Bitwise OR | `a \| b` |
| `^` | Bitwise XOR | `a ^ b` |
| `<<` | Left shift | `a << 2` |
| `>>` | Right shift | `a >> 1` |

### Comparison

| Operator | Description |
|----------|-------------|
| `==` | Equal |
| `!=` | Not equal |
| `<` | Less than |
| `>` | Greater than |
| `<=` | Less than or equal |
| `>=` | Greater than or equal |

### Logical

| Operator | Description |
|----------|-------------|
| `and` | Logical AND |
| `or` | Logical OR |
| `!` | Logical NOT |

## Slice Expressions

```serv
let items = [1, 2, 3, 4, 5]
let first3 = items[0:3]     // [1, 2, 3]
let rest = items[2:]         // [3, 4, 5]
let head = items[:2]         // [1, 2]

let text = "hello world"
let sub = text[0:5]          // "hello"
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
    assert result == 5          // "got X, want 5" on failure
}

test "comparisons" {
    assert 10 > 5               // "10 is not > 5" on failure
    assert "hello" != "world"   // "expected value to not equal world" on failure
}

test "string methods" {
    assert "hello".toUpper() == "HELLO"
    assert "  hi  ".trim() == "hi"
}
```

**Assertion messages:**
- `assert x == 5` → `assertion failed: got 3, want 5`
- `assert x != 0` → `assertion failed: expected value to not equal 0`
- `assert x > 10` → `assertion failed: 5 is not > 10`
- `assert valid` → `assertion failed: expected truthy value, got false`

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
