# Compiler Refactoring Guide

## Overview

Split `parser.go` (1856 lines) and `codegen.go` (2192 lines) into smaller, focused files while keeping everything in the same `compiler` Go package.

**Important:** All files stay in `compiler/` directory with `package compiler`. No sub-packages — Go allows multiple files in the same package. This means zero import changes anywhere.

---

## Target Structure

```
compiler/
├── ast.go                  (keep as-is — 854 lines, well-scoped)
├── lexer.go                (keep as-is — 466 lines, well-scoped)
├── errors.go               (keep as-is — diagnostic formatting)
│
├── parser.go               → parser_core.go
├── (new) parser_expr.go
├── (new) parser_stmt.go
├── (new) parser_services.go
├── (new) parser_functions.go
├── (new) parser_types.go
├── (new) parser_controlflow.go
│
├── codegen.go              → codegen_core.go
├── (new) codegen_expr.go
├── (new) codegen_stmt.go
├── (new) codegen_services.go
├── (new) codegen_types.go
├── (new) codegen_testing.go
├── (new) codegen_helpers.go
```

---

## Parser Split

### `parser_core.go` (keep as `parser.go`, shrink to ~220 lines)
Infrastructure: struct, constructor, token navigation, program parsing, statement dispatch.

| Function | Line | Keep here |
|----------|------|-----------|
| `NewParser` | 57 | ✅ |
| `registerPrefix` | 103 | ✅ |
| `registerInfix` | 107 | ✅ |
| `nextToken` | 111 | ✅ |
| `Errors` | 116 | ✅ |
| `addError` | 120 | ✅ |
| `ParseProgram` | 124 | ✅ |
| `parseStatement` (dispatch switch) | 139 | ✅ |
| `expectPeek` | 1258 | ✅ |
| `peekPrecedence` | 1292 | ✅ |
| `curPrecedence` | 1304 | ✅ |
| `peekError` | 1311 | ✅ |
| `isParamListFollowedByBrace` | 1267 | ✅ |
| `parseBlockStatement` | 858 | ✅ |
| `parseExpressionStatement` | 875 | ✅ |

Also keep: precedence constants, precedence map, Parser struct, type definitions.

### `parser_expr.go` (~450 lines)
All expression parsing functions.

| Function | Current Line |
|----------|-------------|
| `parseExpression` | 881 |
| `parseIdentifier` | 902 |
| `isGenericCallAhead` | 942 |
| `isStructLiteralAhead` | 975 |
| `parseStructLiteral` | 996 |
| `parseIntegerLiteral` | 1033 |
| `parseStringLiteral` | 1046 |
| `parseFloatLiteral` | 1050 |
| `parseDurationLiteral` | 1061 |
| `parseArrayLiteral` | 1065 |
| `parseMapLiteral` | 1084 |
| `parseGroupedExpression` | 1134 |
| `parseInfixExpression` | 1144 |
| `parseCallExpression` | 1158 |
| `parseExpressionList` | 1164 |
| `parseMemberExpression` | 1218 |
| `parseOptionalMemberExpression` | 1229 |
| `parseIndexExpression` | 1315 |
| `parseAssignmentExpression` | 1240 |
| `parseFStringLiteral` | 850 |
| `parseCacheIdentifier` | 854 |
| `parseAssertExpression` | 1210 |
| `parseBooleanLiteral` | 1476 |
| `parseNilLiteral` | 1480 |
| `parseSelfExpression` | 1500 |
| `parseValidateIdentifier` | 1376 |

**Imports needed:** `"fmt"`, `"strconv"`

### `parser_stmt.go` (~300 lines)
General statement parsing.

| Function | Current Line |
|----------|-------------|
| `parseImportStatement` | 214 |
| `parseExternStatement` | 264 |
| `parseLetStatement` | 539 |
| `parseReturnStatement` | 601 |
| `parseTestStatement` | 1189 |
| `parseExportStatement` | 1768 |
| `parseDatabaseStatement` | 793 |
| `parseCacheStatement` | 800 |
| `parseBrokerStatement` | 308 |
| `parsePublishStatement` | 513 |
| `parseSpawnStatement` | 524 |

