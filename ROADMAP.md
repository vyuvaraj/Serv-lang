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
| 9.3 | **Web playground** | ⬜ Pending | Browser-based interactive editor |
| 9.4 | Docker base image | ✅ Done | `Dockerfile.base` configured |
| 9.5 | **Community package registry** | ⬜ Pending | Online index for package sharing |

---

## Remaining Work Items

### 1. Web Playground (Phase 9.3)
- **Goal**: Allow users to write and run Serv programs in a web browser without local installation.
- **Approach**: Compile the Serv compiler to WebAssembly (WASM) or build a backend runner service that runs temporary containers to execute compilation and return output.
- **Effort**: Medium

### 2. Community Package Registry (Phase 9.5)
- **Goal**: Establish a central registry where authors can publish Serv modules and consumers can install them using a CLI command (e.g., `serv install <pkg>`).
- **Approach**: Build a simple registry server index hosting metadata and tarball archives, integrated with local dependency retrieval in the Serv CLI tool.
- **Effort**: Large

---

## Phase 10: Taking Serv to the Next Level (Future Priorities)

To move Serv beyond a simple microservice tool into a premium, world-class programming language for high-performance distributed systems, we propose the following items for the next phase of evolution:

| # | Item | Effort | Description |
|---|------|--------|-------------|
| 10.1 | **Generics (Parametric Polymorphism)** | Large | Introduce type-safe generics (e.g., `fn map[T, U](arr: []T)`) to avoid `interface{}` casting on collections. |
| 10.2 | **First-Class Actor Model** | Large | Formalize `spawn` with local mailboxes, message routing, and supervisor trees for robust concurrency. |
| 10.3 | **Database Schema ORM Generation** | Medium | Compile schema files into strongly-typed query methods, ensuring compile-time safe database interaction. |
| 10.4 | **Distributed Trace Propagation** | Medium | Automatically trace HTTP headers, message brokers, and actor spawns with OpenTelemetry context propagation. |
| 10.5 | **AOT Optimization Pass** | Medium | Build AST optimizations (inlining, constant folding, loop unrolling) before emitting target Go source. |

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
