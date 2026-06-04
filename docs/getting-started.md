# Getting Started with Serv

Serv is a programming language for building background services, APIs, schedulers, and event-driven applications. It compiles to native binaries via Go.

## Installation

### From Source (requires Go 1.22+)

```bash
git clone https://github.com/vyuvaraj/Serv-lang.git
cd Serv-lang
go build -o serv.exe main.go
```

Move `serv.exe` to a directory in your PATH.

### Verify Installation

```bash
serv --help
```

## Hello World

Create `hello.srv`:

```serv
server "8080"

route "GET" "/hello" (req) {
    return { "message": "Hello from Serv!" }
}
```

Build and run:

```bash
serv build hello.srv -o hello.exe
./hello.exe
```

Visit `http://localhost:8080/hello` — you'll see:
```json
{"message": "Hello from Serv!"}
```

## Quick Run (no build step)

```bash
serv run hello.srv
```

## Hot Reload (watch mode)

```bash
serv run hello.srv --watch
```

Changes to `.srv` files trigger automatic rebuild and restart.

## Your First Real Service

```serv
server "3000"
database "sqlite://app.db"

// Run on startup
log.info("Service starting...")
db.query("CREATE TABLE IF NOT EXISTS tasks (id INTEGER PRIMARY KEY, title TEXT, done BOOLEAN)")

// Create a task
route "POST" "/tasks" (req) {
    let errors = validate(req.body, { "title": "required" })
    if errors != nil {
        return { "error": errors }
    }
    db.query("INSERT INTO tasks (title, done) VALUES (?, ?)", req.body, false)
    return { "status": "created" }
}

// List tasks
route "GET" "/tasks" (req) {
    let tasks = db.query("SELECT * FROM tasks")
    return { "tasks": tasks }
}

// Scheduled cleanup
every 1h {
    db.query("DELETE FROM tasks WHERE done = true")
    log.info("Cleaned up completed tasks")
}
```

## Next Steps

- [Language Reference](language-reference.md) — Full syntax guide
- [Built-in Functions](builtins.md) — `log`, `db`, `cache`, `http`, `json`
- [CLI Reference](cli.md) — All commands
- [Standard Library](stdlib.md) — 46 reusable modules
- [Deployment Guide](deployment.md) — Docker, TLS, observability