**Imports needed:** (none beyond what parser_core provides)

### `parser_services.go` (~250 lines)
Service-infrastructure parsing: routes, middleware, websockets, schedulers.

| Function | Current Line |
|----------|-------------|
| `parseServerStatement` | 315 |
| `parseRouteStatement` | 337 |
| `parseToolStatement` | 413 |
| `parseMigrationStatement` | 447 |
| `parseEveryStatement` | 463 |
| `parseCronStatement` | 475 |
| `parseSubscribeStatement` | 487 |
| `parseMiddlewareDeclaration` | 1737 |
| `parseWsStatement` | 1620 |
| `parseDeclareStatement` | 1648 |

**Imports needed:** (none)

### `parser_functions.go` (~200 lines)
Function/method/closure parsing.

| Function | Current Line |
|----------|-------------|
| `parseFnDeclaration` | 608 |
| `parseMethodDeclaration` | 708 |
| `parseFnLiteral` | 1512 |
| `parseArrowFunction` | 1558 |

**Imports needed:** (none)

### `parser_types.go` (~180 lines)
Type system parsing: structs, interfaces, enums, type aliases, annotations.

| Function | Current Line |
|----------|-------------|
| `parseStructDeclaration` | 1577 |
| `parseInterfaceDeclaration` | 1781 |
| `parseEnumStatement` | 1325 |
| `parseTypeAliasStatement` | 1359 |
| `parseValidateStatement` | 1381 |
| `parseTypeAnnotation` | 1488 |

**Imports needed:** (none)

### `parser_controlflow.go` (~180 lines)
Control flow parsing.

| Function | Current Line |
|----------|-------------|
| `parseIfStatement` | 1413 |
| `parseForStatement` | 1450 |
| `parseMatchStatement` | 807 |
| `parseTryCatchStatement` | 760 |
| `parseAwaitExpression` | 1504 |

**Imports needed:** (none)

---

## Codegen Split

### `codegen_core.go` (keep as `codegen.go`, shrink to ~250 lines)
Struct, constructor, Generate(), genStatement dispatch, genBlockStatement.

| Function | Current Line | Keep here |
|----------|-------------|-----------|
| `NewCodegen` | 27 | ✅ |
| `Generate` | 43 | ✅ |
| `genStatement` (dispatch only) | 161 | ✅ |
| `genBlockStatement` | 871 | ✅ |
| `GenerateMainFunc` | 1668 | ✅ |

Also keep: Codegen struct definition, all field declarations.

### `codegen_expr.go` (~700 lines)
The `genExpression` function and its helpers.

| Function | Current Line |
|----------|-------------|
| `genExpression` (entire switch) | 903–1660 |
| `genCollectionCallback` | 1822 |
| `genReduceCallback` | 1860 |
| `genFString` | 1871 |
| `compileInlineExpr` | 1925 |

**Imports needed:** `"fmt"`, `"strings"`, `"path/filepath"`

### `codegen_stmt.go` (~600 lines)
Statement generation (extract from `genStatement` body).

Extract these `case` branches from `genStatement`:
- `*ImportStmt`, `*GoPackageImport`, `*DeclareModuleStmt`, `*ExportStmt`
- `*ExternFnStmt`, `*EnumStmt`, `*TypeAliasStmt`, `*ValidateStmt`
- `*IfStmt`, `*ForStmt`, `*MatchStmt`, `*TryCatchStmt`
- `*LetStmt`, `*DestructureLetStmt`, `*ReturnStmt`, `*ExprStmt`
- `*FnDecl`, `*MethodDecl`, `*TestStmt`
- `*SpawnStmt`, `*PublishStmt`

