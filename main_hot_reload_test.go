package main

import (
	"fmt"
	"io"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestHotReload(t *testing.T) {
	// Create a temp directory for our srv file and execution context
	tmpDir, err := os.MkdirTemp("", "serv_hot_reload_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	srvFile := filepath.Join(tmpDir, "service.srv")

	// Version 1 of the service
	v1Content := `
server ":8099"

route "GET" "/hello" (req) {
	return "hello v1"
}
`
	if err := os.WriteFile(srvFile, []byte(v1Content), 0644); err != nil {
		t.Fatalf("failed to write version 1: %v", err)
	}

	// Build the serv.exe binary to make sure it includes our latest changes
	goPath, err := resolveGoPath()
	if err != nil {
		t.Fatalf("failed to find Go compiler: %v", err)
	}

	servBin := filepath.Join(tmpDir, "serv_test_runner.exe")
	buildCmd := exec.Command(goPath, "build", "-o", servBin, ".")
	if err := buildCmd.Run(); err != nil {
		t.Fatalf("failed to compile serv compiler binary: %v", err)
	}

	// Start the service in hot watch mode
	cmd := exec.Command(servBin, "run", "--hot", srvFile)
	cmd.Env = os.Environ()
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	
	if err := cmd.Start(); err != nil {
		t.Fatalf("failed to start hot reload runner: %v", err)
	}
	defer func() {
		if cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	// Query helper
	queryHello := func(expected string) error {
		client := &http.Client{Timeout: 1 * time.Second}
		resp, err := client.Get("http://127.0.0.1:8099/hello")
		if err != nil {
			return err
		}
		defer resp.Body.Close()
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			return err
		}
		got := strings.TrimSpace(string(body))
		if got != expected {
			return fmt.Errorf("expected %q, got %q", expected, got)
		}
		return nil
	}

	// Wait for V1 to start up and respond
	v1Success := false
	var lastErr error
	for i := 0; i < 100; i++ {
		time.Sleep(200 * time.Millisecond)
		if err := queryHello("hello v1"); err == nil {
			v1Success = true
			break
		} else {
			lastErr = err
		}
	}
	if !v1Success {
		t.Fatalf("service v1 did not start or respond correctly: last error: %v", lastErr)
	}

	// Now rewrite the srv file to Version 2
	v2Content := `
server ":8099"

route "GET" "/hello" (req) {
	return "hello v2"
}
`
	if err := os.WriteFile(srvFile, []byte(v2Content), 0644); err != nil {
		t.Fatalf("failed to write version 2: %v", err)
	}

	// Wait for V2 to hot reload and respond with the new content
	v2Success := false
	for i := 0; i < 40; i++ {
		time.Sleep(200 * time.Millisecond)
		if err := queryHello("hello v2"); err == nil {
			v2Success = true
			break
		}
	}
	if !v2Success {
		t.Fatal("service v2 did not reload or respond with the updated content")
	}
}
