# Serv Language — Critical Analysis & Roadmap

## Executive Summary

Serv has achieved impressive feature breadth for a service-oriented language project. The core compilation pipeline works, the runtime is functional, and the language covers its target domain (services, schedulers, pub/sub, APIs) well. 

With the recent completions of **Find References & Rename refactoring** (6.4), **Module-Level Visibility Enforcement** (8.3), and the **Escape-Analysis Map Optimization** (3.6), the language is entering a highly professional and robust stage. Almost the entire core roadmap has been successfully implemented.

The roadmap below captures the current status of all features and details the remaining next steps to complete Phase 9 (Distribution & Ecosystem) and harden the language for production.

---

## Critical Analysis of Progress

### Strengths Resolved
- **LSP Capabilities**: Cross-file go-to-definition, semantic hover, signature help, and full find-references/rename refactoring are active.
- **Strict Module Visibility**: Imports now strictly validate named symbols and respect the `export` keyword on selective/wildcard imports, preventing private helper exposure.
- **Escape-Analysis Maps**: Zero mutex/lock overhead on request-local/synchronous maps thanks to escape analysis deciding when to use plain Go maps vs `SafeMap`.
- **Error Propagation**: Return-value error tuples combined with the `?` propagation operator have replaced unidiomatic panics.

### Current Status Table

### Phase 1: Language Foundations
| # | Item | Status | Notes |
|---|------|--------|-------|
| 1.1 | Modulo operator (`%`) | ✅ Done | Lexer/parser support |
| 1.2 | `break` / `continue` | ✅ Done | Loop control flow |
| 1.3 | Compound assignment | ✅ Done | `+=`, `-=`, `*=`, `/=`, `%=` |
| 1.4 | Bitwise operators | ✅ Done | `&`, `|`, `^`, `<<`, `>>` |
| 1.5 | Map iteration | ✅ Done | `for key, val in map` |
| 1.6 | Slice expressions | ✅ Done | `arr[1:3]` |
| 1.7 | String concatenation | ✅ Done | Typed string `+` operator |

### Phase 2: Type System
| # | Item | Status | Notes |
|---|------|--------|-------|
| 2.1 | Type inference (assignments) | ✅ Done | `let x = 5` infers `int` |
| 2.2 | Function return type propagation | ✅ Done | Propagates signature returns |
| 2.3 | Collection element type inference | ✅ Done | Tracks array inner types |
| 2.4 | Struct field type tracking | ✅ Done | Resolves struct members |
| 2.5 | Basic type checking | ✅ Done | Validates parameter/assignment matching |
| 2.6 | Null safety (`T?` optional types) | ✅ Done | Compiles and enforces nullable types |
| 2.7 | Union types (`T | error`) | ✅ Done | Enforced at compile-time |

### Phase 3: Performance
| # | Item | Status | Notes |
|---|------|--------|-------|
| 3.1 | Emit native ops for known types | ✅ Done | Eliminates runtime switches |
| 3.2 | Direct field access for structs | ✅ Done | Direct field emission |
| 3.3 | Skip `toSlice()` for typed arrays | ✅ Done | Direct Go slice ranges |
| 3.4 | Prepared statement cache | ✅ Done | Bounded LRU cache |
| 3.5 | Conditional OTEL compilation | ✅ Done | Avoids telemetry overhead when disabled |
| 3.6 | SafeMap only when shared | ✅ Done | Escape analysis optimizes local maps |

### Phase 4: Error Handling
| # | Item | Status | Notes |
|---|------|--------|-------|
| 4.1 | Replace panic with error returns | ✅ Done | Core APIs use error tuples |
| 4.2 | `?` operator for error propagation | ✅ Done | Short-circuits error returns |
| 4.3 | Result type in stdlib | ✅ Done | Full helper suite in `stdlib/result.srv` |

### Phase 5: Compile-Time Analysis
| # | Item | Status | Notes |
|---|------|--------|-------|
| 5.1 | Unused variable warnings | ✅ Done | Emits compiler warnings |
| 5.2 | Missing return detection | ✅ Done | Enforces returns on typed functions |
| 5.3 | Basic type checking | ✅ Done | Parameter size & mismatch validation |
| 5.4 | Unreachable code detection | ✅ Done | Warns on code after returns/breaks |
| 5.5 | Dead import detection | ✅ Done | Flags unused imports |

