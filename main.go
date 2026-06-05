package main

import (
	"bufio"
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"serv/compiler"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]

	switch command {
	case "build":
		// Parse -o flag from anywhere in the args
		outputBinary := "service.exe"
		var buildArgs []string
		rawArgs := os.Args[2:]
		for i := 0; i < len(rawArgs); i++ {
			if rawArgs[i] == "-o" && i+1 < len(rawArgs) {
				outputBinary = rawArgs[i+1]
				i++ // skip the value
			} else {
				buildArgs = append(buildArgs, rawArgs[i])
			}
		}
		if len(buildArgs) < 1 {
			fmt.Println("Usage: serv build <file.srv> [-o <output>]")
			os.Exit(1)
		}
		buildServ(buildArgs[0], outputBinary)

	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		watchFlag := runCmd.Bool("watch", false, "Watch files and hot-reload")
		if err := runCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := runCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv run <file.srv> [--watch]")
			os.Exit(1)
		}

		if *watchFlag {
			runServWatch(args[0])
		} else {
			runServ(args[0], args[1:])
		}

	case "dockerize":
		dockerCmd := flag.NewFlagSet("dockerize", flag.ExitOnError)
		if err := dockerCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := dockerCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv dockerize <file.srv>")
			os.Exit(1)
		}
		dockerizeServ(args[0])

	case "test":
		testCmd := flag.NewFlagSet("test", flag.ExitOnError)
		coverFlag := testCmd.Bool("cover", false, "Report test coverage")
		if err := testCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := testCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv test [--cover] <file.srv>")
			os.Exit(1)
		}
		runTests(args[0], *coverFlag)

	case "lint":
		lintCmd := flag.NewFlagSet("lint", flag.ExitOnError)
		if err := lintCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := lintCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv lint <file.srv>")
			os.Exit(1)
		}
		runLint(args[0])

	case "add":
		addCmd := flag.NewFlagSet("add", flag.ExitOnError)
		if err := addCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := addCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv add <go-package-path>")
			fmt.Println("Example: serv add github.com/google/uuid")
			fmt.Println("         serv add encoding/base64")
			os.Exit(1)
		}
		addPackage(args[0])

	case "packages":
		listPackages()

	case "remove":
		if len(os.Args) < 3 {
			fmt.Println("Usage: serv remove <package-name>")
			os.Exit(1)
		}
		removePackage(os.Args[2])

	case "fmt":
		fmtCmd := flag.NewFlagSet("fmt", flag.ExitOnError)
		checkOnly := fmtCmd.Bool("check", false, "Check if file is formatted (exit 1 if not)")
		if err := fmtCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := fmtCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv fmt [--check] <file.srv>")
			os.Exit(1)
		}
		formatFile(args[0], *checkOnly)

	case "repl":
		runREPL()

	case "init":
		initProject()

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Serv: A Programming Language for Background Services")
	fmt.Println("Usage:")
	fmt.Println("  serv init [name]                           Create a new Serv project")
	fmt.Println("  serv build <file.srv> [-o <output_binary>]  Compile Serv code to native binary")
	fmt.Println("  serv run <file.srv> [--watch]              Compile and run Serv code immediately (with optional hot reload)")
	fmt.Println("  serv test [--cover] <file.srv>             Run tests defined in a Serv file")
	fmt.Println("  serv lint <file.srv>                       Validate syntax and check for errors")
	fmt.Println("  serv fmt <file.srv>                        Format a Serv file")
	fmt.Println("  serv repl                                  Interactive shell for quick experiments")
	fmt.Println("  serv add <go-package>                      Generate .srv.d declaration for a Go package")
	fmt.Println("  serv dockerize <file.srv>                  Generate a Dockerfile for the Serv service")
}

func initProject() {
	name := "my-service"
	if len(os.Args) >= 3 {
		name = os.Args[2]
	}

	// Create project directory
	if err := os.MkdirAll(name, 0755); err != nil {
		fmt.Printf("Failed to create directory: %v\n", err)
		os.Exit(1)
	}

	// main.srv
	mainSrv := `import { ok } from "stdlib/response"

server "8080"

route "GET" "/health" (req) {
    return ok({ "status": "healthy" })
}

route "GET" "/api/hello" (req) {
    let name = req.params.name
    if name == nil {
        return ok({ "message": "Hello, world!" })
    }
    return ok({ "message": f"Hello, {name}!" })
}
`
	if err := os.WriteFile(filepath.Join(name, "main.srv"), []byte(mainSrv), 0644); err != nil {
		fmt.Printf("Failed to write main.srv: %v\n", err)
		os.Exit(1)
	}

	// config.yml
	configYml := `server:
  port: "8080"

log:
  level: "info"
  format: "text"
`
	if err := os.WriteFile(filepath.Join(name, "config.yml"), []byte(configYml), 0644); err != nil {
		fmt.Printf("Failed to write config.yml: %v\n", err)
		os.Exit(1)
	}

	// test file
	testSrv := `test "health check returns ok" {
    // TODO: add your tests here
    assert true
}
`
	if err := os.WriteFile(filepath.Join(name, "main_test.srv"), []byte(testSrv), 0644); err != nil {
		fmt.Printf("Failed to write main_test.srv: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Created project: %s/\n", name)
	fmt.Println("")
	fmt.Println("  Files:")
	fmt.Println("    main.srv       — Your service (routes, logic)")
	fmt.Println("    main_test.srv  — Tests")
	fmt.Println("    config.yml     — Runtime configuration")
	fmt.Println("")
	fmt.Println("  Get started:")
	fmt.Printf("    cd %s\n", name)
	fmt.Println("    serv run main.srv --watch")
	fmt.Println("")
	fmt.Println("  Then visit: http://localhost:8080/health")
}

func runLint(srvFile string) {
	absPath, err := filepath.Abs(srvFile)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	program, err := parseWithDependencies(absPath, make(map[string]bool))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
	}

	// Run static analysis
	source, _ := os.ReadFile(absPath)
	diags := compiler.Analyze(program)
	if len(diags) > 0 {
		fmt.Print(compiler.FormatAnalysisDiagnostics(diags, string(source)))
		// Count errors vs warnings
		errors := 0
		warnings := 0
		for _, d := range diags {
			if d.Severity == "error" {
				errors++
			} else {
				warnings++
			}
		}
		if errors > 0 {
			fmt.Printf("%d error(s), %d warning(s)\n", errors, warnings)
			os.Exit(1)
		}
		fmt.Printf("OK (%d warning(s))\n", warnings)
		return
	}
	fmt.Println("OK")
}

