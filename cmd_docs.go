package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"serv/compiler"
)

func runDocs() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: serv docs generate <file.srv> [-o <output-file>]")
		os.Exit(1)
	}

	subCommand := os.Args[2]
	if subCommand != "generate" {
		fmt.Printf("Unknown docs subcommand: %s. Did you mean 'generate'?\n", subCommand)
		os.Exit(1)
	}

	docsCmd := flag.NewFlagSet("docs generate", flag.ExitOnError)
	outputFile := docsCmd.String("o", "openapi.json", "Output file path")
	
	// Shift args to parse options correctly: serv docs generate [options] <file>
	// Find the file argument and exclude it from parse
	var options []string
	var fileArg string
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "-o" && i+1 < len(os.Args) {
			options = append(options, "-o", os.Args[i+1])
			i++
		} else if strings.HasPrefix(arg, "-") {
			// other flags if any
			options = append(options, arg)
		} else {
			fileArg = arg
		}
	}

	if fileArg == "" {
		fmt.Println("Usage: serv docs generate <file.srv> [-o <output-file>]")
		os.Exit(1)
	}

	if err := docsCmd.Parse(options); err != nil {
		fmt.Printf("Error parsing options: %v\n", err)
		os.Exit(1)
	}

	_, prog, err := parseProject(fileArg)
	if err != nil {
		fmt.Printf("Error parsing project: %v\n", err)
		os.Exit(1)
	}

	jsonStr, err := compiler.GenerateOpenAPI(prog)
	if err != nil {
		fmt.Printf("Error generating OpenAPI: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outputFile, []byte(jsonStr), 0644); err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Successfully generated OpenAPI documentation at %s\n", *outputFile)
}