### Phase 6: LSP & Tooling
| # | Item | Status | Notes |
|---|------|--------|-------|
| 6.1 | Cross-file go-to-definition | ✅ Done | Global symbols lookup |
| 6.2 | Semantic hover on usage | ✅ Done | Real-time hover details |
| 6.3 | Signature help | ✅ Done | Active parameter help |
| 6.4 | Find references / rename | ✅ Done | Workspace-wide refactoring |
| 6.5 | Semantic diagnostics | ✅ Done | Compiler warnings shown in editor |
| 6.6 | `serv fmt` integration | ✅ Done | Document formatting via LSP |

### Phase 7: Testing Framework
| # | Item | Status | Notes |
|---|------|--------|-------|
| 7.1 | Structured assertions | ✅ Done | Informative failure messages |
| 7.2 | Test isolation | ✅ Done | Dedicated test func scopes |
| 7.3 | Test coverage | ✅ Done | `serv test --cover` statement metrics |
| 7.4 | Setup/teardown | ✅ Done | `beforeEach` / `afterEach` hooks |
| 7.5 | Test timeout | ✅ Done | Per-test timeout enforcement |

### Phase 8: Module System Hardening
| # | Item | Status | Notes |
|---|------|--------|-------|
| 8.1 | Package resolution without relative paths | ✅ Done | Direct stdlib imports |
| 8.2 | Circular dependency detection | ✅ Done | Detects cycles and aborts compilation |
| 8.3 | Module-level visibility enforcement | ✅ Done | Strict selective/wildcard export validation |

### Phase 9: Distribution & Ecosystem
| # | Item | Status | Notes |
|---|------|--------|-------|
| 9.1 | Homebrew formula + Scoop manifest | ✅ Done | Available in release-scripts/ |
| 9.2 | VS Code extension publish | ✅ Done | Publisher scripts configured |
| 9.3 | **Web playground** | ✅ Done | Browser-based Monaco editor with WASM compiler + sandbox runner |
| 9.4 | Docker base image | ✅ Done | `Dockerfile.base` configured |
| 9.5 | **Community package registry** | ✅ Done | CLI install & publish commands + local packages/ folder resolution |

---

## Remaining Work Items

All planned ecosystem features from Phase 9 are now fully complete!

---

## Phase 10: Taking Serv to the Next Level (Future Priorities)

To move Serv beyond a simple microservice tool into a premium, world-class programming language for high-performance distributed systems, we propose the following items for the next phase of evolution:

| # | Item | Effort | Description |
|---|------|--------|-------------|
| 10.1 | **Generics (Parametric Polymorphism)** | Large | ✅ Done — Introduce type-safe generics (e.g., `fn map[T, U](arr: []T)`) to avoid `interface{}` casting on collections. |
| 10.2 | **First-Class Actor Model** | Large | ✅ Done — Formalize `spawn` with local mailboxes, message routing, and supervisor trees for robust concurrency. |
| 10.3 | **Database Schema ORM Generation** | Medium | ✅ Done — Compile schema files into strongly-typed query methods, ensuring compile-time safe database interaction. |
| 10.4 | **Distributed Trace Propagation** | Medium | ✅ Done — Automatically trace HTTP headers, message brokers, and actor spawns with OpenTelemetry context propagation. |
| 10.5 | **AOT Optimization Pass** | Medium | ✅ Done — Build AST optimizations (constant folding, dead branch, unreachable code elimination). |
| 10.6 | **WASM Target Compilation** | Large | ✅ Done — `serv build <file.srv> --target wasm` compiles `.srv` files to WASI-compliant WebAssembly. Supported full log formatting via stderr and stubs/abstractions for all non-wasm runtime components. Verified under ServStore transform pipelines. |
| 10.7 | **Stateful Workflows** | Large | ✅ Done — Introduce native Temporal-like `workflow` blocks with automatic state-checkpointing and resilient task retries. |
| 10.8 | **LSP Debugger (DAP) Support** | Medium | ✅ Done — `serv debug <file.srv>` launches a DAP proxy (stdio) backed by Delve. Translates `.srv` breakpoints ↔ generated Go lines using `// .srv line N` source map comments. Full VS Code support: breakpoints, step, stack frames mapped back to `.srv` source. |
| 10.9 | **Serv-verse Core Integrations** | Large | Develop unified connectors and drivers targeting ServQueue (distributed event bus) and ServGate (API Gateway). |

