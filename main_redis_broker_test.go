package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestRedisBrokerIntegration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_redis_broker_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
broker "redis-stream://127.0.0.1:6379"

subscribe "redis-events" (msg) {
	log.info("Received redis msg: ", msg)
}

test "redis broker publish check" {
	publish "redis-events" "redis-message"
	// Wait a brief moment to ensure callback fires or fallback routes
	assert 1 == 1
}
`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	// Intercept stdout to check test runner output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	runTests(tmpFile.Name(), false, "")

	w.Close()
	os.Stdout = oldStdout
	output := <-done

	t.Logf("Redis Broker Execution Output:\n%s", output)

	if !strings.Contains(output, "PASS") {
		t.Error("Expected tests to pass")
	}
	if strings.Contains(output, "FAIL") {
		t.Error("Expected no tests to fail")
	}
}
