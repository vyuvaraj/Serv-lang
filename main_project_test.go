package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestDirectoryMerge(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "serv_dir_merge_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create multiple .srv files that reference each other without imports
	fileAContent := `
export fn getValue() -> int {
	return 42
}
`
	fileBContent := `
fn useValue() -> int {
	return getValue()
}
`

	if err := os.WriteFile(filepath.Join(tmpDir, "a.srv"), []byte(fileAContent), 0644); err != nil {
		t.Fatalf("failed to write a.srv: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "b.srv"), []byte(fileBContent), 0644); err != nil {
		t.Fatalf("failed to write b.srv: %v", err)
	}

	// Compile the directory
	outBin := "test_service.exe"
	binPath, err := buildServNoExit(tmpDir, outBin, "")
	if err != nil {
		t.Fatalf("failed to build directory: %v", err)
	}
	defer os.Remove(binPath)

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Errorf("expected binary to be generated at %s", binPath)
	}
}

func TestManifestBuild(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "serv_manifest_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Create serv.toml pointing to a custom entry file
	tomlContent := `
name = "my-test-project"
version = "1.0.0"
entry = "custom_entry.srv"
`
	entryContent := `
fn someFunc() -> int {
	return 100
}
`

	if err := os.WriteFile(filepath.Join(tmpDir, "serv.toml"), []byte(tomlContent), 0644); err != nil {
		t.Fatalf("failed to write serv.toml: %v", err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "custom_entry.srv"), []byte(entryContent), 0644); err != nil {
		t.Fatalf("failed to write custom_entry.srv: %v", err)
	}

	// Compile the directory
	outBin := "test_service_manifest.exe"
	binPath, err := buildServNoExit(tmpDir, outBin, "")
	if err != nil {
		t.Fatalf("failed to build directory via manifest: %v", err)
	}
	defer os.Remove(binPath)

	if _, err := os.Stat(binPath); os.IsNotExist(err) {
		t.Errorf("expected binary to be generated at %s", binPath)
	}
}