func buildServ(srvFile, outputBinary string) string {
	absPath, err := buildServNoExit(srvFile, outputBinary)
	if err != nil {
		fmt.Printf("Build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Build successful! Binary: %s\n", absPath)
	return absPath
}

func buildServNoExit(srvFile, outputBinary string) (string, error) {
	absPath, err := filepath.Abs(srvFile)
	if err != nil {
		return "", err
	}

	program, err := parseWithDependencies(absPath, make(map[string]bool))
	if err != nil {
		return "", err
	}

	// Run static analysis — show warnings but don't fail build
	source, _ := os.ReadFile(absPath)
	diags := compiler.Analyze(program)
	hasErrors := false
	if len(diags) > 0 {
		fmt.Print(compiler.FormatAnalysisDiagnostics(diags, string(source)))
		for _, d := range diags {
			if d.Severity == "error" {
				hasErrors = true
			}
		}
	}
	if hasErrors {
		return "", fmt.Errorf("compilation failed due to type errors")
	}

	codegen := compiler.NewCodegen(program)
	goCode, err := codegen.Generate()
	if err != nil {
		return "", err
	}

	goCode += "\n" + codegen.GenerateHelpers()
	goCode += "\n" + codegen.GenerateMainFunc()

	buildDir := buildDirFor(absPath)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		return "", err
	}

	// Remove stale test files from previous test runs
	_ = os.Remove(filepath.Join(buildDir, "service.go"))
	_ = os.Remove(filepath.Join(buildDir, "serv_test.go"))

	genGoFile := filepath.Join(buildDir, "main.go")
	if err := os.WriteFile(genGoFile, []byte(goCode), 0644); err != nil {
		return "", err
	}

	// Ensure the build directory has a go.mod that can find serv/runtime
	if err := ensureBuildGoMod(buildDir); err != nil {
		return "", fmt.Errorf("failed to setup build module: %w", err)
	}

	goPath, err := resolveGoPath()
	if err != nil {
		return "", fmt.Errorf("cannot find Go compiler: %w", err)
	}
	cmd := exec.Command(goPath, "build", "-o", filepath.Join(filepath.Dir(absPath), outputBinary), ".")
	cmd.Dir = buildDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}

	return filepath.Join(filepath.Dir(absPath), outputBinary), nil
}

