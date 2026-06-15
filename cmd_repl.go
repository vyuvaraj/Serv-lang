package main

import (
	"bufio"
	"bytes"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"serv/compiler"
)

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

	// Build directory for REPL sessions
	replDir := filepath.Join(".build", "repl")
	os.MkdirAll(replDir, 0755)

	// Ensure go.mod exists for compilation
	if _, err := ensureBuildGoMod(replDir); err != nil {
		fmt.Printf("Warning: could not setup build module for REPL: %v\n", err)
	}

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
			srvSource.WriteString(d)
			srvSource.WriteString("\n")
		}

		if isDecl {
			srvSource.WriteString(line)
			srvSource.WriteString("\n")
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
	if err := scanner.Err(); err != nil {
		fmt.Printf("REPL read error: %v\n", err)
	}

	// .build/repl acts as a cache, no cleanup needed
}

// compileAndRunREPL compiles a temporary .srv file and runs it briefly to capture output.
func compileAndRunREPL(goPath, srvFile, workDir string, printExpr string) (string, error) {
	// Parse and generate
	program, err := parseWithDependencies(srvFile, make(map[string]int))
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

	// Tidy modules after writing Go code
	if err := runGoModTidy(workDir); err != nil {
		return "", fmt.Errorf("failed to tidy REPL module: %w", err)
	}

	// Remove stale binary to force rebuild
	absWorkDir, _ := filepath.Abs(workDir)
	binPath := filepath.Join(absWorkDir, "repl_bin.exe")
	os.Remove(binPath)

	// Build from the workDir (which has its own go.mod)
	buildCmd := exec.Command(goPath, "build", "-o", binPath, ".")
	buildCmd.Dir = absWorkDir
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
