# Serv: A Programming Language for Background Services

Serv is a modern, high-level DSL (Domain-Specific Language) designed specifically for building background services, schedulers, event-driven applications, and API microservices. It compiles directly into native binaries via Go code generation, providing high performance, low resource consumption, and rapid development.

---

## Table of Contents
- [Key Features](#key-features)
- [Getting Started](#getting-started)
- [Editor Support](#editor-support)
- [CLI Commands Reference](#cli-commands-reference)
- [Language Syntax Guide](#language-syntax-guide)
- [Web Playground](#web-playground)
- [Standard Library](#standard-library)
- [Package Management](#package-management)
- [Testing Support](#testing-support)
- [Compilation & Deployment](#compilation--deployment)
- [Documentation](#documentation)

---

## Key Features

- **Declarative Infrastructure**: Routes, schedulers, pub/sub, databases, caches, and WebSockets as language keywords — not library calls.
- **Compiles to Native Binaries**: Go code generation → single binary deployment. No runtime dependencies.
- **Optional Static Typing**: Gradual type system with `int`, `float`, `string`, `bool`, optional types (`T?`), union types (`T | error`), and generics with constraints.
- **48 Standard Library Modules**: Auth, JWT, retry, circuit breaker, pagination, CORS, rate limiting, validation, and more — written in Serv itself.
- **Built-in Test Framework**: `test "name" { assert expr }` blocks with structured assertion messages.
- **Multiple Database Backends**: SQLite, PostgreSQL, Oracle, MongoDB — same `db.query()` API.
- **Multiple Broker Backends**: Kafka, NATS, RabbitMQ, MQTT — same `subscribe`/`publish` syntax.
- **Concurrency Primitives**: `spawn`, `async`/`await`, channels, worker pools.
- **Middleware & Auth**: Declarative middleware with `use [auth, logging]` on routes.
- **Python Interop**: Call Python scripts via `extern fn` bindings.
- **Go Package FFI**: Import any Go package with `serv add <package>` and auto-generated declarations.
- **VS Code Extension**: Full LSP with diagnostics, autocomplete, hover, go-to-definition, and 30+ snippets.
- **OpenTelemetry & Prometheus**: Built-in tracing and metrics export.
- **Docker Support**: `serv dockerize` generates production-ready Dockerfiles.

---

## Getting Started

### Prerequisites
- **Go**: Version 1.22 or higher is required to build the compiler and execute Go-transpiled code.
- **Python 3.x**: Optional (needed if using Python external functions).

### Install via Scoop (Windows)
```powershell
scoop bucket add serv https://github.com/vyuvaraj/scoop-serv
scoop install serv
```

### Install via Script (Windows)
```powershell
irm https://raw.githubusercontent.com/vyuvaraj/Serv-lang/main/release-scripts/install.ps1 | iex
```

### Build from Source
```bash
git clone https://github.com/vyuvaraj/Serv-lang.git
cd Serv-lang
go build -o serv.exe .
```

Add the binary to your system PATH for global access.

---

## Editor Support

### VS Code Extension
Install **Serv Language Support** from the VS Code Marketplace (or from `.vsix` in the repo):
- Syntax highlighting for `.srv` files
- Real-time diagnostics (type errors, unused variables, missing returns)
- Autocomplete and hover information
- Go-to-definition across files
- 30+ code snippets (`route`, `fn`, `struct`, `test`, `every`, `subscribe`, etc.)
- Commands: Run (`Ctrl+Shift+R`), Build (`Ctrl+Shift+B`), Test (`Ctrl+Shift+T`)
- Format on save

---

## CLI Commands Reference

| Command | Description |
|---------|-------------|
| `serv build <file.srv> [-o output]` | Compile to native binary |
| `serv run <file.srv> [--watch]` | Compile and run (with optional hot-reload) |
| `serv test <file.srv> [--cover] [--filter name]` | Run test blocks |
| `serv lint <file.srv>` | Check syntax and static analysis |
| `serv fmt <file.srv> [--check]` | Format code (4-space indent) |
| `serv repl` | Interactive shell |
| `serv add <go-package>` | Generate `.srv.d` declaration for a Go package |
| `serv packages` | List installed package declarations |
| `serv remove <package>` | Remove a package declaration |
| `serv install <name>` | Install a community package |
| `serv publish <dir>` | Publish a package to the registry |
| `serv init [name]` | Create a new Serv project |
| `serv dockerize <file.srv>` | Generate a production Dockerfile |
| `serv debug <file.srv>` | Debug with Delve |
| `serv audit` | Audit Go dependencies for vulnerabilities |

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

### S3 & ServStore Client Operations (`s3`)

Interact with S3-compatible endpoints or a ServStore gateway using the native `s3` runtime functions. You can also import the helper wrapper from the standard library:

```serv
import { newClient, put, get, deleteObject, list, at, search } from "stdlib/s3.srv"

// Initialize client
let client = newClient("http://localhost:8080", "admin", "adminsecret")

// Create and configure a bucket
client.createBucket("my-bucket")
client.setBucketVersioning("my-bucket", true)

// Upload and retrieve objects
client.put("my-bucket", "config.json", "{\"status\": \"active\"}")
let content = client.get("my-bucket", "config.json")
log.info("Content: ", content)

// Time-travel to retrieve previous versions of an object (ServStore only)
let historicalContent = client.at("my-bucket", "config.json", "2026-06-15T09:00:00Z")

// Perform semantic search queries (ServStore only)
let searchResults = client.search("my-bucket", "find active config files", 5)
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

## Web Playground

Serv includes an interactive Web Playground for trying the language in-browser.

- **WASM Compiler**: Syntax analysis and formatting run client-side
- **Sandbox Runner**: Compiles and executes code server-side with auto-termination

```bash
go build -o web_playground/server/server.exe web_playground/server/main.go
./web_playground/server/server.exe
# Open http://localhost:8080
```

---

## Standard Library

Serv ships with 48 importable modules written in Serv itself:

| Category | Modules |
|----------|---------|
| **Auth & Security** | auth, jwt, crypto, cors, sanitize, ip |
| **Resilience** | retry, circuit_breaker, timeout, semaphore, dlq |
| **HTTP** | http_client, response, middleware, ratelimit, webhook |
| **Data** | validation, pagination, pagination_cursor, csv, diff, sort, collections |
| **Config & Env** | config, env, feature_flags |
| **Observability** | tracing, metrics, health, audit |
| **Utilities** | strings_util, datetime, math, url, base64, mask, idempotency, batch, queue |
| **Infra** | s3, cache_patterns, tenant, scheduler, job, graceful |

Import with:
```serv
import { hashPassword, verifyPassword } from "stdlib/crypto"
import { ok, notFound, created } from "stdlib/response"
import { retry } from "stdlib/retry"
```

---

## Package Management

### Publishing
```bash
serv publish <package-dir>
```

### Installing
```bash
serv install <package-name>
```

### Using
```serv
import { Helper, helperFunc } from "mypkg"
```
Resolves to `packages/mypkg/index.srv` or `packages/mypkg/main.srv`. Only `export`-marked declarations are accessible.

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

---

## Documentation

- [Language Reference](docs/language-reference.md) — Full syntax and type system
- [Getting Started](docs/getting-started.md) — First project walkthrough
- [Standard Library](docs/stdlib.md) — All 48 modules documented
- [Built-in Functions](docs/builtins.md) — `log`, `db`, `cache`, `http`, `json`, `metric`
- [CLI Reference](docs/cli.md) — All commands and flags
- [Deployment Guide](docs/deployment.md) — Docker, TLS, observability
- [Examples](examples/) — 42 working examples covering all features

---

## License

Apache 2.0 — see [LICENSE](LICENSE)

---

## Links

- **GitHub**: [github.com/vyuvaraj/Serv-lang](https://github.com/vyuvaraj/Serv-lang)
- **VS Code Extension**: Search "Serv Language Support" in Extensions
- **Issues**: [github.com/vyuvaraj/Serv-lang/issues](https://github.com/vyuvaraj/Serv-lang/issues)
