package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func createNewProject(name, template string) {
	// Create project directory
	if err := os.MkdirAll(name, 0755); err != nil {
		fmt.Printf("Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	var mainSrv string
	var configYml string
	var testSrv string

	configYml = `server:
  port: "8080"

log:
  level: "info"
  format: "text"
`

	testSrv = `test "health check returns ok" {
    // TODO: add your tests here
    assert true
}
`

	switch template {
	case "api":
		mainSrv = `server "8080"
database "sqlite://dev.db"

// Model representing a task
model Task {
    id: integer,
    title: string,
    completed: boolean
}

// Get all tasks
route "GET" "/api/tasks" (req) {
    return db.query("SELECT * FROM tasks;")
}

// Add a new task
route "POST" "/api/tasks" (req) {
    let body = json.parse(req.body)
    if body.title == nil {
        return [nil, "title is required"]
    }
    db.exec("INSERT INTO tasks (title, completed) VALUES (?, ?);", body.title, false)
    return { "status": "created", "title": body.title }
}
`
	case "worker":
		mainSrv = `server "8080"

// Background worker running every 10 seconds
every 10s {
    log.info("Background worker executing routine task...")
    // Perform processing here
}

route "GET" "/api/status" (req) {
    return { "status": "running", "worker_active": true }
}
`
	case "event-processor":
		mainSrv = `server "8080"

// Connect to ServQueue STOMP broker
broker "servqueue://localhost:61613"

// Subscribe to orders topic
subscribe "orders" (msg) {
    log.info(f"Event Processor: processing order: {msg}")
    // Handle message payload here
}

// Route to manually publish messages
route "POST" "/api/orders/publish" (req) {
    publish "orders" req.body
    return { "status": "event_published" }
}
`
	case "full-stack":
		mainSrv = `server "8080"

// UI root
route "GET" "/" (req) {
    return ` + "`" + `<!DOCTYPE html>
<html lang="en">
<head>
    <meta charset="UTF-8">
    <meta name="viewport" content="width=device-width, initial-scale=1.0">
    <title>Serv Application</title>
    <style>
        body {
            font-family: system-ui, -apple-system, sans-serif;
            background: #0f172a;
            color: #f8fafc;
            display: flex;
            flex-direction: column;
            align-items: center;
            justify-content: center;
            height: 100vh;
            margin: 0;
        }
        h1 {
            color: #6366f1;
            font-size: 2.5rem;
            margin-bottom: 0.5rem;
        }
        p {
            color: #94a3b8;
        }
    </style>
</head>
<body>
    <h1>Welcome to Serv!</h1>
    <p>Your full-stack service is running successfully.</p>
</body>
</html>` + "`" + `
}

route "GET" "/api/health" (req) {
    return { "status": "healthy" }
}
`
	default:
		fmt.Printf("Unknown template: %s\n", template)
		fmt.Println("Supported templates: api, worker, event-processor, full-stack")
		os.Exit(1)
	}

	// Write files
	if err := os.WriteFile(filepath.Join(name, "main.srv"), []byte(mainSrv), 0644); err != nil {
		fmt.Printf("Failed to write main.srv: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(name, "config.yml"), []byte(configYml), 0644); err != nil {
		fmt.Printf("Failed to write config.yml: %v\n", err)
		os.Exit(1)
	}
	if err := os.WriteFile(filepath.Join(name, "main_test.srv"), []byte(testSrv), 0644); err != nil {
		fmt.Printf("Failed to write main_test.srv: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Created project: %s/ (template: %s)\n", name, template)
	fmt.Println("")
	fmt.Println("  Files:")
	fmt.Println("    main.srv       — Your service source")
	fmt.Println("    main_test.srv  — Test assertions")
	fmt.Println("    config.yml     — Profile configuration")
	fmt.Println("")
	fmt.Println("  Get started:")
	fmt.Printf("    cd %s\n", name)
	fmt.Println("    serv run main.srv --watch")
}
