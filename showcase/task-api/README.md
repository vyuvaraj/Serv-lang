# Task Manager API

A complete REST API built with Serv — demonstrates the language's key features in a real-world project.

## Features Demonstrated

| Feature | Usage |
|---------|-------|
| HTTP Routes | CRUD endpoints for tasks |
| SQLite Database | Persistent storage with migrations |
| In-Memory Cache | 30s cache on task list |
| Scheduled Jobs | Hourly cleanup of old completed tasks |
| Structs & Methods | `Task` struct with `isComplete()` |
| JSON Parsing | Request body parsing |
| Error Handling | Input validation, 404 responses |
| Path Parameters | `/api/tasks/:id` |
| Query Parameters | `/api/tasks?status=pending` |

## Quick Start

```bash
# From the Serv-lang directory:
serv run showcase/task-api/main.srv --watch
```

The API starts on http://localhost:3000

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| GET | `/health` | Health check (auto) |
| GET | `/api/tasks` | List all tasks |
| GET | `/api/tasks?status=pending` | Filter by status |
| GET | `/api/tasks/:id` | Get single task |
| POST | `/api/tasks` | Create task |
| PUT | `/api/tasks/:id` | Update task status |
| DELETE | `/api/tasks/:id` | Delete task |
| GET | `/api/stats` | Task statistics |

## Usage Examples

```bash
# Create a task
curl -X POST http://localhost:3000/api/tasks \
  -H "Content-Type: application/json" \
  -d '{"title": "Buy groceries"}'

# List all tasks
curl http://localhost:3000/api/tasks

# Mark as done
curl -X PUT http://localhost:3000/api/tasks/1 \
  -H "Content-Type: application/json" \
  -d '{"status": "done"}'

# Get stats
curl http://localhost:3000/api/stats

# Delete
curl -X DELETE http://localhost:3000/api/tasks/1
```

## Project Structure

```
task-api/
├── main.srv       — Routes, database, scheduler (everything in one file)
├── config.yml     — Runtime configuration
├── tasks.db       — SQLite database (auto-created on first run)
└── README.md      — This file
```

## What This Shows

Serv is designed for exactly this kind of project: a focused service with clear infrastructure concerns declared at the top (`server`, `database`, `cache`) and business logic in routes and functions. No framework, no boilerplate, no dependency injection — just declare what you need and write the logic.

The entire API is ~100 lines of code. The equivalent in Go would be 300-400 lines with router setup, middleware chains, database initialization, and graceful shutdown — all of which Serv handles automatically.
