package main

import (
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
		if err := testCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := testCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv test <file.srv>")
			os.Exit(1)
		}
		runTests(args[0])

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
			os.Exit(1)
		}
		addPackage(args[0])

	case "fmt":
		fmtCmd := flag.NewFlagSet("fmt", flag.ExitOnError)
		if err := fmtCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := fmtCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv fmt <file.srv>")
			os.Exit(1)
		}
		formatFile(args[0])

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Serv: A Programming Language for Background Services")
	fmt.Println("Usage:")
	fmt.Println("  serv build <file.srv> [-o <output_binary>]  Compile Serv code to native binary")
	fmt.Println("  serv run <file.srv> [--watch]              Compile and run Serv code immediately (with optional hot reload)")
	fmt.Println("  serv test <file.srv>                       Run tests defined in a Serv file")
	fmt.Println("  serv lint <file.srv>                       Validate syntax of a Serv file")
	fmt.Println("  serv fmt <file.srv>                        Format a Serv file")
	fmt.Println("  serv add <go-package>                      Generate .srv.d declaration for a Go package")
	fmt.Println("  serv dockerize <file.srv>                  Generate a Dockerfile for the Serv service")
}

func runLint(srvFile string) {
	absPath, err := filepath.Abs(srvFile)
	if err != nil {
		fmt.Printf("Error: %v\n", err)
		os.Exit(1)
	}

	_, err = parseWithDependencies(absPath, make(map[string]bool))
	if err != nil {
		fmt.Println(err)
		os.Exit(1)
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

	goPath, err := resolveGoPath()
	if err != nil {
		return "", fmt.Errorf("cannot find Go compiler: %w", err)
	}
	cmd := exec.Command(goPath, "build", "-o", filepath.Join(filepath.Dir(absPath), outputBinary), genGoFile)
	cmd.Dir = filepath.Dir(absPath)
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

func runTests(srvFile string) {
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

	// Clean stale Go files from previous builds to prevent conflicts
	for _, name := range []string{"main.go", "service.go", "serv_test.go"} {
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
	cmd := exec.Command(goPath, "test", "-v", "./...")
	cmd.Dir = buildDir
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		fmt.Printf("Tests failed: %v\n", err)
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

// formatFile formats a .srv file with consistent indentation and spacing.
func formatFile(srvFile string) {
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

	for _, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Skip multiple consecutive empty lines
		if trimmed == "" {
			if !prevEmpty {
				result = append(result, "")
				prevEmpty = true
			}
			continue
		}
		prevEmpty = false

		// Decrease indent for closing braces
		if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") {
			indentLevel--
			if indentLevel < 0 {
				indentLevel = 0
			}
		}

		// Apply indentation
		formatted := strings.Repeat(indent, indentLevel) + trimmed
		result = append(result, formatted)

		// Increase indent for opening braces
		openBraces := strings.Count(trimmed, "{") + strings.Count(trimmed, "[")
		closeBraces := strings.Count(trimmed, "}") + strings.Count(trimmed, "]")
		indentLevel += openBraces - closeBraces
		// Re-adjust if we already decremented for leading close brace
		if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") {
			indentLevel += 1 // we already decremented above
			indentLevel += openBraces - closeBraces
			indentLevel -= 1
		}
		if indentLevel < 0 {
			indentLevel = 0
		}
	}

	// Ensure file ends with newline
	output := strings.Join(result, "\n")
	if !strings.HasSuffix(output, "\n") {
		output += "\n"
	}

	if err := os.WriteFile(srvFile, []byte(output), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Formatted: %s\n", srvFile)
}

// addPackage generates a .srv.d declaration file for a Go package.
func addPackage(pkgPath string) {
	// Run go doc to get package documentation
	goPath, err := resolveGoPath()
	if err != nil {
		fmt.Printf("Cannot find Go compiler: %v\n", err)
		os.Exit(1)
	}

	cmd := exec.Command(goPath, "doc", "-all", pkgPath)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		fmt.Printf("Failed to get package info for %s: %v\n%s\n", pkgPath, err, stderr.String())
		fmt.Println("Make sure the package is available: go get", pkgPath)
		os.Exit(1)
	}

	// Parse the go doc output to extract exported functions
	output := stdout.String()
	functions := parseGoDocFunctions(output)

	if len(functions) == 0 {
		fmt.Printf("No exported functions found in %s\n", pkgPath)
		os.Exit(1)
	}

	// Generate .srv.d file
	pkgName := filepath.Base(pkgPath)
	declDir := "declarations"
	os.MkdirAll(declDir, 0755)

	var content strings.Builder
	content.WriteString(fmt.Sprintf("// Auto-generated declaration for %s\n", pkgPath))
	content.WriteString(fmt.Sprintf("// Generated by: serv add %s\n\n", pkgPath))
	content.WriteString(fmt.Sprintf("declare module \"%s\" {\n", pkgPath))
	for _, fn := range functions {
		content.WriteString(fmt.Sprintf("    fn %s(%s)", fn.name, fn.params))
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

	fmt.Printf("Generated declaration: %s (%d functions)\n", declFile, len(functions))
	fmt.Printf("\nUsage in your .srv file:\n")
	fmt.Printf("  import \"%s\"\n", declFile)
	fmt.Printf("  import %s from \"%s\"\n", pkgName, pkgPath)
}

type goDocFunc struct {
	name       string
	params     string
	returnType string
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
			name:       funcName,
			params:     params,
			returnType: returnType,
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

func parseWithDependencies(filePath string, visited map[string]bool) (*compiler.Program, error) {
	if visited[filePath] {
		return &compiler.Program{}, nil // Prevent circular imports
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
		return nil, fmt.Errorf("errors parsing %s:\n%s", filePath, strings.Join(parser.Errors(), "\n"))
	}

	var mergedStatements []compiler.Statement

	for _, stmt := range program.Statements {
		// Go package imports are kept as-is for codegen (no file to load)
		if _, ok := stmt.(*compiler.GoPackageImport); ok {
			mergedStatements = append(mergedStatements, stmt)
			continue
		}
		// Declare module statements are kept as-is for codegen
		if _, ok := stmt.(*compiler.DeclareModuleStmt); ok {
			mergedStatements = append(mergedStatements, stmt)
			continue
		}
		if imp, ok := stmt.(*compiler.ImportStmt); ok {
			importPath := filepath.Join(filepath.Dir(filePath), imp.Path)
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
