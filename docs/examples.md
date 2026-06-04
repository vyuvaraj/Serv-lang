# Examples

All examples are in the `examples/` directory. Build any example:

```bash
serv build examples/<name>.srv -o demo.exe
```

## By Category

### Getting Started
| File | Description |
|------|-------------|
| `01_scheduler.srv` | Timer-based scheduled tasks |
| `02_rest_api.srv` | Simple REST API with routes |
| `05_error_handling.srv` | Try/catch error handling |

### HTTP & API
| File | Description |
|------|-------------|
| `19_rate_limiting.srv` | Per-route rate limiting |
| `29_middleware.srv` | Middleware chains |
| `43_request_validation.srv` | Body validation with schemas |
| `34_websocket_logging.srv` | WebSocket endpoints + structured logging |

### Database & Cache
| File | Description |
|------|-------------|
| `07_advanced_features.srv` | SQLite + cache + match patterns |
| `08_multi_database.srv` | Multiple database connections |
| `24_migrations.srv` | Database migrations |
| `22_query_hooks.srv` | Before-query hooks |

### Concurrency & Messaging
| File | Description |
|------|-------------|
| `03_pubsub_concurrency.srv` | Pub/sub messaging with spawn |
| `30_async_await.srv` | Async/await patterns |
| `36_channels.srv` | Go-style channels |
| `11_concurrent_maps.srv` | Thread-safe maps |

### Language Features
| File | Description |
|------|-------------|
| `25_structs.srv` | Structs and methods |
| `28_interfaces_collections.srv` | Interfaces + collection methods |
| `32_generics.srv` | Generic functions |
| `46_generic_constraints.srv` | Constrained generics (Ordered, Numeric) |
| `33_string_methods.srv` | String manipulation |
| `38_destructuring.srv` | `let { x, y } = obj` |
| `39_optional_chaining.srv` | `user?.address?.city` |
| `40_spread_operator.srv` | `{ ...defaults, ...overrides }` |
| `41_new_features.srv` | Enums with values, type aliases |

### Integration
| File | Description |
|------|-------------|
| `04_python_binding.srv` | Python extern bindings |
| `31_go_packages.srv` | Importing Go packages |
| `44_package_usage.srv` | Using `serv add` packages (uuid) |
| `15_mcp_support.srv` | MCP tool definitions |

### Configuration & Deployment
| File | Description |
|------|-------------|
| `09_yaml_config.srv` | YAML config file loading |
| `37_structured_logging.srv` | JSON logging, context loggers |
| `42_config_validation.srv` | Required config validation |
| `45_stdlib_usage.srv` | Using the standard library |

## Walkthrough: Building a REST API

```serv
// 1. Declare infrastructure
server "8080"
database "sqlite://todos.db"

// 2. Setup schema
migration "create_todos" {
    db.query("CREATE TABLE IF NOT EXISTS todos (id INTEGER PRIMARY KEY, title TEXT, done BOOLEAN DEFAULT 0)")
}

// 3. Define routes
route "GET" "/todos" (req) {
    let todos = db.query("SELECT * FROM todos")
    return { "todos": todos }
}

route "POST" "/todos" (req) {
    let errors = validate(req.body, { "title": "required" })
    if errors != nil {
        return { "status": 400, "errors": errors }
    }
    db.query("INSERT INTO todos (title) VALUES (?)", req.body)
    return { "status": 201, "message": "Created" }
}

// 4. Background cleanup
every 1h {
    let count = db.query("DELETE FROM todos WHERE done = 1")
    log.info("Cleaned up done todos")
}
```

Build and run:
```bash
serv build todo.srv -o todo.exe
./todo.exe
```

Test:
```bash
curl http://localhost:8080/todos
curl -X POST http://localhost:8080/todos -d '{"title":"Buy milk"}'
curl http://localhost:8080/health
```
