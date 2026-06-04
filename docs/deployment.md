# Deployment Guide

## Building for Production

```bash
serv build app.srv -o myservice.exe
```

The output is a single static binary — no runtime dependencies.

## Docker

Generate a Dockerfile:

```bash
serv dockerize app.srv
```

Or manually:

```dockerfile
FROM golang:1.23-alpine AS builder
WORKDIR /app
COPY . .
RUN go build -o serv main.go
RUN ./serv build app.srv -o service

FROM alpine:latest
COPY --from=builder /app/service /service
EXPOSE 8080
CMD ["/service"]
```

## Port Configuration

Priority (highest to lowest):

1. `--port` CLI flag: `./service --port 9090`
2. `PORT` env var: `PORT=9090 ./service`
3. Config file: `server.port: "9090"` in `config.yml`
4. Source declaration: `server "8080"`

## TLS / HTTPS

```serv
server "443" tls "cert.pem" "key.pem"
```

## Configuration File

Create `config.yml` in the working directory:

```yaml
server:
  port: "8080"

db:
  host: "localhost"
  port: "5432"
  name: "myapp"

log:
  level: "info"
  format: "json"

otel:
  endpoint: "http://collector:4318"
  service: "my-service"
```

Access in code: `config("db.host")` → `"localhost"`

## Config Validation

Fail fast on missing required config:

```serv
validate {
    required "db.host",
    required "db.port",
    required "app.secret"
}
```

If any key is missing at startup, the service exits with an error message showing which keys are missing and how to set them.

## OpenTelemetry (Tracing & Metrics)

Set environment variables to enable:

```bash
OTEL_ENDPOINT=http://localhost:4318 ./service
OTEL_SERVICE_NAME=my-service ./service
```

**Auto-instrumented:**
- HTTP routes (method, path, status, duration)
- Database queries (operation, statement)
- Cache operations (GET/SET, key)
- HTTP client calls (method, URL, status)
- Pub/sub messaging (publish/subscribe, topic)
- Scheduled jobs (every/cron, interval)
- External calls (Python/Go extern functions)

**Protocol:** OTLP/HTTP JSON — compatible with Jaeger, Tempo, Datadog, Honeycomb, etc.

## Health Checks

Auto-generated endpoints (no code needed):

- `GET /health` — Returns `{"status": "healthy"}`
- `GET /ready` — Returns `{"status": "ready"}`
- `GET /metrics` — Prometheus-style metrics

## Structured Logging

```bash
# JSON output (for log aggregators)
LOG_FORMAT=json ./service

# Set level
LOG_LEVEL=debug ./service
```

Output format (JSON mode):
```json
{"level":"info","message":"Request handled","timestamp":"2024-01-01T00:00:00Z","request_id":"abc123"}
```

## Graceful Shutdown

Serv services handle `SIGINT` and `SIGTERM`:
1. Stop accepting new connections
2. Wait up to 15 seconds for active requests to complete
3. Close database connections
4. Exit cleanly

## Cross-Compilation

Build for different platforms:

```bash
GOOS=linux GOARCH=amd64 go build -o serv-linux main.go
./serv-linux build app.srv -o service-linux
```

## CI/CD

GitHub Actions and GitLab CI templates are included:
- `.github/workflows/ci.yml`
- `.gitlab-ci.yml`

Both compile all examples, run tests, check formatting, and build release binaries on version tags.