**Approach:** Create helper methods like `genLetStmt(s *LetStmt)`, `genFnDecl(s *FnDecl)`, etc. The dispatch in `genStatement` calls them. This way `codegen_core.go` has the dispatch and `codegen_stmt.go` has the implementations.

### `codegen_services.go` (~200 lines)
Service infrastructure generation.

Extract from `genStatement`:
- `*BrokerStmt`, `*ServerStmt`, `*DatabaseStmt`, `*CacheStmt`
- `*RouteStmt`, `*MiddlewareDecl`, `*WsStmt`
- `*EveryStmt`, `*CronStmt`, `*SubscribeStmt`
- `*ToolStmt`, `*MigrationStmt`

### `codegen_types.go` (~200 lines)
Type system utilities.

| Function | Current Line |
|----------|-------------|
| `toGoType` | 1936 |
| `servConstraintToGo` | 1961 |
| `zeroValue` | 1985 |
| `getExpressionType` | 2000 |
| `capitalizeFirst` | 1792 |
| `sanitizeTestName` | 1801 |

Also extract from `genStatement`:
- `*StructDecl`, `*InterfaceDecl`

### `codegen_testing.go` (~80 lines)
Test generation.

| Function | Current Line |
|----------|-------------|
| `HasTests` | 1750 |
| `GenerateTests` | 1757 |

### `codegen_helpers.go` (~100 lines)
Helper function generation and utilities.

| Function | Current Line |
|----------|-------------|
| `GenerateHelpers` | 1676 |
| `hasConcurrency` | 2075 |
| `stmtToken` | 2118 |

---

## Execution Steps

### Phase 1: Parser Split (safest first)

```bash
# 1. Create new files with correct package header
# 2. Move functions (cut from parser.go, paste into new file)
# 3. Add required imports to each new file
# 4. Build to verify: go build ./compiler
# 5. Run regression: powershell -ExecutionPolicy Bypass -File test_regression.ps1 -Phase 1
```

**Order of operations:**
1. Create `parser_expr.go` — move all expression functions
2. Build & test
3. Create `parser_controlflow.go` — move if/for/match/try
4. Build & test
5. Create `parser_types.go` — move struct/interface/enum
6. Build & test
7. Create `parser_services.go` — move route/ws/middleware/scheduler
8. Build & test
9. Create `parser_functions.go` — move fn/method/arrow
10. Build & test

### Phase 2: Codegen Split

```bash
# Same approach — one file at a time, build after each
```

**Order of operations:**
1. Create `codegen_types.go` — move type helpers (smallest, safest)
2. Build & test
3. Create `codegen_helpers.go` — move GenerateHelpers, hasConcurrency, stmtToken
4. Build & test
5. Create `codegen_testing.go` — move HasTests, GenerateTests
6. Build & test
7. Create `codegen_expr.go` — move genExpression (largest piece)
8. Build & test
9. Create `codegen_services.go` — extract service cases from genStatement
10. Build & test

---

## Important Notes

1. **Same package, same directory** — all files must be `package compiler` and live in `compiler/`. Go compiles all `.go` files in a directory as one package.

2. **No circular dependencies** — since it's one package, all types and functions are accessible across files.

3. **Import deduplication** — each file only needs imports for what it directly uses. The `fmt`, `strings`, `strconv` imports move to wherever they're actually called.

4. **Don't rename functions** — just move them. No external API changes.

5. **Build after every move** — `go build ./compiler` catches any issues immediately.

6. **One function group at a time** — never move more than one category before verifying the build.

---

## Future: Semantic Analysis Layer

After the split is complete, add a new phase between parsing and codegen:

```
compiler/
├── ...existing files...
├── analyze.go            — Semantic analysis (new)
│   • Variable scope validation
│   • Undefined variable detection  
│   • Type inference propagation
│   • "Did you mean?" for identifiers
│   • Dead code detection
└── typecheck.go          — Type checking (new)
    • Assignment compatibility
    • Function argument type checking
    • Return type verification
    • Generic constraint validation
```

