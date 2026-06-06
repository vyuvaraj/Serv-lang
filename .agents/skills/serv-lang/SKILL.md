---
name: serv-lang
description: Developer guidelines and specifications for the Serv programming language compiler and runtime.
---

# Serv Development Skill

This skill steers development on the **Serv-lang** repository, which includes the compiler, the runtime, example code, and developer scripts.

---

## 1. Product Overview

Serv is a domain-specific programming language (DSL) for building background services, schedulers, event-driven applications, and API microservices. Source files use the `.srv` extension.

- **Compilation Model**: Transpiles `.srv` source files into native Go code, then compiles them into standalone binaries.
- **Key Features**:
  - Declarative syntax for HTTP routes, cron/interval schedulers, pub/sub messaging, database queries, caching, and concurrency.
  - Optional static typing (`int`, `string`, `bool`) that maps directly to Go primitives.
  - Python interoperability via `extern fn` bindings.
  - Built-in test framework (`test` blocks with `assert`).

---

## 2. Project Structure & Key Conventions

- **`main.go`**: CLI entry point.
- **`compiler/`**: The compiler pipeline.
  - `lexer.go`: Tokenizer.
  - `ast.go`: AST node definitions.
  - `parser.go`: Pratt parser (precedence climbing).
  - `codegen.go`: Code generator (AST to Go).
- **`runtime/`**:
  - `runtime.go`: Single-file runtime library linked into generated binaries (HTTP, DB, cache, broker, scheduler, Python bridge).
- **`examples/`**: Numbered example `.srv` programs demonstrating features.
- **`vscode-support/extension/`**: VS Code extension syntax highlighting and configuration.

### Conventions:
- Compiler logic MUST live in the `compiler/` package. Keep parser, lexer, AST, and codegen in separate files.
- The `runtime/` package MUST remain a single-file library (`runtime.go`).
- Generated build artifacts go into `.build/` (gitignored).

---

## 3. Tech Stack & Dependencies

- **Compiler**: Go (1.26.3+)
- **Target**: Native binaries via Go code generation.
- **Primary Dependencies**:
  - `github.com/robfig/cron/v3` (Cron scheduling)
  - `github.com/glebarez/go-sqlite` (SQLite, CGO-free)
  - `github.com/lib/pq` (PostgreSQL)
  - `go.mongodb.org/mongo-driver` (MongoDB)
  - `github.com/redis/go-redis/v9` (Redis cache)
  - `github.com/segmentio/kafka-go` (Kafka broker)
  - `github.com/nats-io/nats.go` (NATS broker)

---

## 4. Performance & Coding Guidelines

To keep generated Go code close to hand-written Go performance:

1. **Avoid `fmt.Sprintf` for Dynamic Equality**:
   - `==` on untyped values should avoid generating `fmt.Sprintf("%v", x) == fmt.Sprintf("%v", y)`. Use `reflect.DeepEqual` or type-switch comparison instead.
2. **Avoid Inline Closures**:
   - Minimize emitting inline IIFEs for dynamic arithmetic operations (e.g. `(func() interface{} { ... })()`). Use pre-compiled runtime helper calls (e.g. `runtime.GetFieldDynamic`, `runtime.Add`) instead.
3. **Type Inference Support**:
   - Track literal types through assignments, function returns, collection elements, and destructuring. Emit native Go ops when types are statically known.
4. **Scoping and Concurrency**:
   - Only wrap maps in `*SafeMap` (mutex per map) if the variable is accessed from multiple goroutines (escape analysis).
   - Use channel-based worker pools instead of standard goroutine spawning for parallel tasks.

---

## 5. Build & Verification Commands

Use the custom developer script `.\dev.ps1` for routine tasks:
- **Build Compiler**: `.\dev.ps1 build`
- **Run Unit Tests**: `.\dev.ps1 test-unit`
- **Run Regression Tests**: `.\dev.ps1 test`
- **Format Code**: `.\dev.ps1 fmt`
- **Lint Code**: `.\dev.ps1 lint`
- **Clean Artifacts**: `.\dev.ps1 clean`
- **Run Full Check**: `.\dev.ps1 all` (build + test-unit + lint)
