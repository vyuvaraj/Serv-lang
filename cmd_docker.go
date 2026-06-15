package main

import (
	"fmt"
	"os"
	"path/filepath"
)

func dockerizeServ(srvFile string) {
	fi, err := os.Stat(srvFile)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	var absPath string
	var targetPath string
	var buildArg string

	if fi.IsDir() {
		absPath, err = filepath.Abs(srvFile)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		targetPath = absPath
		buildArg = "."
	} else {
		absPath, err = filepath.Abs(srvFile)
		if err != nil {
			fmt.Printf("Error: %v\n", err)
			os.Exit(1)
		}
		targetPath = filepath.Dir(absPath)
		buildArg = filepath.Base(srvFile)
	}

	dockerfileContent := fmt.Sprintf(`# Stage 1: Build the Serv executable
FROM golang:1.26.3-alpine AS builder
RUN apk add --no-cache git
WORKDIR /app
COPY . .
RUN go mod download
RUN go build -o serv.exe main.go
RUN ./serv.exe build %s -o service_bin

# Stage 2: Create a minimal production container
FROM alpine:latest
RUN apk --no-cache add ca-certificates python3
WORKDIR /root/
COPY --from=builder /app/service_bin .
COPY --from=builder /app/scripts/ ./scripts/
COPY --from=builder /app/examples/ ./examples/
CMD ["./service_bin"]
`, buildArg)

	dockerfilePath := filepath.Join(targetPath, "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644); err != nil {
		fmt.Printf("Failed to write Dockerfile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Dockerfile successfully generated at: %s\n", dockerfilePath)
	fmt.Println("You can now build and run your Serv service using Docker:")
	fmt.Println("  docker build -t serv-service .")
	fmt.Println("  docker run -p 8080:8080 -e PORT=8080 serv-service")
}
