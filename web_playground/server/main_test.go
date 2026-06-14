package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandleRun(t *testing.T) {
	// Prepare test server
	// Set the servExePath relative to the test location if needed, but since it's testing
	// fallback to go run should work if serv.exe is not found.
	// For testing, let's ensure we point to the repository root.
	servExePath = "" // Force fallback to 'go run main.go' in tests or resolve it:
	
	reqBody, err := json.Marshal(RunRequest{
		Source: `log.info("Test Run Success!")`,
	})
	if err != nil {
		t.Fatalf("Failed to marshal request: %v", err)
	}

	req, err := http.NewRequest("POST", "/api/run", bytes.NewBuffer(reqBody))
	if err != nil {
		t.Fatalf("Failed to create request: %v", err)
	}

	rr := httptest.NewRecorder()
	handler := http.HandlerFunc(handleRun)

	handler.ServeHTTP(rr, req)

	if status := rr.Code; status != http.StatusOK {
		t.Errorf("handler returned wrong status code: got %v want %v", status, http.StatusOK)
	}

	var resp RunResponse
	if err := json.NewDecoder(rr.Body).Decode(&resp); err != nil {
		t.Fatalf("Failed to decode response: %v", err)
	}

	if !resp.Success {
		t.Errorf("Expected run to succeed, but got error: %s. Output: %s", resp.Error, resp.Output)
	}

	if !strings.Contains(resp.Output, "Test Run Success!") {
		t.Errorf("Expected output to contain 'Test Run Success!', got: %q", resp.Output)
	}
}
