# Serv v1.0.0

The first stable release of Serv — a programming language for building background services, APIs, schedulers, and event-driven applications. Compiles to native binaries via Go.

## Install

```bash
# macOS/Linux
curl -fsSL https://raw.githubusercontent.com/vyuvaraj/Serv-lang/main/release-scripts/install.sh | bash

# Windows (PowerShell)
irm https://raw.githubusercontent.com/vyuvaraj/Serv-lang/main/release-scripts/install.ps1 | iex
```

**Prerequisite:** [Go 1.18+](https://go.dev/dl/) must be installed.

## Quick Start

```bash
serv init myapp
cd myapp
serv run main.srv --watch
# Visit http://localhost:8080/health
```

## What's Included

### Language
- Variables with type inference (`let x = 5` tracks as `int`)
- Functions with typed parameters and return types
- Structs, methods, interfaces, enums
- Generics with constraints (`fn max[T: Ordered](a: T, b: T) -> T`)
- Pattern matching (`match`)
- Optional types (`string?`) and union types (`int | error`)
- Error propagation (`let data = fetchData()?`)
- Arrow functions (`x => x * 2`)
- Destructuring, spread operator, optional chaining
- For loops with break/continue, map iteration (`for key, value in map`)
- Slice expressions (`arr[1:3]`)
- All operators: arithmetic, modulo, bitwise, compound assignment, unary

### Infrastructure (declared, not configured)
- `server "8080"` — HTTP server with auto `/health` and `/ready`
- `database "sqlite://app.db"` — SQLite, PostgreSQL, Oracle, MongoDB
- `cache "redis://localhost:6379"` — Redis or in-memory
- `broker "nats://localhost:4222"` — NATS, Kafka, RabbitMQ, MQTT
- `route` with path params, query params, rate limiting, middleware
- `ws` WebSocket endpoints
- `every` / `cron` scheduled tasks
- `subscribe` / `publish` pub/sub messaging
- `spawn` concurrent workers with pool limits
- `migration` database schema management
- `tool` MCP tool definitions

### Standard Library (46 modules)
Auth, JWT, crypto, HTTP client, pagination, retry, circuit breaker, rate limiting, validation, feature flags, multi-tenancy, audit logging, and more. Import with `import { fn } from "stdlib/module"`.

### Toolchain
- `serv build` — compile to native binary
- `serv run --watch` — hot-reload development
- `serv test --cover` — testing with coverage
- `serv lint` — static analysis (type errors, unused vars, unreachable code)
- `serv fmt` — code formatter
- `serv repl` — interactive shell
- `serv init` — project scaffolding
- `serv add` — Go package FFI declarations
- `serv dockerize` — Dockerfile generation

### IDE Support (VS Code)
- Syntax highlighting for all constructs
- Real-time diagnostics (errors + warnings as you type)
- Autocomplete with 40+ snippet templates
- Hover documentation (symbols + builtins)
- Go-to-definition (cross-file)
- Signature help
- Format-on-save
- Run/Build/Test commands (Ctrl+Shift+R/B/T)

### Static Analysis
- Unused variable warnings
- Missing return detection
- Type mismatch errors (wrong argument types/count)
- Null safety enforcement (`let x: int = nil` is a compile error)
- Unreachable code detection
- Dead import detection

### Runtime
- Error handling via `[2]interface{}` tuples (no panics in user-facing code)
- Graceful shutdown (SIGINT/SIGTERM)
- OpenTelemetry tracing (OTLP/HTTP)
- Structured logging (JSON mode)
- Prometheus metrics at `/metrics`
- Config from YAML + env vars + CLI flags
- Python FFI with worker pool
- TLS support

## Showcase Projects

- **`showcase/task-api/`** — Single-file CRUD API with SQLite, caching, scheduled cleanup (~100 lines)
- **`showcase/order-system/`** — Modular event-driven pipeline: API -> pub/sub -> worker -> notifier

## Downloads

| Platform | Archive |
|----------|---------|
| macOS (Apple Silicon) | `serv-darwin-arm64.tar.gz` |
| macOS (Intel) | `serv-darwin-amd64.tar.gz` |
| Linux (x64) | `serv-linux-amd64.tar.gz` |
| Linux (ARM) | `serv-linux-arm64.tar.gz` |
| Windows (x64) | `serv-windows-amd64.zip` |

Each archive contains the compiler, LSP, runtime source, standard library, and module files — everything needed to compile `.srv` files.