### Detail on Next-Level Items

#### 1. Generics (Phase 10.1)
- **Goal**: Support parametric polymorphism in functions, structs, and interfaces.
- **Why**: Eliminates manual casting and boxing/unboxing overhead when building reusable utilities (like collections, queues, or functional maps). Emits Go generics (`func Map[T, U any](...)`) directly.

#### 2. First-Class Actor Model (Phase 10.2)
- **Goal**: Introduce Erlang-style lightweight processes with mailboxes and supervisor trees.
- **Why**: Background services frequently need to maintain state machines or run long-standing connections. Combining `spawn`, `channel`, and supervisor actors ensures that crashing background workers can be safely restarted without dropping requests.

#### 3. Database Schema ORM Generation (Phase 10.3)
- **Goal**: Generate strongly-typed data access objects (DAOs) directly from database migration files or schemas.
- **Why**: Writing raw JSON queries like `db.findOne("users", "{\"active\": true}")` is error-prone. Typing these queries (e.g., `db.Users.findMany(active: true)`) provides compile-time safety and autocompletion.

#### 4. WASM Target Compilation (Phase 10.6)
- **Goal**: Add a compilation target to build WASI-compliant WebAssembly binaries (`serv build --target wasm`).
- **Why**: Allows developers to write their server-side compute-near-data transformations directly in the Serv language instead of external languages (like Rust or Go).

#### 5. Stateful Workflows (Phase 10.7)
- **Goal**: Add declarative workflows that checkpoint execution state, allowing long-running orchestrations to survive service restarts or hardware failures transparently.
- **Why**: Critical for business logic orchestration (onboarding, order processing) that span multiple days or services.

#### 6. LSP Debugger Support (Phase 10.8)
- **Goal**: Implement the Debug Adapter Protocol (DAP) to provide breakpoints, step-in/out, and stack traces back inside Serv source files.
- **Why**: Elevates the language developer experience to match enterprise languages.

---

## Phase 11: Developer Tooling and Project Maturity

To prepare Serv-lang for production usage and support larger codebases, the following project structure, testing, and debugging items are added:

| # | Item | Effort | Description | Status |
|---|------|--------|-------------|--------|
| 11.1 | **Project Manifest (`serv.toml`)** | Medium | A configuration file defining project metadata, entry points, environment profiles, and dependency locks. | ✅ Done |
| 11.2 | **Multi-File Compilation** | Medium | Enable compiling whole directories (`serv build ./`) rather than single-file structures. | ✅ Done |
| 11.3 | **Panic Stack Trace Mapping** | Medium | Map Go runtime panic traces back to original `.srv` line numbers using emitted source map comments. | ✅ Done |
| 11.4 | **Structured Mocking in Tests** | Medium | Add support to stub out network calls (`mock http.get`) and database operations (`mock db.query`) inside test blocks. | ✅ Done |
| 11.5 | **Scoped Symbol Table Refactor** | Large | Refactor the compiler internals to use proper lexical scopes instead of a simple flat symbol table. | ✅ Done |
| 11.6 | **Environment Profiles** | Small | Support loading environment-specific variables and configuration files based on flag (e.g., `--env staging`). | ✅ Done |

### Detail on Phase 11 Items

#### 1. Project Manifest & Multi-File Projects (Phase 11.1 & 11.2)
- **Goal**: Support standard repository structures where a single service consists of multiple files and modular folders, managed by a root `serv.toml` file.
- **Why**: Larger services are impossible to maintain in single-file scripts or flat imports. A project manifest organizes modules and ensures clean build artifacts.

#### 2. Panic Stack Trace Mapping (Phase 11.3)
- **Goal**: Read generated Go stack traces and rewrite them on the fly to refer back to `.srv` line numbers.
- **Why**: Greatly simplifies the debugging process when runtime panics occur in deployed environments.

#### 3. Structured Mocking (Phase 11.4)
- **Goal**: Allow test definitions to override builtin calls temporarily during the scope of a test block.
- **Why**: Unblocks true hermetic unit testing without needing real database connections or external API servers active.

---

## Phase 12: Servverse Native Integration (New — June 2026)

