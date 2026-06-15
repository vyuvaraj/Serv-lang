package main

import (
	"flag"
	"fmt"
	"os"
)

func main() {
	if len(os.Args) < 2 {
		printUsage()
		return
	}

	command := os.Args[1]

	switch command {
	case "build":
		outputBinary := ""
		target := ""
		var buildArgs []string
		rawArgs := os.Args[2:]
		for i := 0; i < len(rawArgs); i++ {
			if rawArgs[i] == "-o" && i+1 < len(rawArgs) {
				outputBinary = rawArgs[i+1]
				i++ // skip the value
			} else if (rawArgs[i] == "--target" || rawArgs[i] == "-target") && i+1 < len(rawArgs) {
				target = rawArgs[i+1]
				i++
			} else {
				buildArgs = append(buildArgs, rawArgs[i])
			}
		}
		if len(buildArgs) < 1 {
			fmt.Println("Usage: serv build <file.srv> [--target <target>] [-o <output>]")
			os.Exit(1)
		}
		if outputBinary == "" {
			if target == "wasm" {
				outputBinary = "service.wasm"
			} else {
				outputBinary = "service.exe"
			}
		}
		buildServ(buildArgs[0], outputBinary, target)

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

	case "install":
		if len(os.Args) < 3 {
			fmt.Println("Usage: serv install <package-name>")
			os.Exit(1)
		}
		installPackage(os.Args[2])

	case "publish":
		if len(os.Args) < 3 {
			fmt.Println("Usage: serv publish <package-dir>")
			os.Exit(1)
		}
		publishPackage(os.Args[2])

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
	fmt.Println("  serv install <package-name>                Install a third-party Serv module")
	fmt.Println("  serv publish <package-dir>                 Publish a Serv module to the registry")
	fmt.Println("  serv dockerize <file.srv>                  Generate a Dockerfile for the Serv service")
}
