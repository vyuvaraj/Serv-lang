# Serv: A Programming Language for Background Services

Serv is a modern, high-level DSL (Domain-Specific Language) designed specifically for building background services, schedulers, event-driven applications, and API microservices. It compiles directly into native binaries via Go code generation, providing high performance, low resource consumption, and rapid development.

---

## Table of Contents
- [Key Features](#key-features)
- [Getting Started](#getting-started)
  - [Prerequisites](#prerequisites)
  - [Building the Compiler](#building-the-compiler)
- [CLI Commands Reference](#cli-commands-reference)
- [Language Syntax Guide](#language-syntax-guide)
  - [Core Architecture Statements](#core-architecture-statements)
  - [Schedulers (`every` & `cron`)](#schedulers-every--cron)
  - [Web Servers & HTTP APIs (`route`)](#web-servers--http-apis-route)
  - [Pub/Sub Broker (`publish` & `subscribe`)](#pubsub-broker-publish--subscribe)
  - [Concurrency & Worker Pools (`spawn`)](#concurrency--worker-pools-spawn)
  - [Database Operations (`db.query`)](#database-operations-dbquery)
  - [Cache Operations (`cache.set` & `cache.get`)](#cache-operations-cacheset--cacheget)
  - [Python Interoperability (`extern fn`)](#python-interoperability-extern-fn)
  - [Built-in Functions & Utilities](#built-in-functions--utilities)
- [Testing Support](#testing-support)
- [Compilation & Deployment](#compilation--deployment)

---

## Key Features

- **Optional Static Typing & Compiler Optimizations**: Declare static types (`int`, `string`, `bool`) to generate native Go code, bypass interface reflection overhead, and execute mathematical/logic operations directly on Go primitives.
- **High-Performance Pub/Sub**: Channel-decoupled event queue with a pool of 20 concurrent background workers for low latency and zero blockages.
- **Declarative Background Workflows**: Native syntax for defining periodic intervals and cron schedules.
- **Built-in HTTP & Pub/Sub Routing**: Directly declare endpoints, routing, and message queue subscriptions.
- **Multi-Database Support**: Out-of-the-box integration with SQLite, PostgreSQL, Oracle, and MongoDB.
- **Embedded Cache**: Native in-memory key-value caching.
- **Simple Concurrency**: Spawn asynchronous threads or rate-limited worker pools with a single keyword.
- **Python Extensibility**: Seamless bindings to execute Python scripts directly from Serv code.
- **Modern Syntax**: Includes string interpolation, pattern matching (`match`), and exception handling (`try-catch`).
- **First-class Testing**: Write and run unit tests natively using the test framework.

---

## Getting Started

### Prerequisites
- **Go**: Version 1.18 or higher is required to build the compiler and execute Go-transpiled code.
- **Python 3.x**: Optional (needed if using Python external functions).

### Building the Compiler
To build the Serv compiler from source, clone or navigate to the repository directory and run:
```bash
go build -o serv.exe main.go
```
This compiles the Serv CLI (`serv.exe`). Add this binary to your system PATH for global access.

---

## CLI Commands Reference

Serv provides a comprehensive CLI for compilation, execution, testing, and deployment:

### 1. Compile to Native Binary
Compiles the `.srv` file, generates a native executable, and deletes intermediate builds.
```bash
serv build <file.srv> [-o <output_binary>]
```
*Example:* `serv build main.srv -o app_service.exe`

### 2. Run Immediately
Compiles and starts the service in a single command.
```bash
serv run <file.srv>
```

### 3. Run in Watch Mode (Hot-Reload)
Starts the service and monitors the workspace for file changes. If any `.srv` or `.py` file is modified, the compiler automatically rebuilds and restarts the service.
```bash
serv run <file.srv> --watch
```

### 4. Run Unit Tests
Locates all `test` blocks in the Serv file, generates a temporary test suite, and executes tests with real-time feedback.
```bash
serv test <file.srv>
```

### 5. Generate Dockerfile
Generates a optimized multi-stage `Dockerfile` suitable for containerizing your service.
```bash
serv dockerize <file.srv>
```

---

## Language Syntax Guide

### Core Architecture Statements

Serv allows you to declare global settings and connections dynamically or using values loaded from environment variables:

```serv
// Declare port dynamically from environment variables
server env("PORT")

// Setup global message broker (options: "in-memory", or Kafka address)
broker "in-memory"

// Setup databases (SQLite, PostgreSQL, Oracle, MongoDB)
database "sqlite://service_data.db"
database env("DATABASE_URL")

// Setup in-memory cache
cache "in-memory"
```

---

### Static Typing & Type Annotations

Serv supports optional static typing on variables and function signatures. Providing types compiles them directly into native Go types, skipping the performance overhead of runtime `interface{}` conversions.

Supported types: `int`, `string`, `bool`.

#### Variable Type Annotations
Specify types using `: type` after the identifier:
```serv
let count: int = 100
let label: string = "Items in queue"
let isActive: bool = true
```

#### Function Signature Type Annotations
Specify parameter and return types to optimize function calls and compiler math:
```serv
fn calculateTotal(base: int, tax: int) -> int {
    let result: int = base + tax
    return result
}
```

---

### Schedulers (`every` & `cron`)

Easily define background routines that run periodically or at scheduled times.

#### Interval Scheduler
Runs a block of code at a specific time duration (e.g., `s` for seconds, `m` for minutes, `h` for hours).
```serv
every 5s {
    log.info("System healthcheck running...")
}
```

#### Cron Scheduler
Executes using standard cron patterns. Can load patterns from environment variables.
```serv
cron "0 */2 * * * *" {
    log.info("This runs every 2 minutes.")
}

// Load from environment variable
cron env("BACKUP_CRON") {
    log.warn("Starting system database backup...")
}
```

---

### Web Servers & HTTP APIs (`route`)

Declare HTTP request endpoints with simple routes. Serv handles request body parsing natively.

```serv
route "GET" "/status" (req) {
    log.info("Status check requested")
    return { 
        "status": "Serv is operating normally", 
        "timestamp": time.now() 
    }
}

route "POST" "/webhook" (req) {
    let body = req.body
    log.info("Received body payload: ", body)
    return { "received": true }
}
```

---

### Pub/Sub Broker (`publish` & `subscribe`)

Publish event messages and register subscriptions.

```serv
// Publish message onto a topic channel
publish "events.incoming" { "user_id": 101, "action": "login" }

// Subscribe to messages on a topic
subscribe "events.incoming" (msg) {
    log.info("Broker received event: ", msg)
}
```

---

### Concurrency & Worker Pools (`spawn`)

You can execute operations asynchronously without blocking the main workflow thread.

#### Fire-and-Forget Goroutines
```serv
subscribe "incoming.tasks" (msg) {
    // Spawns a lightweight concurrent thread
    spawn processTask(msg)
}
```

#### Rate-Limited Worker Pools
Specify a worker limit to control resource consumption:
```serv
// Spawns up to 5 concurrent workers maximum
spawn(5) handleHeavyCalculation(data)
```

---

### Database Operations (`db.query`)

Execute queries directly on the configured databases.

#### SQL Databases (SQLite, PostgreSQL, Oracle)
Supports query parsing and placeholders (`?` translates automatically to appropriate placeholders like `$1` dynamically for PostgreSQL).
```serv
// Create schema table on startup
db.query("CREATE TABLE IF NOT EXISTS metrics (id INTEGER PRIMARY KEY, ts TEXT)")

// Insert records
db.query("INSERT INTO metrics (ts) VALUES (?)", time.now())

// Read records
let results = db.query("SELECT * FROM metrics LIMIT 5")
log.info("Metrics: ", results)
```

#### MongoDB Operations
Executes collection queries using standardized document queries:
```serv
let result = db.query("insert", "logs", "{\"service\": \"Serv\", \"action\": \"db_test\"}")
```

---

### Cache Operations (`cache.set` & `cache.get`)

Leverage native in-memory caching to save and read states quickly:

```serv
// Set key with cache TTL (Time to Live)
cache.set("session_user_1", { "id": 1, "role": "admin" }, "10m")

// Fetch value from cache
let session = cache.get("session_user_1")
log.info("Active Session: ", session)
```

---

### Python Interoperability (`extern fn`)

Map complex algorithms or specialized Python libraries directly to Serv functions:

```serv
// Map external Python method
extern fn analyzeText(text) from "python:./scripts/analyzer.py:analyze"

let result = analyzeText("Hello world!")
log.info("Python output: ", result)
```

---

### Built-in Functions & Utilities

#### JSON Support
```serv
let obj = json.parse("{\"status\": true}")
let rawString = json.stringify(obj)
```

#### String Interpolation (f-strings)
```serv
let name = "Serv"
let statusMessage = f"System: {name} is running!"
```

#### Pattern Matching (`match`)
```serv
match eventType {
    "PAYMENT_COMPLETED" => {
        log.info("Processing checkout success...")
    }
    "USER_LOGOUT" => {
        log.info("Cleaning session...")
    }
    _ => {
        log.warn("Unknown event category received")
    }
}
```

#### Exception Handling (`try-catch`)
```serv
try {
    let res = http.get("http://invalid-url.com")
} catch (err) {
    log.error("HTTP request failed: ", err)
}
```

---

## Web Playground & Sandbox (Phase 9.3)

Serv provides an interactive Web Playground where you can write, format, compile, and execute Serv scripts in your web browser.

### Key Features
- **In-Browser WASM Compiler**: Syntax analysis, warning diagnostics, and formatting (`serv fmt`) run entirely client-side using WebAssembly.
- **Backend Sandbox Runner**: Compiles the code to a native binary in a sandboxed directory and executes it.
- **Auto-Termination**: Long-running background services (HTTP servers, cron schedulers) are dynamically stopped after 1.5 seconds to capture initial output logs and release OS socket handles instantly.

### Running Locally
To launch the Web Playground locally, run:
```bash
# Build the playground server binary
go build -o web_playground/server/server.exe web_playground/server/main.go

# Start the server (default port: 8080)
./web_playground/server/server.exe
```
Then navigate to `http://localhost:8080` in your web browser.

---

## Community Package Registry (Phase 9.5)

Serv includes a built-in package distribution tool to publish modules to the registry and install third-party dependencies locally.

### 1. Publishing a Package
Bundle a directory containing `.srv` files and upload it to the registry:
```bash
serv publish <package-dir>
```

### 2. Installing a Package
Download a package from the registry and install it into a local `packages/` directory:
```bash
serv install <package-name>
```

### 3. Importing Packages
Import installed modules in your code using non-relative imports:
```serv
import { Helper, helperFunc } from "mypkg"

let instance = Helper { val: 42 }
let msg = helperFunc()
```
Imports will automatically resolve to `packages/mypkg/index.srv` or `packages/mypkg/main.srv` with strict visibility checks. Only declarations marked with `export` are accessible outside the module.

---

## Testing Support

Serv includes a native test harness built into the language itself. This makes it trivial to write unit tests alongside your code and verify logic without external framework setups.

### Defining Tests
Add `test` blocks and use the `assert` statement to check variables:
```serv
fn doubleValue(val) {
    return val * 2
}

test "doubling math verification" {
    assert doubleValue(2) == 4
    assert doubleValue(5) == 10
}

test "check string comparison" {
    let val = "Serv" + "Lang"
    assert val == "ServLang"
}
```

### Running Tests
Execute:
```bash
serv test test_sample.srv
```

*Output:*
```
Running tests from test_sample.srv...
=== RUN   Test_DoublingMathVerification
--- PASS: Test_DoublingMathVerification (0.00s)
=== RUN   Test_CheckStringComparison
--- PASS: Test_CheckStringComparison (0.00s)
PASS
ok  	serv/.build	1.518s
```

---

## Compilation & Deployment

When `serv build` or `serv test` is executed, the compiler compiles the input `.srv` code into a temporary directory called `.build`.

Inside `.build`:
1. `service.go`: Synthesizes code for all declarations, routes, and background routines.
2. `main.go`: Provides the service runtime engine and entry points.
3. `serv_test.go`: Aggregates the `test` blocks translated to Go's native testing framework.

The output binary compiles out all debug logs and features a fast, low-overhead native runtime engine.
