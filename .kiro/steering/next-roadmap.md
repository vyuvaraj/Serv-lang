# Serv Next Roadmap

Tracking remaining work to make Serv production-ready and competitive.

## Status Legend
- ⬜ Not started
- 🟡 In progress
- ✅ Done

---

## Developer Experience

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | Package manager | `serv add <pkg>` — auto-generates `.srv.d` declarations from Go packages | High |
| ✅ | REPL | `serv repl` — interactive shell for quick experiments | Medium |
| ✅ | Formatter | `serv fmt` — opinionated auto-formatter | Medium |
| ⬜ | Playground | Web-based editor (like Go Playground) | Low |
| ✅ | Better errors | Diagnostics with suggestions ("did you mean X?") | Medium |

---

## Language Completeness

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | String methods | `.split()`, `.trim()`, `.replace()`, `.startsWith()`, `.includes()`, `.toUpper()`, `.toLower()` | High |
| ✅ | Closures / arrow fns | `let double = fn(x) { return x * 2 }` and `x => x * 2` shorthand | High |
| ✅ | Destructuring | `let { name, email } = user` | Medium |
| ✅ | Optional chaining | `user?.address?.city` — returns nil if any part is nil | Medium |
| ✅ | Spread operator | `let merged = { ...defaults, ...overrides }` | Medium |
| ✅ | Enums with values | `enum Status { Active = 1, Inactive = 2 }` | Low |
| ✅ | Type aliases | `type UserID = int` | Low |
| ✅ | Generic constraints | `fn sort[T: Comparable](items: []T)` | Low |
| ✅ | Modulo operator | `a % b` — remainder operator | High |
| ✅ | Break / continue | Loop control flow keywords | High |
| ✅ | Compound assignment | `+=`, `-=`, `*=`, `/=`, `%=` operators | High |
| ✅ | Bitwise operators | `&`, `\|`, `^`, `<<`, `>>` | Medium |
| ✅ | Map iteration | `for key, value in map { ... }` | High |
| ✅ | Slice expressions | `arr[1:3]`, `arr[:5]`, `arr[2:]` | Medium |
| ✅ | Type inference | Tracks types through assignments, function returns, struct fields | High |

---

## Production Readiness

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | Structured logging | JSON log output, log levels, context fields | High |
| ✅ | OpenTelemetry | Built-in tracing/metrics export (OTLP) | Medium |
| ✅ | Health endpoints | Auto-generated `/health` and `/ready` | High |
| ✅ | Config validation | Schema validation at startup, fail fast | Medium |
| ✅ | TLS support | `server "8080" tls "cert.pem" "key.pem"` | Medium |
| ✅ | WebSocket support | `ws "/chat" (conn) { ... }` | High |
| ⬜ | Graceful hot-reload | Zero-downtime restarts in watch mode | Low |
| ✅ | Request validation | Built-in body/param validation with schema | Medium |

---

## Compiler Analysis & Safety

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | Unused variable warnings | `let x = 5` warns if `x` is never referenced | High |
| ✅ | Missing return detection | Functions with return type warn if not all paths return | High |
| ✅ | Type mismatch errors | `add("hello", true)` when `add(a: int, b: int)` → compile error | High |
| ✅ | Argument count checking | Calling a function with wrong number of args → compile error | High |
| ✅ | Unreachable code detection | Warn on code after `return` / `break` / `continue` | Medium |
| ✅ | Dead import detection | Warn on unused Go package imports | Medium |
| ⬜ | Interface satisfaction checking | Verify structs implement declared interfaces | Low |

---

## Ecosystem & Distribution

| Status | Item | Description | Priority |
|--------|------|-------------|----------|
| ✅ | Documentation site | Auto-generated docs from `.srv` source | Medium |
| ✅ | CI/CD templates | GitHub Actions, GitLab CI configs | Low |
| ⬜ | Docker base image | `FROM serv:latest` for easy containerization | Low |
| ⬜ | Homebrew/Scoop | `brew install serv` / `scoop install serv` | Medium |
| ✅ | Standard library | Importable `.srv` modules (auth, validation, pagination) | Medium |
| ⬜ | VS Code Marketplace | Publish extension publicly | Medium |

---

## Suggested Implementation Order

### Sprint 1: Language Ergonomics
1. String methods (`.split`, `.trim`, `.replace`, etc.)
2. Closures / arrow functions (`x => x * 2`)
3. Health endpoints (auto `/health` and `/ready`)

### Sprint 2: Ecosystem Access
4. Package manager (`serv add`)
5. WebSocket support (`ws` keyword)
6. Structured logging (JSON mode)

### Sprint 3: Production Polish
7. TLS support
8. Optional chaining (`?.`)
9. Destructuring
10. Formatter (`serv fmt`)