These items complete the compiler→ecosystem loop defined in Phase 10.9 and extend Serv-lang to be a first-class citizen in multi-service Servverse deployments.

| # | Item | Effort | Description |
|---|------|--------|-------------|
| 12.1 | **`servqueue://` compiler connector** | Large | ✅ Done — Native URI driver for ServQueue STOMP. Enables `broker "servqueue://host"` from `.srv` code without HTTP boilerplate. Extends `runtime/broker.go`. |
| 12.2 | **`servgate://` route registration** | Medium | ✅ Done — Self-announce service routes to ServGate at startup via compiler-emitted registration call. Enables zero-config routing in a Servverse deployment. |
| 12.3 | **`serv deploy --target k8s`** | Medium | ✅ Done — Generate Kubernetes Deployment + Service YAML from `serv.toml` project manifest. |
| 12.4 | **`serv deploy --target fly`** | Small | ✅ Done — Generate `fly.toml` and trigger Fly.io deployment. |
| 12.5 | **`serv new <template>`** | Small | ✅ Done — Starter project scaffolding — `api`, `worker`, `event-processor`, `full-stack`, `microservice`. |
| 12.6 | **`serv-ai` adapter** | Large | ✅ Done — `ai "openai://gpt-4"`, `ai "anthropic://claude-3"`, `ai "ollama://localhost"` connection strings. Exposes `ai.complete()` and `ai.embed()` APIs. Pairs with ServStore semantic search. |
| 12.7 | **`serv monitor`** | Medium | ✅ Done — Terminal htop-style runtime inspector. Shows live request rate, latency percentiles, goroutine count, and route-level breakdown. Fills gap before ServMetrics exists. |

---

## Phase 13: Adapter Expansion & Developer Experience (Proposed — Q3 2026)

To align with the "Adapters First, Platform Second" strategy and widen the addressable developer audience, the following items extend the runtime adapter layer and improve onboarding:

| # | Item | Effort | Description | Status |
|---|------|--------|-------------|--------|
| 13.1 | **`auth` keyword & adapter** | Medium | `auth "keycloak://host/realm"`, `auth "auth0://domain"`, `auth "oidc://issuer"` connection strings. Middleware auto-validates tokens via configured provider. | [x] |
| 13.2 | **`search` keyword & adapter** | Medium | `search "meilisearch://host:7700/index"`, `search "elastic://host:9200/index"` with `search.query()` and `search.index()` APIs. | [ ] |
| 13.3 | **`mail` keyword & adapter** | Small | `mail "smtp://host:587"`, `mail "ses://us-east-1"`, `mail "sendgrid://key"` with `mail.send()` API. | [x] |
| 13.4 | **MySQL database adapter** | Small | Add MySQL driver to `runtime/db.go` via `database "mysql://..."` connection string. | [x] |
| 13.5 | **Turso/libSQL database adapter** | Small | `database "turso://db.turso.io"` — emerging edge database, high adoption signal. | [ ] |
| 13.6 | **Redis Streams broker adapter** | Small | `broker "redis-stream://host:6379"` — common lightweight event streaming alternative. | [x] |
| 13.7 | **`store` keyword (multi-backend)** | Medium | `store "s3://bucket"`, `store "gcs://bucket"`, `store "r2://bucket"`, `store "local://./uploads"` with unified `store.put()` / `store.get()` API. | [x] |
| 13.8 | **Canonical `serv.toml` example** | Small | Add a well-documented example `serv.toml` to the repo root showing multi-file projects, env profiles, and dependency locks. | [x] |
| 13.9 | **VS Code Extension marketplace publish** | Small | Register publisher, package, and publish to Visual Studio Marketplace. Highest ROI discoverability item. | [x] |
| 13.10 | **Graceful shutdown in runtime** | Small | `signal.NotifyContext` pattern in generated `main.go` — drain connections, flush spans, close DB pools on SIGTERM. | [x] |
| 13.11 | **Standardized error response contract** | Small | All generated HTTP handlers return `{"error": "msg", "code": "ERR_CODE", "trace_id": "..."}` on failure. | [x] |
| 13.12 | **API versioning helpers** | Small | `route "GET" "/v1/users"` grouping via `version "v1" { ... }` block syntax or stdlib helper. | [ ] |
| 13.13 | **Full OIDC discovery & JWKS validation** | Medium | For `auth "oidc://issuer"`, auto-fetch `/.well-known/openid-configuration`, cache JWKS public keys, validate RS256/ES256 signatures, and support key rotation. Currently only validates issuer claim. | [x] |
| 13.14 | **Auth role/scope guards** | Small | `route "GET" "/admin" (req) use [auth.role("admin")]` — compile-time syntax for role-based route access using JWT claims. | [x] |

