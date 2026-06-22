package main

import (
	"flag"
	"fmt"
	"os"
	"strings"
	"serv/compiler"
)

func runGenerate() {
	if len(os.Args) < 3 {
		fmt.Println("Usage: serv generate client <file.srv> [--lang <typescript|python|go>] [-o <output-file>]")
		os.Exit(1)
	}

	subCommand := os.Args[2]
	if subCommand != "client" {
		fmt.Printf("Unknown generate subcommand: %s. Only 'client' is supported.\n", subCommand)
		os.Exit(1)
	}

	generateCmd := flag.NewFlagSet("generate client", flag.ExitOnError)
	lang := generateCmd.String("lang", "typescript", "Target language (typescript, python, go)")
	outputFile := generateCmd.String("o", "", "Output file path (default depends on language)")

	// Parse flags: serv generate client [options] <file>
	var options []string
	var fileArg string
	for i := 3; i < len(os.Args); i++ {
		arg := os.Args[i]
		if arg == "-o" && i+1 < len(os.Args) {
			options = append(options, "-o", os.Args[i+1])
			i++
		} else if (arg == "--lang" || arg == "-lang") && i+1 < len(os.Args) {
			options = append(options, "--lang", os.Args[i+1])
			i++
		} else if strings.HasPrefix(arg, "-") {
			options = append(options, arg)
		} else {
			fileArg = arg
		}
	}

	if fileArg == "" {
		fmt.Println("Usage: serv generate client <file.srv> [--lang <typescript|python|go>] [-o <output-file>]")
		os.Exit(1)
	}

	if err := generateCmd.Parse(options); err != nil {
		fmt.Printf("Error parsing options: %v\n", err)
		os.Exit(1)
	}

	// Set default output file if not specified
	langLower := strings.ToLower(*lang)
	if *outputFile == "" {
		switch langLower {
		case "typescript", "ts":
			*outputFile = "client.ts"
		case "python", "py":
			*outputFile = "client.py"
		case "go":
			*outputFile = "client.go"
		default:
			*outputFile = "client.ts"
		}
	}

	_, prog, err := parseProject(fileArg)
	if err != nil {
		fmt.Printf("Error parsing project: %v\n", err)
		os.Exit(1)
	}

	g := compiler.NewClientGenerator(prog)

	var generatedCode string
	switch langLower {
	case "typescript", "ts":
		generatedCode, err = g.GenerateTypeScript()
	case "python", "py":
		generatedCode, err = g.GeneratePython()
	case "go":
		generatedCode, err = g.GenerateGo()
	default:
		fmt.Printf("Unsupported language: %s. Choose from: typescript, python, go.\n", *lang)
		os.Exit(1)
	}

	if err != nil {
		fmt.Printf("Error generating client SDK: %v\n", err)
		os.Exit(1)
	}

	if err := os.WriteFile(*outputFile, []byte(generatedCode), 0644); err != nil {
		fmt.Printf("Error writing output file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("✓ Successfully generated %s client SDK at %s\n", *lang, *outputFile)
}
