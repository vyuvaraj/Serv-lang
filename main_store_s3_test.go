package main

import (
	"os"
	"testing"
)

func TestStoreS3PipelineCompilation(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_store_s3_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
import { put, get, transform } from "stdlib/store.srv"

test "store s3 pipeline transform call compilation" {
	store "servstore://admin:adminsecret@localhost:8081/my-bucket"

	assert put("doc.txt", "hello") == true
	assert get("doc.txt") == "hello"

	// Call transform on store
	let res = transform("doc.txt", ["upper.wasm"], "out.txt", true)
}
`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	// Compile it to check parsing, type-checking and code generation
	_, err = buildServNoExit(tmpFile.Name(), "temp_store_s3_pipeline.exe", "", "", "")
	if err != nil {
		t.Fatalf("Build failed: %v", err)
	}
	_ = os.Remove("temp_store_s3_pipeline.exe")
}
