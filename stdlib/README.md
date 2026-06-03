# Serv Standard Library

Reusable `.srv` modules for common service patterns. Import what you need:

```serv
import { ok, notFound } from "stdlib/response.srv"
import { requireAuth, bearerToken } from "stdlib/auth.srv"
```

---

## Module Index

| Module | Exports | Category |
|--------|---------|----------|
| `auth.srv` | `bearerToken`, `basicAuth`, `requireAuth` | Security |
| `crypto.srv` | `hashPassword`, `verifyPassword`, `randomToken`, `randomHex`, `hmacSign`, `hmacVerify` | Security |
| `jwt.srv` | `jwtEncode`, `jwtDecode`, `jwtIsExpired` | Security |
| `sanitize.srv` | `escapeHTML`, `stripTags`, `escapeSQL`, `sanitizeFilename`, `normalizeWhitespace` | Security |
| `ratelimit.srv` | `createLimiter`, `isAllowed`, `remaining`, `resetLimiter` | Security |
| `validation.srv` | `required`, `isEmail`, `isURL`, `minLength`, `maxLength`, `validateFields` | Input |
| `response.srv` | `ok`, `created`, `badRequest`, `notFound`, `serverError`, `errorResponse` | HTTP |
| `pagination.srv` | `offset`, `pageResponse`, `parsePageParams` | HTTP |
| `middleware.srv` | `corsHeaders`, `requestId`, `logRequest`, `isPreflight` | HTTP |
| `http_client.srv` | `getJSON`, `postJSON`, `isSuccess`, `isClientError`, `isServerError` | HTTP |
| `url.srv` | `encodeURI`, `parseQuery`, `buildQuery`, `joinPath`, `extractPath` | HTTP |
| `datetime.srv` | `now`, `timestamp`, `isExpired`, `formatDuration`, `sleep` | Utilities |
| `strings_util.srv` | `slugify`, `truncate`, `capitalize`, `isEmpty`, `repeat`, `matches` | Utilities |
| `math.srv` | `min`, `max`, `clamp`, `abs`, `percent`, `between`, `sum`, `average` | Utilities |
| `sort.srv` | `sortAsc`, `sortDesc`, `reverse`, `minOf`, `maxOf` | Utilities |
| `collections.srv` | `groupBy`, `unique`, `flatten`, `chunk`, `first`, `last`, `countWhere` | Data |
| `csv.srv` | `parseCSV`, `parseRow`, `toRow`, `toCSV` | Data |
| `diff.srv` | `hasChanged`, `fieldChanged`, `changeRecord` | Data |
| `env.srv` | `requireEnv`, `envOrDefault`, `envInt`, `envBool`, `envExists` | Config |
| `retry.srv` | `backoffDelay`, `defaultMaxRetries`, `defaultBaseDelay` | Resilience |
| `circuit_breaker.srv` | `createBreaker`, `isOpen`, `recordSuccess`, `recordFailure`, `resetBreaker`, `failureCount` | Resilience |
| `queue.srv` | `createQueue`, `enqueue`, `dequeue`, `queueSize`, `queueIsEmpty` | Resilience |
| `events.srv` | `on`, `emit`, `hasHandler` | Messaging |
| `metrics.srv` | `counter`, `counterWithLabel`, `gauge`, `recordLatency`, `trackRequest` | Observability |
| `testing_helpers.srv` | `assertEqual`, `assertNotNil`, `assertNil`, `assertContains`, `assertTrue`, `assertFalse`, `assertLength` | Testing |
| `health.srv` | `healthy`, `unhealthy`, `degraded`, `buildHealthResponse` | Ops |
| `scheduler.srv` | `scheduleAfter`, `isScheduled`, `cancelSchedule`, `getDelay` | Scheduling |
| `webhook.srv` | `buildPayload`, `sendWebhook`, `verifySignature`, `retryRecord` | Integration |
| `cors.srv` | `allowOrigin`, `allowAll`, `preflightResponse`, `isOriginAllowed` | HTTP |
| `graceful.srv` | `initShutdown`, `isShuttingDown`, `connectionOpened`, `connectionClosed`, `isDrained` | Ops |
| `tracing.srv` | `traceId`, `spanId`, `startSpan`, `endSpan`, `addTag`, `traceContext` | Observability |
| `semaphore.srv` | `createSemaphore`, `tryAcquire`, `release`, `available`, `utilization` | Concurrency |
| `batch.srv` | `createBatch`, `addToBatch`, `batchSize`, `isBatchFull`, `flushBatch` | Processing |
| `idempotency.srv` | `checkIdempotency`, `markProcessed`, `isProcessed`, `getProcessedResult` | Reliability |
| `job.srv` | `createJob`, `startJob`, `completeJob`, `failJob`, `jobStatus` | Processing |
| `feature_flags.srv` | `enableFlag`, `disableFlag`, `isEnabled`, `toggleFlag`, `initFlag` | Config |
| `config.srv` | `getConfig`, `requireConfig`, `configInt`, `configBool`, `configList`, `hasConfig` | Config |
| `tenant.srv` | `extractTenant`, `tenantConfig`, `isTenantActive`, `tenantCacheKey`, `tenantFilter` | Multi-tenancy |
| `dlq.srv` | `createDLQ`, `sendToDLQ`, `dlqSize`, `dlqHasItems`, `clearDLQ` | Reliability |
| `audit.srv` | `auditLog`, `auditAction`, `auditAccess`, `auditAuth`, `auditDenied` | Compliance |

