package main

import (
	"bytes"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"time"

	"serv/compiler"
	"serv/dap"
)

// debugServ compiles <srvFile> with debug flags, extracts the source map from
// the generated Go code, starts Delve in DAP mode, and runs a DAP proxy on
// stdio so that VS Code can debug .srv files with full source-level support.
//
// Prerequisites: 'dlv' (Delve) must be installed and on PATH.
//
//	go install github.com/go-delve/delve/cmd/dlv@latest
func debugServ(srvFile string) {
	absPath, err := filepath.Abs(srvFile)
	if err != nil {
		fmt.Printf("debug: resolve path: %v\n", err)
		os.Exit(1)
	}

	// 1. Compile with debug flags (no inlining, no optimizations).
	fmt.Printf("[serv debug] Compiling %s...\n", filepath.Base(absPath))
	buildDir, genGoFile, err := buildServDebug(absPath)
	if err != nil {
		fmt.Printf("[serv debug] Build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[serv debug] Build dir: %s\n", buildDir)

	// 2. Parse the generated Go file to build the source map.
	sm, err := dap.ParseSourceMap(genGoFile, absPath)
	if err != nil {
		fmt.Printf("[serv debug] Source map: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[serv debug] Source map: %d entries\n", sm.Len())

	// 3. Find a free local TCP port for dlv.
	port, err := freePort()
	if err != nil {
		fmt.Printf("[serv debug] No free port: %v\n", err)
		os.Exit(1)
	}
	dlvAddr := fmt.Sprintf("localhost:%d", port)

	// 4. Verify dlv is available.
	if _, err := exec.LookPath("dlv"); err != nil {
		fmt.Println("[serv debug] 'dlv' not found in PATH.")
		fmt.Println("  Install Delve with:")
		fmt.Println("  go install github.com/go-delve/delve/cmd/dlv@latest")
		os.Exit(1)
	}

	// 5. Launch dlv in DAP mode (headless, listening on TCP).
	fmt.Printf("[serv debug] Starting Delve DAP server on %s...\n", dlvAddr)
	dlvCmd := exec.Command(
		"dlv", "dap",
		"--listen="+dlvAddr,
		"--headless",
		"--log=false",
	)
	dlvCmd.Dir = buildDir
	dlvCmd.Stderr = os.Stderr
	if err := dlvCmd.Start(); err != nil {
		fmt.Printf("[serv debug] Failed to start dlv: %v\n", err)
		os.Exit(1)
	}
	defer func() {
		if dlvCmd.Process != nil {
			dlvCmd.Process.Kill()
		}
	}()

	// 6. Wait for dlv to be ready (max 5s).
	if err := waitForPort(dlvAddr, 5*time.Second); err != nil {
		fmt.Printf("[serv debug] dlv not ready: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("[serv debug] Delve ready. Starting DAP proxy on stdio.\n")

	// 7. Run the DAP proxy on stdin/stdout.
	proxy := dap.NewProxy(sm, dlvAddr)
	if err := proxy.Run(os.Stdin, os.Stdout); err != nil {
		fmt.Fprintf(os.Stderr, "[serv debug] Proxy exited: %v\n", err)
	}
}

// buildServDebug compiles the .srv file with debug-friendly Go build flags:
//   - -gcflags=all=-N -l  disables inlining and optimizations so Delve can
//     accurately step through every source line.
//
// Returns the build directory path and the generated main.go path.
func buildServDebug(absPath string) (buildDir, genGoFile string, err error) {
	program, err := parseWithDependencies(absPath, make(map[string]bool))
	if err != nil {
		return "", "", err
	}

	// Static analysis (warnings only — don't block debug builds on style issues).
	source, _ := os.ReadFile(absPath)
	diags := compiler.Analyze(program)
	hasErrors := false
	for _, d := range diags {
		if d.Severity == "error" {
			hasErrors = true
		}
	}
	if hasErrors {
		fmt.Print(compiler.FormatAnalysisDiagnostics(diags, string(source)))
		return "", "", fmt.Errorf("compilation failed due to type errors")
	}

	codegen := compiler.NewCodegen(program)
	goCode, err := codegen.Generate()
	if err != nil {
		return "", "", err
	}
	goCode += "\n" + codegen.GenerateHelpers()
	goCode += "\n" + codegen.GenerateMainFunc()

	buildDir = buildDirFor(absPath)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", "", err
	}

	genGoFile = filepath.Join(buildDir, "main.go")
	if err := os.WriteFile(genGoFile, []byte(goCode), 0644); err != nil {
		return "", "", err
	}

	if err := ensureBuildGoMod(buildDir); err != nil {
		return "", "", fmt.Errorf("setup build module: %w", err)
	}
	if err := runGoModTidy(buildDir); err != nil {
		return "", "", fmt.Errorf("go mod tidy: %w", err)
	}

	goPath, err := resolveGoPath()
	if err != nil {
		return "", "", err
	}

	// Build with debug flags: disable inlining (-l) and optimizations (-N).
	cmd := exec.Command(goPath, "build",
		"-gcflags=all=-N -l",
		"-o", "debug_service",
		".",
	)
	cmd.Dir = buildDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return "", "", fmt.Errorf("go build (debug): %v: %s", err, stderr.String())
	}

	return buildDir, genGoFile, nil
}

// freePort finds an available TCP port on localhost.
func freePort() (int, error) {
	ln, err := net.Listen("tcp", "localhost:0")
	if err != nil {
		return 0, err
	}
	port := ln.Addr().(*net.TCPAddr).Port
	ln.Close()
	return port, nil
}

// waitForPort blocks until the TCP address is connectable or the deadline passes.
func waitForPort(addr string, timeout time.Duration) error {
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		conn, err := net.DialTimeout("tcp", addr, 200*time.Millisecond)
		if err == nil {
			conn.Close()
			return nil
		}
		time.Sleep(100 * time.Millisecond)
	}
	return fmt.Errorf("timeout waiting for %s after %s", addr, timeout)
}
