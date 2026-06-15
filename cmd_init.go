package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func initProject() {
	name := "my-service"
	if len(os.Args) >= 3 {
		name = os.Args[2]
	}

	// Create project directory
	if err := os.MkdirAll(name, 0755); err != nil {
		fmt.Printf("Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	// main.srv
	mainSrv := `server "8080"

// Path parameter: curl http://localhost:8080/api/hello/Alice
route "GET" "/api/hello/:name" (req) {
    let name = req.params.name
    return { "message": f"Hello, {name}!" }
}

// Query parameter: curl http://localhost:8080/api/greet?name=Bob
route "GET" "/api/greet" (req) {
    let name = req.params.name
    if name == nil {
        return { "message": "Hello, world!" }
    }
    return { "message": f"Hello, {name}!" }
}
`
	if err := os.WriteFile(filepath.Join(name, "main.srv"), []byte(mainSrv), 0644); err != nil {
		fmt.Printf("Failed to write main.srv: %v\n", err)
		os.Exit(1)
	}

	// config.yml
	configYml := `server:
  port: "8080"

log:
  level: "info"
  format: "text"
`
	if err := os.WriteFile(filepath.Join(name, "config.yml"), []byte(configYml), 0644); err != nil {
		fmt.Printf("Failed to write config.yml: %v\n", err)
		os.Exit(1)
	}

	// test file
	testSrv := `test "health check returns ok" {
    // TODO: add your tests here
    assert true
}
`
	if err := os.WriteFile(filepath.Join(name, "main_test.srv"), []byte(testSrv), 0644); err != nil {
		fmt.Printf("Failed to write main_test.srv: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Created project: %s/\n", name)
	fmt.Println("")
	fmt.Println("  Files:")
	fmt.Println("    main.srv       — Your service (routes, logic)")
	fmt.Println("    main_test.srv  — Tests")
	fmt.Println("    config.yml     — Runtime configuration")
	fmt.Println("")
	fmt.Println("  Get started:")
	fmt.Printf("    cd %s\n", name)
	fmt.Println("    serv run main.srv --watch")
	fmt.Println("")
	fmt.Println("  Then visit: http://localhost:8080/health")
}