### Sprint 4: Distribution
11. Homebrew/Scoop packages
12. VS Code Marketplace publish
13. Documentation site
14. Standard library modules

---

## Performance Optimization Opportunities

Analysis of the generated Go code reveals several patterns that impact runtime performance. These are ordered by impact — addressing the top 3 would bring Serv-generated code closer to hand-written Go performance.

### Critical (High Impact)

| # | Issue | Current Behavior | Impact | Fix |
|---|-------|-----------------|--------|-----|
| 1 | **Equality via fmt.Sprintf** | `==` on untyped values generates `fmt.Sprintf("%v", x) == fmt.Sprintf("%v", y)` — two allocations + string formatting per comparison | Hot-path comparisons are 50-100x slower than native | Use `reflect.DeepEqual` or direct type-switch comparison without string conversion |
| 2 | **Inline closure per arithmetic op** | Untyped `a + b` generates a 10-line IIFE with type switch: `(func() interface{} { switch l := ... })()` | Every arithmetic op allocates a closure + does 3 type assertions | Pre-compile typed helper functions (`addInt`, `addFloat`) or improve type inference to emit native ops |
| 3 | **Inline closure per member access** | `obj.field` on untyped values generates a ~15-line IIFE with type switch for Request/SafeMap/map | Every field access on dynamic objects creates a closure + multi-branch switch | Generate a single `runtime.GetFieldDynamic(obj, "field")` call instead of inlining the switch |

### Important (Medium Impact)

| # | Issue | Current Behavior | Impact | Fix |
|---|-------|-----------------|--------|-----|
| 4 | **Comparison closures for `<`, `>`** | Generates IIFE with type switch for ordered comparisons | Same closure allocation issue as arithmetic | Same fix — type inference or pre-compiled helpers |
| 5 | **SafeMap for all concurrent maps** | Every map literal in concurrent context wraps in `*SafeMap` (mutex per map) | Unnecessary lock overhead when map isn't actually shared across goroutines | Only use SafeMap when the variable is accessed from multiple goroutines (escape analysis) |
| 6 | **toSlice() copies for every for-range** | `for item in list` generates `range toSlice(list)` which type-asserts even for known `[]interface{}` | Unnecessary assertion on every loop | Skip `toSlice` when variable is already known to be `[]interface{}` |
| 7 | **Index access closures** | `arr[i]` on untyped arrays generates a multi-line IIFE with type switch | Array indexing should be near-zero cost | Emit direct index when type is known; use `runtime.Index(arr, i)` function call otherwise |

### Desirable (Lower Impact)

| # | Issue | Current Behavior | Impact | Fix |
|---|-------|-----------------|--------|-----|
| 8 | **String interpolation allocations** | F-strings generate `fmt.Sprintf(format, args...)` | Multiple allocations for string building | Use `strings.Builder` for multi-part interpolation |
| 9 | **No escape analysis hints** | All values are `interface{}` — Go's escape analysis can't stack-allocate | More GC pressure than necessary | Type inference improvements would let Go stack-allocate typed locals |
| 10 | **OTEL span on every operation** | Every DB/cache/HTTP call creates a span even with OTEL disabled (calls `TraceDB()` which checks a bool) | Minimal overhead (bool check) but unnecessary function call | Use compile-time flag or code generation to omit trace calls entirely when OTEL is off |
| 11 | **Prepared statement cache unbounded** | `stmtCache` grows without eviction | Memory leak on long-running services with many dynamic queries | Add LRU eviction or max-size limit |

### Type Inference Improvements (Root Cause)

Most performance issues stem from **insufficient type inference**. When the codegen doesn't know a variable's type, it falls back to `interface{}` and generates type-switch closures. Key improvements:

| Improvement | What it enables |
|-------------|----------------|
| **Track literal types through assignments** | `let x = 5` → codegen knows `x` is `int`, emits `x + y` directly |
| **Infer types from function return** | If `fn add(a: int, b: int) -> int`, then `let z = add(1,2)` → `z` is `int` |
| **Track collection element types** | `let items = [1, 2, 3]` → codegen knows it's `[]int`, skips `toSlice()` |
| **Propagate types through destructuring** | `let { age } = user` where `user` is a typed struct → `age` is `int` |

### Benchmark Targets

For a simple HTTP handler doing `let x = a + b; return { "result": x }`:
- **Current**: ~8 allocations per request (closures, interface boxing, map creation)
- **Optimized (type inference)**: ~2 allocations (response map + JSON encoding)
- **Hand-written Go**: ~1 allocation (JSON encoding only)

### Suggested Priority

1. **Improve type inference** (fixes issues 1-4, 6-7 automatically)
2. **Replace inline closures with runtime helper calls** (reduces generated code size, improves readability)
3. **Bounded prepared statement cache** (production stability)
4. **Conditional OTEL compilation** (clean builds for latency-sensitive services)