The pipeline becomes:
```
Lexer → Parser → AST → Analyze → TypeCheck → Codegen → Go Source
```

This is a separate effort from the file split and should be its own branch.


---

## Runtime Split

### Overview

Split `runtime/runtime.go` (3512 lines, 153 functions) into focused files. Same principle as the compiler split — all files stay `package runtime` in the `runtime/` directory.

### Target Structure

```
runtime/
├── core.go            — Server startup, config loading, env, init, CLI flags (~350 lines)
├── http.go            — HTTP server, routing, trie, request handling, middleware (~500 lines)
├── db.go              — Database init, queries, MongoDB, prepared stmt cache (~600 lines)
├── cache.go           — Redis/in-memory cache operations (~100 lines)
├── broker.go          — Pub/sub (NATS, Kafka, RabbitMQ, MQTT, STOMP, in-memory) (~400 lines)
├── scheduler.go       — Every, Cron, Sleep (~100 lines)
├── logging.go         — All logging functions, ContextLogger (~200 lines)
├── collections.go     — SafeMap, Filter, Map, Find, Reduce, Push, Contains, Length (~250 lines)
├── strings.go         — String methods (Split, Trim, Replace, StartsWith, etc.) (~100 lines)
├── concurrency.go     — Spawn, Await, AwaitAll, channels, atomics, semaphores (~350 lines)
├── python.go          — Python daemon pool, CallPython, worker management (~200 lines)
├── helpers.go         — HTTP client, JSON parse/stringify, metrics, registry (~250 lines)
├── mcp.go             — MCP tool server, JSON-RPC handling (~200 lines)
├── perf.go            — Equal, Compare, Arith, MemberAccess, GetField, MergeMaps (~200 lines)
├── validation.go      — ValidateConfig, ValidateBody (~100 lines)
├── otel.go            — (already exists — OpenTelemetry tracing)
```

---

### Function-to-File Mapping

#### `core.go` — Server lifecycle & config
| Function | Line |
|----------|------|
| `init` (config/log) | 371, 509 |
| `loadYAMLConfig` | 396 |
| `flattenMap` | 445 |
| `Env` | 486 |
| `Config` | 490 |
| `Noop` | 131 |
| `getCliFlag` | 135 |
| `InitServer` | 1066 |
| `InitServerTLS` | 1076 |
| `StartServer` | 1521 |
| `handleMetrics` | 1714 |
| `handleHealth` | 1730 |
| `handleReady` | 1740 |

Also keep: all `var` blocks for config, serverPort, etc., and the `Request`/`HTTPResponse` structs.

#### `http.go` — Routing & request handling
| Function | Line |
|----------|------|
| `newRouteRateLimiter` | 1091 |
| `(*routeRateLimiter).allow` | 1111 |
| `AddRoute` | 1132 |
| `AddRouteWithMiddleware` | 1200 |
| `RegisterMiddleware` | 1156 |
| `insertRoute` (trie) | 3449 |
| `matchRoute` (trie) | 3486 |
| `newTrieNode` | 3445 |

Also move: `trieNode` struct, `routeRateLimiter` struct, middleware registry vars.

#### `db.go` — Database layer
| Function | Line |
|----------|------|
| `InitDB` | 2590 |
| `configureDBPool` | 2563 |
| `getCachedStmt` | 2643 |
| `AddBeforeQueryHook` | 2688 |
| `DBQuery` | 2694 |
| `DBQueryPage` | 2126 |
| `DBFindOne` | 2194 |
| `DBCount` | 2229 |
| `DBUpsert` | 2259 |
| `DBQuerySafe` | 2885 |
| `runMongoQuery` | 2778 |
| `RegisterMigration` | 1472 |
| `RunMigrations` | 1478 |

