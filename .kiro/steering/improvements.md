# Serv Improvements — From Real-World Validation

Issues and gaps discovered while porting the regulatory-cron-service (Java/Spring Boot) to Serv.

## Bugs to Fix

| Status | Issue | Impact | Details |
|--------|-------|--------|---------|
| ⬜ | `match` fails with member access | Can't dispatch by `task.jobClass` | Parser expects simple expressions in match cases |
| ⬜ | Typed `let` + arithmetic with function returns | `let x: int = 0; x = x + fn()` fails | Type mismatch: interface{} vs int |
| ⬜ | `cron` keyword conflicts with object access | Had to rename `cron.next()` to `schedule.next()` | Keyword takes precedence over identifier |
| ✅ | `-o` flag position sensitivity | `serv build file.srv -o out` didn't work | Fixed: manual arg parsing |
| ✅ | Variable scoping in nested blocks | Variables invisible outside if/for | Fixed: scope per block |
| ✅ | Top-level `spawn` outside function body | Generated Go code outside func | Fixed: wrap in init() |

## High Priority — Blocks Real API Development

| Status | Feature | Use Case | Proposed Syntax |
|--------|---------|----------|-----------------|
| ⬜ | Request query params | Read `?page=0&size=20` from URL | `req.query.page` or `req.query["page"]` |
| ⬜ | Request headers | Read `X-Actor`, `Authorization` | `req.headers["X-Actor"]` |
| ⬜ | HTTP response status codes | Return 201, 202, 404, 429 | `return { "status": 201, "body": data }` or `response(201, data)` |
| ⬜ | `break` statement | Exit loops early | `break` |
| ⬜ | `continue` statement | Skip loop iterations | `continue` |

## Medium Priority — Developer Experience

| Status | Feature | Use Case | Proposed Syntax |
|--------|---------|----------|-----------------|
| ⬜ | Fix `match` with expressions | Dispatch by computed value | `match expr { "val" => { } }` |
| ⬜ | Multi-line strings | JSON templates, SQL queries | Backtick strings already supported in lexer |
| ⬜ | Date/time formatting | Display timestamps | `time.format(timestamp, "2006-01-02")` |
| ⬜ | Sorted queries | Find next due job | `db.findOne("jobs", filter, sortBy)` |
| ⬜ | Request body parsing | Auto-parse JSON body | `req.json` (parsed body) vs `req.body` (raw string) |

## Low Priority — Nice to Have

| Status | Feature | Use Case |
|--------|---------|----------|
| ⬜ | Ternary expression | `let x = condition ? a : b` |
| ⬜ | String template literals (multiline) | Complex JSON without escaping |
| ⬜ | `typeof` operator | Runtime type checking |
| ⬜ | Spread operator | `{ ...defaults, ...overrides }` |
| ⬜ | Destructuring | `let { name, email } = user` |
| ⬜ | Optional chaining | `user?.address?.city` |

## Data Structures — Runtime Libraries (Not Language Primitives)

| Status | Structure | API | Use Case |
|--------|-----------|-----|----------|
| ⬜ | Set | `set.new()`, `.add()`, `.has()`, `.remove()`, `.size()` | Deduplication, membership |
| ⬜ | Sorted Map | `sortedmap.new()`, `.put(key, val)`, `.first()`, `.range()` | Scheduling, leaderboards |
| ⬜ | Ring Buffer | `ring.new(size)`, `.add()`, `.items()` | Fixed-size history |

## Implementation Order

### Sprint A: API Completeness (blocks integration)
1. Request query params (`req.query`)
2. Request headers (`req.headers`)
3. HTTP response status codes
4. `break` / `continue`

### Sprint B: Developer Ergonomics
5. Fix `match` with member access expressions
6. Sorted DB queries (`db.findOne` with sort)
7. Auto-parsed request body (`req.json`)
8. Date/time formatting

### Sprint C: Language Polish
9. Multi-line template strings
10. Ternary expressions
11. Optional chaining
12. Destructuring

## Lessons Learned

1. **Keyword conflicts are painful** — `cron`, `cache`, `match` as keywords prevent using them as object names. Consider reserving fewer keywords or allowing context-sensitive parsing.

2. **Type inference + dynamic dispatch = friction** — When `let x = 0` infers `int` but later `x = x + dynamicValue()` returns `interface{}`, Go rejects it. Either commit to fully dynamic (everything is `interface{}`) or add proper type coercion.

3. **MongoDB queries need more flexibility** — `db.findOne` without sort, skip, or projection limits what you can do. The Java version uses MongoTemplate with full query builder support.

4. **The goroutine-per-job pattern works well** — No need for a priority queue. Go handles thousands of sleeping goroutines efficiently. `schedule.sleepUntilNext(cronExpr)` is the right abstraction.

5. **Registry pattern is essential** — Dynamic dispatch by name (`registry.set` / `registry.call`) is needed for any plugin-like architecture. Good that it's now in the runtime.

6. **Channel-based worker pools are idiomatic** — Replacing Java's `ExecutorService` with channels + spawn workers is natural and requires no framework.
