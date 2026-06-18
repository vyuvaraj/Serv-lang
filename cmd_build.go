package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"serv/compiler"
)

func buildServ(srvFile, outputBinary, target string) string {
	absPath, err := buildServNoExit(srvFile, outputBinary, target)
	if err != nil {
		fmt.Printf("Build failed: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Build successful! Binary: %s\n", absPath)
	return absPath
}

func buildServNoExit(srvFile, outputBinary, target string) (string, error) {
	absPath, program, err := parseProject(srvFile)
	if err != nil {
		return "", err
	}

	// Validate target support for WASM
	if target == "wasm" {
		for _, stmt := range program.Statements {
			switch stmt.(type) {
			case *compiler.ServerStmt, *compiler.RouteStmt, *compiler.DatabaseStmt, *compiler.BrokerStmt,
				*compiler.EveryStmt, *compiler.CronStmt, *compiler.SubscribeStmt, *compiler.WsStmt:
				return "", fmt.Errorf("wasm target does not support service architecture statements: server, route, database, broker, every, cron, subscribe, ws")
			}
		}
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
	if target == "wasm" {
		goCode += "\nfunc main() {}\n"
	} else {
		goCode += "\n" + codegen.GenerateMainFunc()
	}

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
	goModChanged, err := ensureBuildGoMod(buildDir)
	if err != nil {
		return "", fmt.Errorf("failed to setup build module: %w", err)
	}
	// Only run go mod tidy when go.mod actually changed (avoids ~1-2s overhead per build)
	if goModChanged {
		if err := runGoModTidy(buildDir); err != nil {
			return "", fmt.Errorf("failed to tidy build module: %w", err)
		}
	}

	goPath, err := resolveGoPath()
	if err != nil {
		return "", fmt.Errorf("cannot find Go compiler: %w", err)
	}

	targetOutPath := filepath.Join(filepath.Dir(absPath), outputBinary)
	relOutPath, err := filepath.Rel(buildDir, targetOutPath)
	if err != nil {
		relOutPath = targetOutPath
	}

	cmd := exec.Command(goPath, "build", "-o", relOutPath, ".")
	cmd.Dir = buildDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	if target == "wasm" {
		cmd.Env = append(cmd.Env, "GOOS=wasip1", "GOARCH=wasm")
	}
	var stderr bytes.Buffer
	cmd.Stderr = &stderr

	if err := cmd.Run(); err != nil {
		return "", fmt.Errorf("%v: %s", err, stderr.String())
	}

	return filepath.Join(filepath.Dir(absPath), outputBinary), nil
}

func runServ(srvFile string, extraArgs []string, profile bool, env string) {
	binPath, err := buildServNoExit(srvFile, "temp_service.exe", "")
	if err != nil {
		fmt.Printf("Build failed: %v\n", err)
		os.Exit(1)
	}
	defer os.Remove(binPath)

	fmt.Printf("Running native service: %s...\n", binPath)
	cmd := exec.Command(binPath, extraArgs...)
	cmd.Stdin = os.Stdin
	stdoutPipe, err := cmd.StdoutPipe()
	if err != nil {
		fmt.Printf("Failed to create stdout pipe: %v\n", err)
		os.Exit(1)
	}
	
	stderrPipe, err := cmd.StderrPipe()
	if err != nil {
		fmt.Printf("Failed to create stderr pipe: %v\n", err)
		os.Exit(1)
	}

	cmd.Env = os.Environ()
	if profile {
		cmd.Env = append(cmd.Env, "SERV_PROFILE=true")
	}
	if env != "" {
		cmd.Env = append(cmd.Env, "SERV_ENV="+env)
	}

	if err := cmd.Start(); err != nil {
		fmt.Printf("Failed to start service: %v\n", err)
		os.Exit(1)
	}

	rewriter := NewStackTraceRewriter(srvFile)
	var wg sync.WaitGroup
	wg.Add(2)
	go func() {
		rewriter.Rewrite(stdoutPipe, os.Stdout)
		wg.Done()
	}()
	go func() {
		rewriter.Rewrite(stderrPipe, os.Stderr)
		wg.Done()
	}()

	err = cmd.Wait()
	wg.Wait()

	if err != nil {
		fmt.Printf("Service exited with error: %v\n", err)
		os.Exit(1)
	}
}

func runServWatch(srvFile string, env string) {
	fmt.Printf("Starting Serv in Watch Mode: %s...\n", srvFile)

	var cmd *exec.Cmd

	restart := func() {
		if cmd != nil && cmd.Process != nil {
			fmt.Println("[WATCH] File change detected. Restarting service...")
			cmd.Process.Kill()
			cmd.Wait()
		}

		binPath, err := buildServNoExit(srvFile, "watch_service.exe", "")
		if err != nil {
			fmt.Printf("[WATCH] Rebuild failed:\n%v\n", err)
			return
		}

		fmt.Printf("[WATCH] Starting service: %s\n", binPath)
		cmd = exec.Command(binPath)
		stdoutPipe, err := cmd.StdoutPipe()
		if err != nil {
			fmt.Printf("[WATCH] Failed to create stdout pipe: %v\n", err)
			return
		}
		
		stderrPipe, err := cmd.StderrPipe()
		if err != nil {
			fmt.Printf("[WATCH] Failed to create stderr pipe: %v\n", err)
			return
		}

		cmd.Env = os.Environ()
		if env != "" {
			cmd.Env = append(cmd.Env, "SERV_ENV="+env)
		}

		if err := cmd.Start(); err != nil {
			fmt.Printf("[WATCH] Failed to start service: %v\n", err)
			return
		}

		rewriter := NewStackTraceRewriter(srvFile)
		go rewriter.Rewrite(stdoutPipe, os.Stdout)
		go rewriter.Rewrite(stderrPipe, os.Stderr)
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
// Returns true if the go.mod was created or changed (indicating go mod tidy is needed).
func ensureBuildGoMod(buildDir string) (bool, error) {
	goModPath := filepath.Join(buildDir, "go.mod")

	// Find the Serv installation root (where runtime/ and go.mod live)
	servRoot := findServRoot()
	if servRoot == "" {
		return false, fmt.Errorf("cannot find Serv installation (runtime/ directory). Set SERV_HOME or ensure serv.exe is next to the runtime/ folder")
	}

	// Read the Serv project's go.mod to get the Go version and dependencies
	servGoMod := filepath.Join(servRoot, "go.mod")
	servGoModContent, err := os.ReadFile(servGoMod)
	if err != nil {
		return false, fmt.Errorf("cannot read %s: %w", servGoMod, err)
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

	// Check if go.mod content has changed (skip go mod tidy if identical)
	existingContent, readErr := os.ReadFile(goModPath)
	if readErr == nil && string(existingContent) == goMod {
		// go.mod unchanged — also check go.sum exists
		if _, err := os.Stat(filepath.Join(buildDir, "go.sum")); err == nil {
			return false, nil
		}
	}

	if err := os.WriteFile(goModPath, []byte(goMod), 0644); err != nil {
		return false, err
	}

	// Copy go.sum from Serv root (needed for transitive dependencies)
	servGoSum := filepath.Join(servRoot, "go.sum")
	if sumContent, err := os.ReadFile(servGoSum); err == nil {
		os.WriteFile(filepath.Join(buildDir, "go.sum"), sumContent, 0644)
	}

	return true, nil
}

// runGoModTidy runs go mod tidy inside the build directory to resolve transitive dependencies after go files are written.
func runGoModTidy(buildDir string) error {
	goPath, err := resolveGoPath()
	if err != nil {
		return err
	}
	cmd := exec.Command(goPath, "mod", "tidy")
	cmd.Dir = buildDir
	cmd.Env = append(os.Environ(), "GOWORK=off")
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("go mod tidy failed: %v, stderr: %s", err, stderr.String())
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

// resolveImportPath resolves a Serv import path to an absolute file path.
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

	// 5. Package resolution: check in local packages/ folder
	if !strings.HasPrefix(importStr, "./") && !strings.HasPrefix(importStr, "../") && !strings.HasPrefix(importStr, "/") && !strings.HasPrefix(importStr, "stdlib/") {
		basePkg := strings.TrimSuffix(importStr, ".srv")
		basePkg = strings.TrimSuffix(basePkg, ".srv.d")

		candidates := []string{
			filepath.Join("packages", importStr),
			filepath.Join("packages", basePkg, "index.srv"),
			filepath.Join("packages", basePkg, "main.srv"),
		}

		for _, c := range candidates {
			if _, err := os.Stat(c); err == nil {
				abs, _ := filepath.Abs(c)
				return abs
			}
		}
	}

	// Default: resolve relative to the importing file's directory
	return filepath.Join(filepath.Dir(importerPath), importStr)
}

func parseWithDependencies(filePath string, visited map[string]int) (*compiler.Program, error) {
	if visited[filePath] == 1 {
		// Circular dependency detected — build the cycle path for the error message
		return nil, fmt.Errorf("circular import detected: %s is already being imported (import cycle)", filepath.Base(filePath))
	}
	if visited[filePath] == 2 {
		return &compiler.Program{}, nil
	}
	visited[filePath] = 1
	defer func() { visited[filePath] = 2 }()

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
				// Selective import validation
				subExports := make(map[string]bool)  // name -> isExported
				subDefined := make(map[string]bool)  // name -> exists
				structNames := make(map[string]bool) // imported struct names

				// Identify defined and exported symbols in the subProgram
				for _, subStmt := range subProgram.Statements {
					name := statementName(subStmt)
					if name == "" {
						continue
					}
					subDefined[name] = true
					if _, ok := subStmt.(*compiler.ExportStmt); ok {
						subExports[name] = true
					}
				}

				// Validate each imported name
				for _, n := range imp.Names {
					if !subDefined[n] {
						return nil, fmt.Errorf("symbol '%s' is not defined in '%s'", n, imp.Path)
					}
					if !subExports[n] {
						return nil, fmt.Errorf("cannot import non-exported symbol '%s' from '%s'", n, imp.Path)
					}
				}

				// Collect imported struct names to auto-include their methods
				for _, subStmt := range subProgram.Statements {
					name := statementName(subStmt)
					if name == "" {
						continue
					}
					inner := subStmt
					if exp, ok := subStmt.(*compiler.ExportStmt); ok {
						inner = exp.Inner
					}

					// If this name is explicitly imported and is a Struct
					var isStruct bool
					for _, importedName := range imp.Names {
						if importedName == name {
							if _, ok := inner.(*compiler.StructDecl); ok {
								isStruct = true
							}
							break
						}
					}
					if isStruct {
						structNames[name] = true
					}
				}

				// Merge imported statements
				for _, subStmt := range subProgram.Statements {
					name := statementName(subStmt)

					// Non-named statements are always included (they might be transitive routes/background workers/etc.)
					if name == "" {
						mergedStatements = append(mergedStatements, subStmt)
						continue
					}

					inner := subStmt
					isExported := false
					if exp, ok := subStmt.(*compiler.ExportStmt); ok {
						inner = exp.Inner
						isExported = true
					}

					// Include if name is in the import list
					isImported := false
					for _, importedName := range imp.Names {
						if importedName == name {
							isImported = true
							break
						}
					}
					if isImported {
						mergedStatements = append(mergedStatements, inner)
						continue
					}

					// Auto-include exported methods for imported struct types
					if method, ok := inner.(*compiler.MethodDecl); ok {
						if structNames[method.TypeName] && isExported {
							mergedStatements = append(mergedStatements, inner)
						}
					}
				}
			} else {
				// Wildcard import: include all non-named statements + explicitly exported named statements
				for _, subStmt := range subProgram.Statements {
					name := statementName(subStmt)
					if name == "" {
						mergedStatements = append(mergedStatements, subStmt)
					} else {
						if exp, ok := subStmt.(*compiler.ExportStmt); ok {
							mergedStatements = append(mergedStatements, exp.Inner)
						}
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

func copyFileIfExists(src, dst string) {
	data, err := os.ReadFile(src)
	if err == nil {
		os.WriteFile(dst, data, 0644)
	}
}

func parseProject(srvFile string) (string, *compiler.Program, error) {
	fi, err := os.Stat(srvFile)
	if err != nil {
		return "", nil, err
	}

	var absPath string
	var program *compiler.Program
	visited := make(map[string]int)

	if fi.IsDir() {
		absDir, err := filepath.Abs(srvFile)
		if err != nil {
			return "", nil, err
		}

		tomlPath := filepath.Join(absDir, "serv.toml")
		if _, err := os.Stat(tomlPath); err == nil {
			manifest, err := compiler.ParseManifest(tomlPath)
			if err != nil {
				return "", nil, fmt.Errorf("failed to parse manifest: %w", err)
			}
			entryPath := filepath.Join(absDir, manifest.Entry)
			if _, err := os.Stat(entryPath); err != nil {
				return "", nil, fmt.Errorf("entry point '%s' not found", manifest.Entry)
			}
			absPath = entryPath
			program, err = parseWithDependencies(absPath, visited)
			if err != nil {
				return "", nil, err
			}
		} else {
			files, err := os.ReadDir(absDir)
			if err != nil {
				return "", nil, err
			}
			var srvFiles []string
			for _, file := range files {
				if !file.IsDir() && strings.HasSuffix(file.Name(), ".srv") {
					srvFiles = append(srvFiles, filepath.Join(absDir, file.Name()))
				}
			}
			if len(srvFiles) == 0 {
				return "", nil, fmt.Errorf("no .srv files found in directory %s", srvFile)
			}

			absPath = srvFiles[0]
			var mergedProgram *compiler.Program
			for _, file := range srvFiles {
				prog, err := parseWithDependencies(file, visited)
				if err != nil {
					return "", nil, err
				}
				if mergedProgram == nil {
					mergedProgram = prog
				} else {
					mergedProgram.Statements = append(mergedProgram.Statements, prog.Statements...)
				}
			}
			program = mergedProgram
		}
	} else {
		var err error
		absPath, err = filepath.Abs(srvFile)
		if err != nil {
			return "", nil, err
		}
		program, err = parseWithDependencies(absPath, visited)
		if err != nil {
			return "", nil, err
		}
	}
	return absPath, program, nil
}