---

## Phase 14: Next-Level Language Evolution (Proposed — Q4 2026+)

These items take Serv from a capable service language to a **category-defining** platform language. Each unlocks a new class of use case or developer segment.

| # | Item | Effort | Description | Status |
|---|------|--------|-------------|--------|
| 14.1 | **Compile-time dependency injection** | Large | Declare service dependencies as interfaces, auto-wire implementations via `serv.toml` bindings. Enables testable architectures without runtime reflection. | [ ] |
| 14.2 | **Hot-reload without restart (`serv run --hot`)** | Large | On `.srv` file save, recompile and swap the running binary via SO/DLL injection or process replacement with socket handoff. Zero-downtime local development. | [ ] |
| 14.3 | **OpenAPI spec auto-generation** | Medium | `serv docs generate` emits a complete OpenAPI 3.1 spec from route declarations, request/response types, and middleware annotations. | [x] |
| 14.4 | **Client SDK code generation** | Large | `serv generate client --lang typescript` / `--lang python` / `--lang go` emits typed API client libraries from route declarations. No OpenAPI intermediary needed. | [x] |
| 14.5 | **Incremental compilation** | Large | Cache AST and codegen artifacts per-file. Only recompile changed files and their dependents. Critical for large multi-file projects (>50 files). | [ ] |
| 14.6 | **Effect system (side-effect tracking)** | Large | Annotate functions as `pure`, `io`, or `async`. Compiler enforces that `pure` functions cannot call `io` functions. Enables safe parallelization and easier testing. | [ ] |
| 14.7 | **`pipe` operator** | Small | `data |> transform() |> validate() |> save()` — sugar for function chaining. High readability for data transformation pipelines. | [ ] |
| 14.8 | **Pattern matching on types** | Medium | `match value { case s: string => ..., case n: int => ..., case User { name } => ... }` — destructuring match with type narrowing. | [ ] |
| 14.9 | **Compile-time macros** | Large | `@derive(Serialize, Validate)` annotations that generate boilerplate at compile time — similar to Rust derive or Java annotation processors. | [ ] |
| 14.10 | **REPL with hot service context** | Medium | `serv repl --attach localhost:8080` connects to a running service and evaluates expressions against live state — inspect DB, cache, and variables interactively. | [ ] |
| 14.11 | **Language-level circuit breaker** | Small | `resilient fn callPayment() retries 3 timeout 5s circuit_breaker { ... }` — declarative resiliency annotations on function signatures, compiled to runtime wrappers. | [ ] |
| 14.12 | **Streaming response support** | Medium | `route "GET" "/events" (req) stream { yield { data: "ping" }; every 1s { yield heartbeat() } }` — SSE/chunked streaming as a first-class route type. | [x] |
| 14.13 | **GraphQL endpoint declaration** | Large | `graphql "/api" { type Query { users: [User] } resolver users() { ... } }` — native GraphQL schema + resolver syntax compiled to a performant Go handler. | [ ] |
| 14.14 | **Cross-compilation targets** | Medium | `serv build --os linux --arch arm64` — cross-compile from any host to any Go-supported target. Enables edge/IoT deployment from a single dev machine. | [ ] |
| 14.15 | **Language server code actions** | Medium | Quick-fix suggestions in the LSP: "Extract to function", "Add error handling", "Generate test stub", "Wrap in try/catch". Moves beyond diagnostics into active refactoring assistance. | [ ] |

---

## Phase 15: Differentiating Factors — What No Other Language Offers (Strategic)

These are features that create a **moat** around Serv — capabilities that competing languages and frameworks fundamentally cannot replicate without rebuilding from scratch. Each exploits Serv's unique position as a compiled DSL with full control over code generation.

