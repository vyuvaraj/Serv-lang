---
inclusion: always
---

# Project Conventions

## Build & Run
```bash
serv build main.srv -o service.exe
serv run main.srv --watch
```

## Testing
```bash
serv test main.srv
serv test --cover main.srv
```

## Linting & Formatting
```bash
serv lint main.srv     # errors + warnings
serv fmt main.srv      # auto-format
serv fmt --check .     # CI check
```

## Project Structure
```
myapp/
├── main.srv           # Entry point (server, routes)
├── models/            # Struct definitions
│   └── user.srv
├── handlers/          # Route handler functions
│   └── auth.srv
├── jobs/              # Scheduled tasks
│   └── cleanup.srv
├── config.yml         # Runtime configuration
└── tests/             # Test files
    └── user_test.srv
```

## Configuration
Use `config.yml` for runtime settings, accessed via `config("key")`:
```yaml
db:
  host: "localhost"
  port: "5432"
app:
  secret: "change-me"
```

## Environment Variables
- `PORT` — override server port
- `LOG_FORMAT=json` — JSON structured logging
- `LOG_LEVEL=debug` — log verbosity
- `OTEL_ENDPOINT` — enable OpenTelemetry tracing

## Error Handling Pattern
Prefer the `?` operator for clean error propagation:
```serv
fn loadUser(id: int) -> User? {
    let row = db.query("SELECT * FROM users WHERE id = ?", id)?
    return User { name: row.name, email: row.email }
}
```

## Code Style
- Use type annotations on public function parameters
- Use `string?` for values that can be nil
- Prefer `stdlib/` imports over reimplementing utilities
- One service concern per file (routes, models, jobs separated)
