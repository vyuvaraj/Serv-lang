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

func TestNewAndDeployK8s(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "serv_new_deploy_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	projDir := filepath.Join(tmpDir, "test-api-proj")

	// 1. Test template creation (api template)
	createNewProject(projDir, "api")

	// Verify scaffolded files
	if _, err := os.Stat(filepath.Join(projDir, "main.srv")); os.IsNotExist(err) {
		t.Error("expected main.srv to be created")
	}
	if _, err := os.Stat(filepath.Join(projDir, "config.yml")); os.IsNotExist(err) {
		t.Error("expected config.yml to be created")
	}
	if _, err := os.Stat(filepath.Join(projDir, "main_test.srv")); os.IsNotExist(err) {
		t.Error("expected main_test.srv to be created")
	}

	// 2. Test deploy command for k8s target
	deployServ(filepath.Join(projDir, "main.srv"), "k8s")

	// Verify generated Kubernetes manifests and Dockerfile
	if _, err := os.Stat(filepath.Join(projDir, "Dockerfile")); os.IsNotExist(err) {
		t.Error("expected Dockerfile to be created")
	}
	if _, err := os.Stat(filepath.Join(projDir, "k8s", "deployment.yaml")); os.IsNotExist(err) {
		t.Error("expected k8s/deployment.yaml to be created")
	}
	if _, err := os.Stat(filepath.Join(projDir, "k8s", "service.yaml")); os.IsNotExist(err) {
		t.Error("expected k8s/service.yaml to be created")
	}
	if _, err := os.Stat(filepath.Join(projDir, "k8s", "configmap.yaml")); os.IsNotExist(err) {
		t.Error("expected k8s/configmap.yaml to be created")
	}

	// Read and verify deployment contains volumeMount
	depContent, err := os.ReadFile(filepath.Join(projDir, "k8s", "deployment.yaml"))
	if err != nil {
		t.Fatalf("failed to read deployment.yaml: %v", err)
	}
	if !contains(string(depContent), "config-volume") {
		t.Error("expected deployment.yaml to contain config-volume mount reference")
	}
}

func contains(s, substr string) bool {
	return len(s) >= len(substr) && (s == substr || len(substr) == 0 || stringsContains(s, substr))
}

func stringsContains(s, substr string) bool {
	for i := 0; i+len(substr) <= len(s); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