---

## Categories

### Security
- **auth.srv** — Token extraction, bearer/basic auth, auth guards
- **crypto.srv** — Password hashing, HMAC signing, token generation
- **jwt.srv** — JWT encode/decode/expiry (lightweight; use `serv add github.com/golang-jwt/jwt/v5` for production)

### HTTP
- **response.srv** — Standard HTTP response builders (ok, notFound, etc.)
- **pagination.srv** — Page offset calculation, response envelope
- **middleware.srv** — CORS headers, request ID generation, preflight detection
- **http_client.srv** — JSON GET/POST wrappers, status code helpers

### Utilities
- **datetime.srv** — Timestamps, expiry checks, duration formatting
- **strings_util.srv** — Slugify, truncate, capitalize, pattern matching
- **collections.srv** — Array utilities (unique, flatten, chunk, first/last)

### Config & Environment
- **env.srv** — Required env vars, defaults, type coercion (int/bool)

### Resilience
- **retry.srv** — Exponential backoff calculation

### Messaging
- **events.srv** — In-process event bus (emit/on pattern)

### Testing
- **testing_helpers.srv** — Expressive assertions for test blocks

### Operations
- **health.srv** — Custom health check builders
- **graceful.srv** — Shutdown state, connection draining, drain detection

### Scheduling
- **scheduler.srv** — Dynamic runtime scheduling beyond `every`/`cron`

### Integration
- **webhook.srv** — Outgoing webhook payloads, signature verification, retry records
- **cors.srv** — CORS header generation, origin checking, preflight responses

### Concurrency
- **semaphore.srv** — Named semaphores with slot tracking and utilization metrics

### Processing
- **batch.srv** — Accumulate-and-flush batch pattern with size tracking
- **job.srv** — Background job lifecycle (pending → running → completed/failed)

### Reliability
- **idempotency.srv** — Idempotency key pattern for deduplication
- **dlq.srv** — Dead letter queue for failed message tracking

### Multi-tenancy
- **tenant.srv** — Tenant extraction from requests, scoped config/cache/DB keys

### Compliance
- **audit.srv** — Structured audit trail (actions, access, auth, denied events)

---

## Usage Example

```serv
import { requireAuth, bearerToken } from "stdlib/auth.srv"
import { ok, badRequest } from "stdlib/response.srv"
import { required, isEmail } from "stdlib/validation.srv"
import { envOrDefault } from "stdlib/env.srv"

server envOrDefault("PORT", "8080")

route "POST" "/api/users" (req) {
    let authErr = requireAuth(req)
    if authErr != nil {
        return authErr
    }

    let errors = validate(req.body, {
        "email": "required,email",
        "name": "required"
    })
    if errors != nil {
        return badRequest(errors)
    }

    return ok({ "created": true })
}
```

---

## Contributing

Add new modules as `stdlib/<name>.srv`. Export functions with `export fn`. Follow existing patterns:
- Pure functions where possible
- No side effects unless explicitly documented
- Use `interface{}` params (no type annotations) for maximum flexibility
