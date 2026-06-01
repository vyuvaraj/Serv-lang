# Tech Stack

## Language & Build System

- **Compiler written in**: Go (1.26.3+)
- **Module**: `serv` (see `go.mod`)
- **Target output**: Native binaries via Go code generation
- **Source extension**: `.srv`

## Architecture

Serv is a transpiler: `.srv` → Go source → native binary. The pipeline is:
1. Lexer tokenizes `.srv` source
2. Parser builds an AST (Pratt parser with precedence climbing)
3. Codegen emits Go source code
4. `go build` compiles the generated Go into a binary

## Key Dependencies

| Library | Purpose |
|---------|---------|
| `github.com/robfig/cron/v3` | Cron scheduling |
| `github.com/glebarez/go-sqlite` | SQLite (CGO-free) |
| `github.com/lib/pq` | PostgreSQL |
| `github.com/sijms/go-ora/v2` | Oracle DB |
| `go.mongodb.org/mongo-driver` | MongoDB |
| `github.com/redis/go-redis/v9` | Redis cache |
| `github.com/segmentio/kafka-go` | Kafka broker |
| `github.com/nats-io/nats.go` | NATS broker |
| `github.com/rabbitmq/amqp091-go` | RabbitMQ broker |
| `github.com/eclipse/paho.mqtt.golang` | MQTT broker |
| `github.com/go-stomp/stomp/v3` | STOMP broker |
| `gopkg.in/yaml.v3` | YAML config parsing |

## Common Commands

```bash
# Build the Serv compiler
go build -o serv.exe main.go

# Compile a .srv file to a native binary
serv build <file.srv> -o <output>

# Run a .srv file (compile + execute)
serv run <file.srv>

# Run with hot-reload (watches .srv and .py files)
serv run <file.srv> --watch

# Run tests defined in a .srv file
serv test <file.srv>

# Lint/validate syntax
serv lint <file.srv>

# Generate a Dockerfile
serv dockerize <file.srv>

# Cross-compile release packages (Windows + Linux)
powershell ./release-scripts/build_release.ps1
```

## Build Artifacts

- `.build/` — Temporary directory for generated Go source during compilation (auto-created, should not be committed)
- `dist/` — Release packages created by `build_release.ps1`