| # | Item | Effort | Description | Why Nobody Else Can Do This |
|---|------|--------|-------------|----------------------------|
| 15.1 | **AI-assisted code generation at compile time** | Large | The compiler calls an LLM during compilation to auto-generate boilerplate: `route "GET" "/users" (req) @ai.implement { // describe intent }`. Generates handler body from natural language docstring at build time (not runtime). | Serv controls the codegen pipeline — general-purpose compilers can't insert AI steps. |
| 15.2 | **Compile-time contract verification** | Large | The compiler statically verifies that every route handler matches its declared request/response schema. A type mismatch between the route annotation and the return value is a compile error — like TypeScript for APIs but enforced at binary generation. | Only possible in a DSL with route + type info available during compilation. |
| 15.3 | **Automatic chaos testing injection** | Medium | `serv test --chaos` adds random latency, failures, and network errors into `db.query()`, `http.get()`, and `broker.publish()` calls during test execution — without modifying source code. Compiler knows all infra call sites. | Compiler has full knowledge of every infra callsite. General-purpose languages need external proxies. |
| 15.4 | **Zero-config distributed tracing (no SDK)** | Small | Every `route`, `subscribe`, `spawn`, and `every` block automatically gets OTel spans with correct parent-child relationships. No import, no SDK initialization, no manual context passing — the compiler inserts it. | Only achievable when the compiler owns the concurrency and I/O primitives. Go/Java/Python need manual instrumentation. |
| 15.5 | **Infra-aware dead code elimination** | Medium | If no `database` declaration exists, all `db.*` runtime code is excluded from the binary. If no `broker` is declared, pub/sub code is gone. Binary size matches actual usage — not maximum feature set. | Compiler knows the full dependency graph of language primitives → runtime modules. |
| 15.6 | **Deployment manifest inference** | Medium | `serv deploy --target k8s` reads the `.srv` source: sees `database`, emits a PersistentVolumeClaim; sees `cache`, emits a Redis sidecar; sees `every`, emits a CronJob. Infrastructure requirements inferred from source code — no manual YAML. | Source code IS the infra specification. No other language can infer infra from syntax. |
| 15.7 | **Live type narrowing from database schema** | Large | `database "postgres://..."` at compile time connects to the live database, reads the schema, and provides typed completion for `db.query()` results. `let user = db.query("SELECT * FROM users WHERE id = ?", id)` — `user.email` is auto-typed as `string?` from the schema. | The compiler is the database client. General-purpose languages need separate ORM codegen steps. |
| 15.8 | **Cross-service type safety** | Large | When Service A defines `route "POST" "/orders" (req: OrderRequest)` and Service B calls `http.post("serv://service-a/orders", payload)`, the compiler verifies `payload` matches `OrderRequest` at compile time — across repositories using published `.srv.d` declarations. | Cross-service contracts checked at compile time. Impossible in HTTP-based systems without shared type registries. |
| 15.9 | **Automatic API versioning from git history** | Medium | `serv docs diff v1.2.0..v1.3.0` compares route declarations between git tags and generates a changelog of API breaking changes, new endpoints, and deprecated routes. Compile-time breaking change detection. | Compiler understands route semantics — diff tools only see text. |
| 15.10 | **MCP-native services (AI agent endpoints)** | Medium | The `tool` keyword compiles to both an HTTP endpoint AND an MCP stdio server. A single `.srv` file produces services consumable by both humans (REST) and AI agents (MCP protocol) — dual-interface from one declaration. | MCP is built into the language. Other frameworks bolt it on as middleware. |
| 15.11 | **Compile-time resource estimation** | Medium | After compilation, emit a resource profile: "This service requires ~50MB RAM at idle, handles ~10K req/s on 2 cores, needs 1 DB connection pool (max 20)". Derived from static analysis of routes, spawn counts, and infra declarations. Useful for k8s resource requests. | The compiler has complete visibility into what the service does. Runtime profiling is the only alternative elsewhere. |
| 15.12 | **Language-level feature flags** | Small | `@feature("new-checkout") route "POST" "/checkout/v2" (req) { ... }` — the compiler emits both the new and old handler, with a runtime feature-flag check. `serv deploy --enable new-checkout` activates it. No external feature flag service needed. | Feature boundaries visible at compile time. Other systems need runtime-only flag evaluation. |

> See [UNIFIED_ROADMAP.md](../UNIFIED_ROADMAP.md) for the full ecosystem priority matrix and architectural recommendations.