Also move: `dbInstance`, `mongoDB`, `stmtCache`, `beforeQueryHooks` vars, MongoDB connection logic.

#### `cache.go` — Caching
| Function | Line |
|----------|------|
| `InitCache` | 2942 |
| `CacheSet` | 2955 |
| `CacheGet` | 2980 |

Also move: `redisClient`, `localCache`, `localCacheEntry` vars/structs.

#### `broker.go` — Pub/sub messaging
| Function | Line |
|----------|------|
| `InitBroker` | 855 |
| `Subscribe` | 906 |
| `Publish` | 984 |
| `startPubSubWorkers` | 3415 |
| `executeCallbackSafe` | 3427 |

Also move: all broker connection vars (natsConn, kafkaWriter, amqpChannel, etc.), subscriber maps.

#### `scheduler.go` — Timers & cron
| Function | Line |
|----------|------|
| `Every` | 792 |
| `Cron` | 826 |
| `Sleep` | 1444 |
| `CronNext` | 2400 |
| `CronSleepUntilNext` | 2426 |
| `SpawnWithTimeout` | 2460 |

Also move: `cronInstance`, `cronOnce` vars.

#### `logging.go` — Structured logging
| Function | Line |
|----------|------|
| `shouldLog` | 521 |
| `logStructured` | 528 |
| `logStructuredWithFields` | 532 |
| `LogInfo` | 561 |
| `LogWarn` | 565 |
| `LogError` | 569 |
| `LogDebug` | 573 |
| `(*ContextLogger).Info` | 585 |
| `(*ContextLogger).Warn` | 591 |
| `(*ContextLogger).Error` | 597 |
| `(*ContextLogger).Debug` | 603 |
| `(*ContextLogger).With` | 609 |
| `LogWith` | 625 |
| `LogFields` | 643 |
| `LogSetLevel` | 660 |
| `LogGetLevel` | 676 |
| `ContextLoggerInfo` | 684 |
| `ContextLoggerWarn` | 694 |
| `ContextLoggerError` | 703 |
| `ContextLoggerDebug` | 712 |
| `ContextLoggerWith` | 721 |

Also move: `logJSON`, `logLevel`, `logLevelMu` vars, `ContextLogger` struct.

#### `collections.go` — Data structures & collection methods
| Function | Line |
|----------|------|
| `Filter` | 154 |
| `Map` | 167 |
| `Find` | 177 |
| `Reduce` | 189 |
| `ForEach` | 199 |
| `Length` | 208 |
| `Push` | 226 |
| `Contains` | 232 |
| `toInterfaceSlice` | 243 |
| `isTruthyVal` | 253 |
| `NewSafeMap` | 3020 |
| `NewSafeMapFromMap` | 3024 |
| `(*SafeMap).Set` | 3031 |
| `(*SafeMap).Get` | 3037 |
| `(*SafeMap).Delete` | 3043 |
| `(*SafeMap).MarshalJSON` | 3049 |
| `(*SafeMap).All` | 3056 |
| `ToSafeValue` | 3391 |

Also move: `SafeMap` struct, `localCacheEntry` if not in cache.go.

#### `strings.go` — String method implementations
| Function | Line |
|----------|------|
| `StringSplit` | 275 |
| `StringTrim` | 286 |
| `StringReplace` | 290 |
| `StringStartsWith` | 294 |
| `StringEndsWith` | 298 |
| `StringIncludes` | 302 |
| `StringToUpper` | 306 |
| `StringToLower` | 310 |
| `StringSubstring` | 314 |
| `StringIndexOf` | 336 |
| `StringRepeat` | 340 |
| `toInt` | 344 |

