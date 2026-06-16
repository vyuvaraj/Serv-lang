package main

import (
	"os"
	"testing"
)

func TestS3PipelineCompilation(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_s3_pipeline_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
import { newClient, transform } from "stdlib/s3.srv"

test "s3 pipeline transform call compilation" {
	let client = newClient("http://localhost:8080", "admin", "adminsecret")
	assert client != nil

	// Call transform
	let res = client.transform("my-bucket", "input.txt", ["upper.wasm"], "output.txt", true)
	
	// Also test global transform call
	let res2 = transform("my-bucket", "input.txt", "upper.wasm", "output.txt", false)
}
`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	// Compile it to check parsing, type-checking and code generation
	_, err = buildServNoExit(tmpFile.Name(), "temp_s3_pipeline.exe", "")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	_ = os.Remove("temp_s3_pipeline.exe")
}
