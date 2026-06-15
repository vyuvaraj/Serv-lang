package main

import (
	"fmt"
	"os"

	"serv/compiler"
)

func runLint(srvFile string) {
	absPath, program, err := parseProject(srvFile)
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