#### `concurrency.go` — Goroutines, channels, atomics
| Function | Line |
|----------|------|
| `Await` | 1164 |
| `AwaitAll` | 1178 |
| `AcquireSemaphore` | 1998 |
| `ReleaseSemaphore` | 2010 |
| `getOrCreateAtomic` | 2031 |
| `AtomicNew` | 2043 |
| `AtomicInc` | 2053 |
| `AtomicDec` | 2072 |
| `AtomicGet` | 2091 |
| `AtomicSet` | 2100 |
| `AtomicCAS` | 2110 |
| `ChannelNew` | 2483 |
| `ChannelSend` | 2493 |
| `ChannelReceive` | 2502 |
| `ChannelTryReceive` | 2515 |
| `ChannelTrySend` | 2532 |
| `ChannelClose` | 2546 |
| `ChannelLen` | 2555 |

Also move: `AtomicValue` struct, `atomicStore`, semaphore maps, channel registry.

#### `python.go` — Python interop
| Function | Line |
|----------|------|
| `initPythonDaemonPool` | 1776 |
| `CallPython` | 1798 |
| `isProcessAlive` | 1884 |
| `spawnPythonWorker` | 1897 |
| `shutdownPythonWorkers` | 1957 |

Also move: `pythonWorker` struct, `pythonPoolQueue`, pool vars.

#### `helpers.go` — HTTP client, JSON, metrics, registry
| Function | Line |
|----------|------|
| `MetricInc` | 730 |
| `MetricGauge` | 736 |
| `HTTPGet` | 743 |
| `HTTPPost` | 763 |
| `HTTPGetSafe` | 2899 |
| `HTTPPostSafe` | 2913 |
| `JSONParse` | 1979 |
| `JSONStringify` | 1989 |
| `JSONParseSafe` | 2927 |
| `RegistrySet` | 2319 |
| `RegistryCall` | 2330 |
| `RegistryList` | 2377 |
| `RegistryHas` | 2389 |

Also move: metrics vars (`metricsCounters`, `metricsGauges`), registry map.

#### `mcp.go` — MCP tool server
| Function | Line |
|----------|------|
| `AddMCPTool` | 1282 |
| `InvokeMCPToolForTesting` | 1292 |
| `startMCPServer` | 1316 |
| `sendRPCError` | 1332 |
| `handleMCPRequest` | 1345 |

Also move: `mcpTools` map, `jsonRPCRequest`/`jsonRPCResponse` structs.

#### `perf.go` — Performance helpers
| Function | Line |
|----------|------|
| `Equal` | 3107 |
| `Compare` | 3135 |
| `Arith` | 3178 |
| `MemberAccess` | 3233 |
| `GetField` | 3069 |
| `MergeMaps` | 3263 |

#### `validation.go` — Config & request validation
| Function | Line |
|----------|------|
| `ValidateConfig` | 3283 |
| `ValidateBody` | 3306 |

#### `websocket.go` — WebSocket support
| Function | Line |
|----------|------|
| `(*WSConn).Send` | 1232 |
| `(*WSConn).Receive` | 1244 |
| `(*WSConn).Close` | 1252 |
| `AddWebSocket` | 1264 |

Also move: `WSConn` struct, `wsHandlers` map, `upgrader` var.

---

### Execution Order (by independence)

1. **strings.go** — zero dependencies, pure functions
2. **perf.go** — only depends on `GetField` (uses reflect)
3. **validation.go** — uses `Config`, `json`, `SafeMap`
4. **logging.go** — self-contained with own vars
5. **collections.go** — SafeMap + collection methods
6. **concurrency.go** — atomics, channels, semaphores
7. **scheduler.go** — Every, Cron, uses logging
8. **cache.go** — Redis/in-memory
9. **python.go** — daemon pool (self-contained)
10. **mcp.go** — tool server
11. **helpers.go** — metrics, HTTP client, JSON, registry
12. **broker.go** — pub/sub (uses logging, metrics)
13. **websocket.go** — uses gorilla/websocket
14. **db.go** — database (largest, uses mongo/sql drivers)
15. **http.go** — routing (uses everything above)
16. **core.go** — startup (orchestrates all)

Build after each: `go build ./runtime`
