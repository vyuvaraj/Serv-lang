package main

import (
	"archive/tar"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPackageRegistryIntegration(t *testing.T) {
	// 1. Setup mock registry server
	packages := make(map[string][]byte)

	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method == "POST" && strings.HasPrefix(r.URL.Path, "/publish") {
			name := r.URL.Query().Get("name")
			if name == "" {
				http.Error(w, "missing name query parameter", http.StatusBadRequest)
				return
			}
			data, err := io.ReadAll(r.Body)
			if err != nil {
				http.Error(w, err.Error(), http.StatusInternalServerError)
				return
			}
			packages[name] = data
			w.WriteHeader(http.StatusCreated)
			return
		}

		if r.Method == "GET" && strings.HasPrefix(r.URL.Path, "/packages/") {
			name := strings.TrimPrefix(r.URL.Path, "/packages/")
			name = strings.TrimSuffix(name, ".tar.gz")
			data, ok := packages[name]
			if !ok {
				http.Error(w, "package not found", http.StatusNotFound)
				return
			}
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write(data)
			return
		}

		http.Error(w, "not found", http.StatusNotFound)
	}))
	defer ts.Close()

	// 2. Set environment variables to point to mock registry
	os.Setenv("SERV_REGISTRY", ts.URL)
	defer os.Unsetenv("SERV_REGISTRY")

	// 3. Create temp workspace directories
	tmpDir, err := os.MkdirTemp("", "serv_registry_test")
	if err != nil {
		t.Fatalf("failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	origCwd, _ := os.Getwd()
	os.Chdir(tmpDir)
	defer os.Chdir(origCwd)

	// Create source package "mypkg"
	srcPkgDir := filepath.Join(tmpDir, "mypkg")
	if err := os.MkdirAll(srcPkgDir, 0755); err != nil {
		t.Fatalf("failed to create src pkg dir: %v", err)
	}
	pkgFileContent := `
export struct Helper {
	val: int
}
export fn helperFunc() -> string {
	return "registry_success"
}
`
	if err := os.WriteFile(filepath.Join(srcPkgDir, "index.srv"), []byte(pkgFileContent), 0644); err != nil {
		t.Fatalf("failed to write package file: %v", err)
	}

	// 4. Publish package using publishPackage
	publishPackage(srcPkgDir)

	// Verify that the package tarball was received by the mock registry
	if _, ok := packages["mypkg"]; !ok {
		t.Fatalf("registry did not receive the published package 'mypkg'")
	}

	// Verify the tarball content inside registry map
	rawBytes := packages["mypkg"]
	gr, err := gzip.NewReader(strings.NewReader(string(rawBytes)))
	if err != nil {
		t.Fatalf("registry tarball is not a valid gzip: %v", err)
	}
	tr := tar.NewReader(gr)
	foundFile := false
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("failed to read registry tarball headers: %v", err)
		}
		if hdr.Name == "index.srv" {
			foundFile = true
		}
	}
	gr.Close()
	if !foundFile {
		t.Errorf("registry tarball is missing index.srv")
	}

	// 5. Install package using installPackage
	installPackage("mypkg")

	// Verify extraction
	installedFile := filepath.Join("packages", "mypkg", "index.srv")
	if _, err := os.Stat(installedFile); err != nil {
		t.Fatalf("installed package file does not exist at %s: %v", installedFile, err)
	}

	// 6. Test compilation/import resolution with installed package
	importerContent := `
import { Helper, helperFunc } from "mypkg"
`
	importerPath := filepath.Join(tmpDir, "importer.srv")
	if err := os.WriteFile(importerPath, []byte(importerContent), 0644); err != nil {
		t.Fatalf("failed to write importer.srv: %v", err)
	}

	prog, err := parseWithDependencies(importerPath, make(map[string]int))
	if err != nil {
		t.Fatalf("failed to parse with package dependency: %v", err)
	}

	// Verify symbols are imported
	hasHelper := false
	hasHelperFunc := false
	for _, stmt := range prog.Statements {
		name := statementName(stmt)
		switch name {
		case "Helper":
			hasHelper = true
		case "helperFunc":
			hasHelperFunc = true
		}
	}

	if !hasHelper || !hasHelperFunc {
		t.Errorf("expected Helper and helperFunc to be imported via packages folder resolution")
	}
}