func runServ(srvFile string, extraArgs []string) {
	binPath, err := buildServNoExit(srvFile, "temp_service.exe")
	if err != nil {
		fmt.Printf("Build failed: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(binPath)

	fmt.Printf("Running native service: %s...\n", binPath)
	cmd := exec.Command(binPath, extraArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Service exited with error: %v\n", err)
		os.Exit(1)
	}
}

func runServWatch(srvFile string) {
	fmt.Printf("Starting Serv in Watch Mode: %s...\n", srvFile)

	var cmd *exec.Cmd

	restart := func() {
		if cmd != nil && cmd.Process != nil {
			fmt.Println("[WATCH] File change detected. Restarting service...")
			cmd.Process.Kill()
			cmd.Wait()
		}

		binPath, err := buildServNoExit(srvFile, "watch_service.exe")
		if err != nil {
			fmt.Printf("[WATCH] Rebuild failed:\n%v\n", err)
			return
		}

		fmt.Printf("[WATCH] Starting service: %s\n", binPath)
		cmd = exec.Command(binPath)
		cmd.Stdout = os.Stdout
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			fmt.Printf("[WATCH] Failed to start service: %v\n", err)
		}
	}

	restart()
	defer func() {
		if cmd != nil && cmd.Process != nil {
			cmd.Process.Kill()
		}
	}()

	watchDir := filepath.Dir(srvFile)
	lastMods := getFileModTimes(watchDir)

	for {
		time.Sleep(500 * time.Millisecond)
		currentMods := getFileModTimes(watchDir)

		changed := false
		for path, mtime := range currentMods {
			oldTime, exists := lastMods[path]
			if !exists || mtime.After(oldTime) {
				changed = true
				break
			}
		}

		if !changed && len(currentMods) != len(lastMods) {
			changed = true
		}

		if changed {
			lastMods = currentMods
			restart()
		}
	}
}

func runTests(srvFile string, withCoverage bool) {
	absPath, err := filepath.Abs(srvFile)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	program, err := parseWithDependencies(absPath, make(map[string]bool))
	if err != nil {
		fmt.Printf("Parse error: %v\n", err)
		os.Exit(1)
	}

	cg := compiler.NewCodegen(program)
	goCode, err := cg.Generate()
	if err != nil {
		fmt.Printf("Codegen error: %v\n", err)
		os.Exit(1)
	}

	if !cg.HasTests() {
		fmt.Println("No tests found in", srvFile)
		return
	}

	buildDir := buildDirFor(absPath)
	if err := os.MkdirAll(buildDir, 0755); err != nil {
		fmt.Printf("Failed to create build dir: %v\n", err)
		os.Exit(1)
	}

	// Ensure the build directory has a go.mod that can find serv/runtime
	if err := ensureBuildGoMod(buildDir); err != nil {
		fmt.Printf("Failed to setup build module: %v\n", err)
		os.Exit(1)
	}

	// Clean stale Go files from previous builds to prevent conflicts
	for _, name := range []string{"main.go", "service.go", "serv_test.go", "coverage.out"} {
		_ = os.Remove(filepath.Join(buildDir, name))
	}

	// Write service.go: all generated declarations (functions, init blocks, etc.)
	serviceCode := goCode + "\n" + cg.GenerateHelpers()
	if err := os.WriteFile(filepath.Join(buildDir, "service.go"), []byte(serviceCode), 0644); err != nil {
		fmt.Printf("Failed to write service.go: %v\n", err)
		os.Exit(1)
	}

	// Write a minimal main.go stub (go test needs a package main with a main func)
	mainStub := "// Code generated by Serv compiler. DO NOT EDIT.\npackage main\n\nfunc main() {}\n"
	if err := os.WriteFile(filepath.Join(buildDir, "main.go"), []byte(mainStub), 0644); err != nil {
		fmt.Printf("Failed to write main.go: %v\n", err)
		os.Exit(1)
	}

	// Write serv_test.go
	testCode := cg.GenerateTests()
	testFile := filepath.Join(buildDir, "serv_test.go")
	if err := os.WriteFile(testFile, []byte(testCode), 0644); err != nil {
		fmt.Printf("Failed to write test file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Running tests from %s...\n", srvFile)
	goPath, err := resolveGoPath()
	if err != nil {
		fmt.Printf("Cannot find Go compiler: %v\n", err)
		os.Exit(1)
	}

	// Build go test command with appropriate flags
	testArgs := []string{"test", "-v"}
	if withCoverage {
		coverFile := filepath.Join(buildDir, "coverage.out")
		testArgs = append(testArgs, "-coverprofile="+coverFile)
	}
	testArgs = append(testArgs, "./...")

	cmd := exec.Command(goPath, testArgs...)
	cmd.Dir = buildDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	testErr := cmd.Run()

	// Show coverage summary if requested
	if withCoverage {
		coverFile := filepath.Join(buildDir, "coverage.out")
		if _, statErr := os.Stat(coverFile); statErr == nil {
			fmt.Println()
			fmt.Println("--- Coverage ---")
			coverCmd := exec.Command(goPath, "tool", "cover", "-func="+coverFile)
			coverCmd.Dir = buildDir
			var coverOut bytes.Buffer
			coverCmd.Stdout = &coverOut
			coverCmd.Stderr = os.Stderr
			if err := coverCmd.Run(); err == nil {
				// Extract just the total line
				lines := strings.Split(coverOut.String(), "\n")
				for _, line := range lines {
					if strings.Contains(line, "total:") {
						fmt.Println(strings.TrimSpace(line))
					}
				}
			}
			fmt.Printf("Coverage profile: %s\n", coverFile)
		}
	}

	if testErr != nil {
		fmt.Printf("Tests failed: %v\n", testErr)
		os.Exit(1)
	}
}

func getFileModTimes(dir string) map[string]time.Time {

	mods := make(map[string]time.Time)
	filepath.Walk(dir, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if strings.Contains(path, ".build") ||
			strings.Contains(path, "watch_service.exe") ||
			strings.Contains(path, "service.exe") ||
			strings.Contains(path, "temp_service.exe") {
			return nil
		}
		if !info.IsDir() && (strings.HasSuffix(path, ".srv") || strings.HasSuffix(path, ".py")) {
			mods[path] = info.ModTime()
		}
		return nil
	})
	return mods
}

func dockerizeServ(srvFile string) {
	absPath, err := filepath.Abs(srvFile)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	baseName := filepath.Base(srvFile)
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
`, baseName)

	dockerfilePath := filepath.Join(filepath.Dir(absPath), "Dockerfile")
	if err := os.WriteFile(dockerfilePath, []byte(dockerfileContent), 0644); err != nil {
		fmt.Printf("Failed to write Dockerfile: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Dockerfile successfully generated at: %s\n", dockerfilePath)
	fmt.Println("You can now build and run your Serv service using Docker:")
	fmt.Println("  docker build -t serv-service .")
	fmt.Println("  docker run -p 8080:8080 -e PORT=8080 serv-service")
}

// resolveGoPath finds the Go compiler binary, checking PATH first then common install locations.
func resolveGoPath() (string, error) {
	// First try PATH lookup (works cross-platform)
	if path, err := exec.LookPath("go"); err == nil {
		return path, nil
	}
	// Fallback: check common install locations
	candidates := []string{
		"C:\\Program Files\\Go\\bin\\go.exe",
		"/usr/local/go/bin/go",
		"/usr/bin/go",
	}
	for _, c := range candidates {
		if _, err := os.Stat(c); err == nil {
			return c, nil
		}
	}
	return "", fmt.Errorf("'go' not found in PATH or common install locations; ensure Go is installed and in your PATH")
}

// buildDirFor returns a unique .build subdirectory based on the source file path hash.
// This prevents concurrent builds of different .srv files from clobbering each other.
func buildDirFor(absPath string) string {
	h := sha256.Sum256([]byte(absPath))
	short := hex.EncodeToString(h[:4]) // 8 hex chars is enough uniqueness
	return filepath.Join(filepath.Dir(absPath), ".build", short)
}

// ensureBuildGoMod creates a go.mod in the build directory that can resolve serv/runtime.
// It uses a replace directive pointing to the Serv installation directory.
func ensureBuildGoMod(buildDir string) error {
	goModPath := filepath.Join(buildDir, "go.mod")

	// Find the Serv installation root (where runtime/ and go.mod live)
	servRoot := findServRoot()
	if servRoot == "" {
		return fmt.Errorf("cannot find Serv installation (runtime/ directory). Set SERV_HOME or ensure serv.exe is next to the runtime/ folder")
	}

	// Read the Serv project's go.mod to get the Go version and dependencies
	servGoMod := filepath.Join(servRoot, "go.mod")
	servGoModContent, err := os.ReadFile(servGoMod)
	if err != nil {
		return fmt.Errorf("cannot read %s: %w", servGoMod, err)
	}

	// Extract go version from Serv's go.mod
	goVersion := "1.22"
	for _, line := range strings.Split(string(servGoModContent), "\n") {
		line = strings.TrimSpace(line)
		if strings.HasPrefix(line, "go ") {
			goVersion = strings.TrimPrefix(line, "go ")
			break
		}
	}

	// Generate go.mod for the build directory
	goMod := fmt.Sprintf(`module serv-build

go %s

require serv v0.0.0

replace serv v0.0.0 => %s
`, goVersion, filepath.ToSlash(servRoot))

	if err := os.WriteFile(goModPath, []byte(goMod), 0644); err != nil {
		return err
	}

	// Copy go.sum from Serv root (needed for transitive dependencies)
	servGoSum := filepath.Join(servRoot, "go.sum")
	if sumContent, err := os.ReadFile(servGoSum); err == nil {
		os.WriteFile(filepath.Join(buildDir, "go.sum"), sumContent, 0644)
	}

	// Run go mod tidy to resolve the require/replace properly
	goPath, err := resolveGoPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(goPath, "mod", "tidy")
	cmd.Dir = buildDir
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		// Non-fatal — the build might still work without tidy
		_ = err
	}

	return nil
}

// findServRoot locates the Serv installation directory (where runtime/ and go.mod live).
func findServRoot() string {
	// 1. SERV_HOME environment variable
	if home := os.Getenv("SERV_HOME"); home != "" {
		if _, err := os.Stat(filepath.Join(home, "runtime")); err == nil {
			return home
		}
	}

	// 2. Next to the running binary
	if exePath, err := os.Executable(); err == nil {
		exeDir := filepath.Dir(exePath)
		if _, err := os.Stat(filepath.Join(exeDir, "runtime")); err == nil {
			return exeDir
		}
		// One level up (for bin/serv layout)
		parent := filepath.Dir(exeDir)
		if _, err := os.Stat(filepath.Join(parent, "runtime")); err == nil {
			return parent
		}
	}

	// 3. Current working directory (developer mode)
	if cwd, err := os.Getwd(); err == nil {
		if _, err := os.Stat(filepath.Join(cwd, "runtime")); err == nil {
			return cwd
		}
	}

	return ""
}

// formatFile formats a .srv file with consistent indentation and spacing.
func formatFile(srvFile string, checkOnly bool) {
	content, err := os.ReadFile(srvFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	lines := strings.Split(string(content), "\n")
	var result []string
	indentLevel := 0
	indent := "    " // 4 spaces
	prevEmpty := false
	prevWasBlock := false // track if previous non-empty line ended a block or was a top-level decl

	// Top-level keywords that should have a blank line before them (if not already)
	topLevelKeywords := map[string]bool{
		"server": true, "database": true, "cache": true, "broker": true,
		"route": true, "fn": true, "every": true, "cron": true,
		"subscribe": true, "test": true, "struct": true, "interface": true,
		"middleware": true, "ws": true, "enum": true, "validate": true,
		"type": true, "export": true,
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Collapse multiple consecutive empty lines into one
		if trimmed == "" {
			if !prevEmpty {
				result = append(result, "")
				prevEmpty = true
			}
			continue
		}
		prevEmpty = false

		// Count braces outside of strings to determine indent changes
		netBraces := countNetBraces(trimmed)

		// Decrease indent BEFORE writing if line starts with closing brace/bracket
		if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") {
			indentLevel--
			if indentLevel < 0 {
				indentLevel = 0
			}
		}

		// Insert blank line before top-level keywords (at indent 0) if previous wasn't empty
		if indentLevel == 0 && i > 0 {
			firstWord := strings.Fields(trimmed)[0]
			// Strip trailing punctuation
			firstWord = strings.TrimRight(firstWord, "({[")
			if topLevelKeywords[firstWord] && !prevEmpty && len(result) > 0 && result[len(result)-1] != "" {
				if !prevWasBlock {
					result = append(result, "")
				}
			}
		}

		// Apply indentation
		formatted := strings.Repeat(indent, indentLevel) + trimmed
		result = append(result, formatted)

		// Track if this line is a closing brace (end of block)
		prevWasBlock = strings.HasPrefix(trimmed, "}")

		// Increase indent AFTER writing if line has net opening braces
		if netBraces > 0 {
			indentLevel += netBraces
		} else if netBraces < 0 && !strings.HasPrefix(trimmed, "}") && !strings.HasPrefix(trimmed, "]") {
			// Lines like `} else {` — net is 0, already handled
			indentLevel += netBraces
			if indentLevel < 0 {
				indentLevel = 0
			}
		}
	}

	// Remove trailing empty lines, then ensure single newline at end
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	output := strings.Join(result, "\n") + "\n"

	if checkOnly {
		// Normalize line endings for cross-platform comparison
		normalizedOutput := strings.ReplaceAll(output, "\r\n", "\n")
		normalizedContent := strings.ReplaceAll(string(content), "\r\n", "\n")
		if normalizedOutput != normalizedContent {
			fmt.Printf("%s: not formatted\n", srvFile)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(srvFile, []byte(output), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Formatted: %s\n", srvFile)
}

// countNetBraces counts opening minus closing braces/brackets in a line,
// ignoring braces inside string literals and comments.
func countNetBraces(line string) int {
	net := 0
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inString {
			if ch == '\\' && i+1 < len(line) {
				i++ // skip escaped char
				continue
			}
			if ch == stringChar {
				inString = false
			}
			continue
		}
		// Check for comment start
		if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
			break // rest of line is comment
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			inString = true
			stringChar = ch
			continue
		}
		if ch == '{' {
			net++
		} else if ch == '}' {
			net--
		}
	}
	return net
}

// runREPL starts an interactive Serv shell.
// Expressions are evaluated immediately, variable declarations persist across lines.
func runREPL() {
	fmt.Println("Serv REPL v1.0 — type expressions, 'let' declarations, or 'exit' to quit")
	fmt.Println("Examples: let x = 5, x + 10, \"hello\".toUpper(), log.info(\"hi\")")
	fmt.Println()

	goPath, err := resolveGoPath()
	if err != nil {
		fmt.Printf("Cannot find Go compiler: %v\n", err)
		os.Exit(1)
	}

	// Build directory for REPL sessions (inside project so go.mod is found)
	replDir := filepath.Join(".build", "repl")
	os.MkdirAll(replDir, 0755)

	// Accumulated state: variable declarations and function definitions
	var declarations []string
	scanner := bufio.NewScanner(os.Stdin)

	for {
		fmt.Print("serv> ")
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		if line == "exit" || line == "quit" {
			fmt.Println("Bye!")
			break
		}
		if line == "clear" {
			declarations = nil
			fmt.Println("(state cleared)")
			continue
		}
		if line == "state" {
			if len(declarations) == 0 {
				fmt.Println("(no declarations)")
			} else {
				for _, d := range declarations {
					fmt.Println("  " + d)
				}
			}
			continue
		}

		// Parse the input
		lexer := compiler.NewLexer(line)
		parser := compiler.NewParser(lexer)
		program := parser.ParseProgram()

		if len(parser.Errors()) > 0 {
			for _, e := range parser.Errors() {
				fmt.Println("  error:", e)
			}
			continue
		}

		if len(program.Statements) == 0 {
			continue
		}

		// Determine if this is a declaration (let, fn) or an expression to evaluate
		stmt := program.Statements[0]
		isDecl := false
		switch stmt.(type) {
		case *compiler.LetStmt, *compiler.FnDecl, *compiler.DestructureLetStmt:
			isDecl = true
		}

		// Build a temporary Serv program that includes all prior state + current line
		var srvSource strings.Builder
		for _, d := range declarations {
			srvSource.WriteString(d + "\n")
		}

		if isDecl {
			srvSource.WriteString(line + "\n")
		} else {
			// Expression: wrap in a let so it gets evaluated
			srvSource.WriteString(fmt.Sprintf("let _result = %s\n", line))
		}

		// Compile and run
		tmpFile := filepath.Join(replDir, "repl_input.srv")
		os.WriteFile(tmpFile, []byte(srvSource.String()), 0644)

		// Determine what to print
		printExpr := ""
		if isDecl {
			if letStmt, ok := stmt.(*compiler.LetStmt); ok {
				printExpr = letStmt.Name
			}
		} else {
			printExpr = "_result"
		}

		output, buildErr := compileAndRunREPL(goPath, tmpFile, replDir, printExpr)
		if buildErr != nil {
			fmt.Printf("  error: %s\n", buildErr)
			continue
		}

		// Print output
		output = strings.TrimSpace(output)
		if output != "" {
			fmt.Println(output)
		}

		// If it was a declaration, save it to state
		if isDecl {
			declarations = append(declarations, line)
		}
	}

	// .build/repl acts as a cache, no cleanup needed
}

// compileAndRunREPL compiles a temporary .srv file and runs it briefly to capture output.
func compileAndRunREPL(goPath, srvFile, workDir string, printExpr string) (string, error) {
	// Parse and generate
	program, err := parseWithDependencies(srvFile, make(map[string]bool))
	if err != nil {
		return "", err
	}

	cg := compiler.NewCodegen(program)
	goCode, err := cg.Generate()
	if err != nil {
		return "", err
	}

	goCode += "\n" + cg.GenerateHelpers()

	// REPL main: print the result expression, then exit
	if printExpr != "" {
		goCode += fmt.Sprintf("\nfunc main() {\n\tfmt.Println(%s)\n}\n", printExpr)
	} else {
		goCode += "\nfunc main() {}\n"
	}

	// Write Go file
	mainFile := filepath.Join(workDir, "main.go")
	os.WriteFile(mainFile, []byte(goCode), 0644)

	// Remove stale binary to force rebuild
	absWorkDir, _ := filepath.Abs(workDir)
	binPath := filepath.Join(absWorkDir, "repl_bin.exe")
	os.Remove(binPath)

	// Build from module root
	buildCmd := exec.Command(goPath, "build", "-o", binPath, "./"+workDir)
	var buildErr bytes.Buffer
	buildCmd.Stderr = &buildErr
	if err := buildCmd.Run(); err != nil {
		return "", fmt.Errorf("%s", strings.TrimSpace(buildErr.String()))
	}

	// Run the compiled binary and capture output
	runCmd := exec.Command(binPath)
	output, runErr := runCmd.CombinedOutput()
	if runErr != nil {
		// Ignore exit errors (timeouts etc) — just return what we got
		if len(output) > 0 {
			return string(output), nil
		}
	}

	return string(output), nil
}

func copyFileIfExists(src, dst string) {
	data, err := os.ReadFile(src)
	if err == nil {
		os.WriteFile(dst, data, 0644)
	}
}

// addPackage generates a .srv.d declaration file for a Go package.
func addPackage(pkgPath string) {
	// Run go doc to get package documentation
	goPath, err := resolveGoPath()
	if err != nil {
		fmt.Printf("Cannot find Go compiler: %v\n", err)
		os.Exit(1)
	}

	// Step 1: Try to go get the package first (add to go.mod)
	fmt.Printf("Fetching %s...\n", pkgPath)
	getCmd := exec.Command(goPath, "get", pkgPath)
	getCmd.Stderr = os.Stderr
	if err := getCmd.Run(); err != nil {
		// Not fatal — package might already be available (stdlib)
		fmt.Printf("Note: go get %s returned an error (may already be available)\n", pkgPath)
	}

	// Step 2: Run go doc to extract function signatures
	cmd := exec.Command(goPath, "doc", "-all", pkgPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to get package info for %s: %v\n%s\n", pkgPath, err, stderr.String())
		fmt.Println("Make sure the package is available: go get", pkgPath)
		os.Exit(1)
	}

	// Step 3: Parse the go doc output to extract exported functions
	output := stdout.String()
	functions := parseGoDocFunctions(output)

	if len(functions) == 0 {
		fmt.Printf("No exported functions found in %s\n", pkgPath)
		os.Exit(1)
	}

	// Step 4: Generate .srv.d file
	pkgName := filepath.Base(pkgPath)
	declDir := "declarations"
	os.MkdirAll(declDir, 0755)

	var content strings.Builder
	content.WriteString(fmt.Sprintf("// Auto-generated declaration for %s\n", pkgPath))
	content.WriteString(fmt.Sprintf("// Generated by: serv add %s\n\n", pkgPath))
	content.WriteString(fmt.Sprintf("declare module \"%s\" {\n", pkgPath))
	for _, fn := range functions {
		if fn.multiReturn {
			content.WriteString(fmt.Sprintf("    fn! %s(%s)", fn.name, fn.params))
		} else {
			content.WriteString(fmt.Sprintf("    fn %s(%s)", fn.name, fn.params))
		}
		if fn.returnType != "" {
			content.WriteString(fmt.Sprintf(" -> %s", fn.returnType))
		}
		content.WriteString("\n")
	}
	content.WriteString("}\n")

	declFile := filepath.Join(declDir, pkgName+".srv.d")
	if err := os.WriteFile(declFile, []byte(content.String()), 0644); err != nil {
		fmt.Printf("Failed to write declaration file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Generated declaration: %s (%d functions)\n", declFile, len(functions))
	fmt.Printf("\nUsage in your .srv file:\n")
	fmt.Printf("  import %s from \"%s\"\n", pkgName, pkgPath)
	fmt.Printf("  let result = %s.FunctionName(args)\n", pkgName)
}

// listPackages shows all installed declaration files.
func listPackages() {
	declDir := "declarations"
	entries, err := os.ReadDir(declDir)
	if err != nil {
		fmt.Println("No packages installed yet. Use: serv add <package>")
		return
	}

	fmt.Println("Installed packages:")
	fmt.Println()
	for _, e := range entries {
		if !e.IsDir() && strings.HasSuffix(e.Name(), ".srv.d") {
			name := strings.TrimSuffix(e.Name(), ".srv.d")
			// Read first two lines to get the package path
			content, _ := os.ReadFile(filepath.Join(declDir, e.Name()))
			lines := strings.Split(string(content), "\n")
			pkgPath := name
			for _, line := range lines {
				if strings.Contains(line, "declare module") {
					start := strings.Index(line, "\"")
					end := strings.LastIndex(line, "\"")
					if start >= 0 && end > start {
						pkgPath = line[start+1 : end]
					}
					break
				}
			}
			fmt.Printf("  %-20s %s\n", name, pkgPath)
		}
	}
}

// removePackage deletes a declaration file.
func removePackage(name string) {
	declDir := "declarations"
	declFile := filepath.Join(declDir, name+".srv.d")
	if _, err := os.Stat(declFile); os.IsNotExist(err) {
		fmt.Printf("Package '%s' not found in declarations/\n", name)
		fmt.Println("Use: serv packages — to see installed packages")
		os.Exit(1)
	}
	if err := os.Remove(declFile); err != nil {
		fmt.Printf("Failed to remove %s: %v\n", declFile, err)
		os.Exit(1)
	}
	fmt.Printf("✓ Removed package: %s\n", name)
}

type goDocFunc struct {
	name        string
	params      string
	returnType  string
	multiReturn bool // true if function returns (value, error)
}

func parseGoDocFunctions(doc string) []goDocFunc {
	var functions []goDocFunc
	lines := strings.Split(doc, "\n")

	for _, line := range lines {
		line = strings.TrimSpace(line)
		// Match: func FuncName(params) returnType
		if !strings.HasPrefix(line, "func ") {
			continue
		}
		// Skip methods (have a receiver)
		trimmed := strings.TrimPrefix(line, "func ")
		if strings.HasPrefix(trimmed, "(") {
			continue // method with receiver
		}

		// Extract function name
		parenIdx := strings.Index(trimmed, "(")
		if parenIdx < 0 {
			continue
		}
		funcName := trimmed[:parenIdx]

		// Extract params and return type
		rest := trimmed[parenIdx:]
		// Find matching closing paren
		depth := 0
		closeIdx := -1
		for i, ch := range rest {
			if ch == '(' {
				depth++
			} else if ch == ')' {
				depth--
				if depth == 0 {
					closeIdx = i
					break
				}
			}
		}
		if closeIdx < 0 {
			continue
		}

		paramsRaw := rest[1:closeIdx]
		returnRaw := strings.TrimSpace(rest[closeIdx+1:])

		// Simplify params to Serv types
		params := simplifyGoParams(paramsRaw)
		returnType := simplifyGoType(returnRaw)

		functions = append(functions, goDocFunc{
			name:        funcName,
			params:      params,
			returnType:  returnType,
			multiReturn: isMultiReturn(returnRaw),
		})
	}
	return functions
}

func simplifyGoParams(raw string) string {
	if raw == "" {
		return ""
	}
	parts := strings.Split(raw, ",")
	var result []string
	for _, p := range parts {
		p = strings.TrimSpace(p)
		tokens := strings.Fields(p)
		if len(tokens) >= 2 {
			name := tokens[0]
			goType := strings.Join(tokens[1:], " ")
			srvType := goTypeToServType(goType)
			result = append(result, name+": "+srvType)
		} else if len(tokens) == 1 {
			result = append(result, tokens[0])
		}
	}
	return strings.Join(result, ", ")
}

func simplifyGoType(raw string) string {
	raw = strings.TrimSpace(raw)
	// Remove parentheses from multi-return
	raw = strings.TrimPrefix(raw, "(")
	raw = strings.TrimSuffix(raw, ")")
	if raw == "" {
		return ""
	}
	// For multi-return, just take the first type
	parts := strings.Split(raw, ",")
	return goTypeToServType(strings.TrimSpace(parts[0]))
}

func isMultiReturn(raw string) bool {
	raw = strings.TrimSpace(raw)
	return strings.HasPrefix(raw, "(") && strings.Contains(raw, ",")
}

func goTypeToServType(goType string) string {
	goType = strings.TrimSpace(goType)
	switch goType {
	case "string":
		return "string"
	case "int", "int64", "int32", "uint", "uint64":
		return "int"
	case "float64", "float32":
		return "float"
	case "bool":
		return "bool"
	case "error":
		return "string"
	case "[]byte":
		return "string"
	default:
		if strings.HasPrefix(goType, "[]") {
			return "[]" + goTypeToServType(strings.TrimPrefix(goType, "[]"))
		}
		return "string" // default fallback
	}
}

// resolveImportPath resolves a Serv import path to an absolute file path.
// Supports:
//   - Relative paths: "./models/user.srv", "../stdlib/auth.srv"
//   - Stdlib shorthand: "stdlib/auth.srv" or "stdlib/auth" (resolved from compiler install dir)
//   - Absolute-looking paths with .srv extension get resolved relative to the importer
func resolveImportPath(importerPath, importStr string) string {
	// Strip .srv extension if missing (allow "stdlib/auth" as shorthand for "stdlib/auth.srv")
	if !strings.HasSuffix(importStr, ".srv") && !strings.HasSuffix(importStr, ".srv.d") {
		importStr = importStr + ".srv"
	}

	// If path starts with "stdlib/" — resolve from multiple locations
	if strings.HasPrefix(importStr, "stdlib/") {
		// 1. Try relative to the working directory
		if _, err := os.Stat(importStr); err == nil {
			abs, _ := filepath.Abs(importStr)
			return abs
		}

		// 2. Try from the importing file's directory (walk up to find stdlib/)
		dir := filepath.Dir(importerPath)
		for i := 0; i < 10; i++ {
			candidate := filepath.Join(dir, importStr)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
				candidate = filepath.Join(dir, importStr)
				if _, err := os.Stat(candidate); err == nil {
					return candidate
				}
				break
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}

		// 3. Try relative to the serv binary location (installed stdlib)
		exePath, err := os.Executable()
		if err == nil {
			exeDir := filepath.Dir(exePath)
			candidate := filepath.Join(exeDir, importStr)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
			// Also check one level up from binary (e.g. bin/../stdlib/)
			candidate = filepath.Join(filepath.Dir(exeDir), importStr)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}

		// 4. Try SERV_HOME environment variable
		if servHome := os.Getenv("SERV_HOME"); servHome != "" {
			candidate := filepath.Join(servHome, importStr)
			if _, err := os.Stat(candidate); err == nil {
				return candidate
			}
		}
	}

	// Default: resolve relative to the importing file's directory
	return filepath.Join(filepath.Dir(importerPath), importStr)
}

func parseWithDependencies(filePath string, visited map[string]bool) (*compiler.Program, error) {
	if visited[filePath] {
		// Circular dependency detected — build the cycle path for the error message
		return nil, fmt.Errorf("circular import detected: %s is already being imported (import cycle)", filepath.Base(filePath))
	}
	visited[filePath] = true

	content, err := os.ReadFile(filePath)
	if err != nil {
		return nil, fmt.Errorf("failed to read file %s: %w", filePath, err)
	}

	lexer := compiler.NewLexer(string(content))
	parser := compiler.NewParser(lexer)
	program := parser.ParseProgram()

	if len(parser.Errors()) > 0 {
		diagnostics := compiler.FormatDiagnostics(parser.Errors(), string(content))
		return nil, fmt.Errorf("errors parsing %s:\n%s", filePath, diagnostics)
	}

	var mergedStatements []compiler.Statement

	for _, stmt := range program.Statements {
		// Go package imports are kept as-is for codegen (no file to load)
		// Also auto-load matching .srv.d declaration file if available
		if goPkg, ok := stmt.(*compiler.GoPackageImport); ok {
			mergedStatements = append(mergedStatements, stmt)
			// Auto-load declaration file: declarations/<pkgname>.srv.d
			pkgName := filepath.Base(goPkg.Path)
			declFile := filepath.Join("declarations", pkgName+".srv.d")
			if _, err := os.Stat(declFile); err == nil {
				declProgram, err := parseWithDependencies(declFile, visited)
				if err == nil {
					for _, ds := range declProgram.Statements {
						mergedStatements = append(mergedStatements, ds)
					}
				}
			}
			continue
		}
		// Declare module statements are kept as-is for codegen
		if _, ok := stmt.(*compiler.DeclareModuleStmt); ok {
			mergedStatements = append(mergedStatements, stmt)
			continue
		}
		if imp, ok := stmt.(*compiler.ImportStmt); ok {
			importPath := resolveImportPath(filePath, imp.Path)
			subProgram, err := parseWithDependencies(importPath, visited)
			if err != nil {
				return nil, err
			}

			if len(imp.Names) > 0 {
				// Selective import: only include exported statements matching the requested names
				// Also auto-include methods for any imported struct types
				nameSet := make(map[string]bool)
				for _, n := range imp.Names {
					nameSet[n] = true
				}
				// First pass: collect imported struct names
				structNames := make(map[string]bool)
				for _, subStmt := range subProgram.Statements {
					name := exportedName(subStmt)
					if nameSet[name] {
						inner := subStmt
						if exp, ok := subStmt.(*compiler.ExportStmt); ok {
							inner = exp.Inner
						}
						if _, isStruct := inner.(*compiler.StructDecl); isStruct {
							structNames[name] = true
						}
					}
				}
				// Second pass: include matching names + methods on imported structs
				for _, subStmt := range subProgram.Statements {
					name := exportedName(subStmt)
					inner := subStmt
					if exp, ok := subStmt.(*compiler.ExportStmt); ok {
						inner = exp.Inner
					}
					// Include if name matches directly
					if nameSet[name] {
						mergedStatements = append(mergedStatements, inner)
						continue
					}
					// Auto-include methods for imported struct types
					if method, ok := inner.(*compiler.MethodDecl); ok {
						if structNames[method.TypeName] {
							mergedStatements = append(mergedStatements, inner)
						}
					}
				}
			} else {
				// Wildcard import: include all exported statements (and non-export statements for backward compat)
				for _, subStmt := range subProgram.Statements {
					if exp, ok := subStmt.(*compiler.ExportStmt); ok {
						mergedStatements = append(mergedStatements, exp.Inner)
					} else {
						mergedStatements = append(mergedStatements, subStmt)
					}
				}
			}
		} else {
			mergedStatements = append(mergedStatements, stmt)
		}
	}

	program.Statements = mergedStatements
	return program, nil
}

// exportedName returns the symbol name of an exported statement, or "" if not exported/named.
func exportedName(stmt compiler.Statement) string {
	// Check if it's an ExportStmt wrapper
	if exp, ok := stmt.(*compiler.ExportStmt); ok {
		return statementName(exp.Inner)
	}
	// For backward compatibility, non-exported statements in imported files
	// are still included in wildcard imports
	return statementName(stmt)
}

// statementName extracts the declared name from a statement (for import filtering).
func statementName(stmt compiler.Statement) string {
	switch s := stmt.(type) {
	case *compiler.FnDecl:
		return s.Name
	case *compiler.StructDecl:
		return s.Name
	case *compiler.MethodDecl:
		return s.TypeName + "." + s.Name
	case *compiler.LetStmt:
		return s.Name
	case *compiler.EnumStmt:
		return s.Name
	case *compiler.InterfaceDecl:
		return s.Name
	case *compiler.ExportStmt:
		return statementName(s.Inner)
	default:
		return ""
	}
}
