package main

import (
	"flag"
	"fmt"
	"os"
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
		outputBinary := ""
		target := ""
		goos := ""
		goarch := ""
		tags := ""
		offline := false
		var buildArgs []string
		rawArgs := os.Args[2:]
		for i := 0; i < len(rawArgs); i++ {
			if rawArgs[i] == "-o" && i+1 < len(rawArgs) {
				outputBinary = rawArgs[i+1]
				i++ // skip the value
			} else if (rawArgs[i] == "--target" || rawArgs[i] == "-target") && i+1 < len(rawArgs) {
				target = rawArgs[i+1]
				i++
			} else if (rawArgs[i] == "--os" || rawArgs[i] == "-os") && i+1 < len(rawArgs) {
				goos = rawArgs[i+1]
				i++
			} else if (rawArgs[i] == "--arch" || rawArgs[i] == "-arch") && i+1 < len(rawArgs) {
				goarch = rawArgs[i+1]
				i++
			} else if (rawArgs[i] == "--tags" || rawArgs[i] == "-tags") && i+1 < len(rawArgs) {
				tags = rawArgs[i+1]
				i++
			} else if rawArgs[i] == "--offline" {
				offline = true
			} else {
				buildArgs = append(buildArgs, rawArgs[i])
			}
		}

		if offline {
			compiler.SkipInfraReachability = true
		}
		if len(buildArgs) < 1 {
			buildArgs = []string{"."}
		}
		if outputBinary == "" {
			if target == "wasm" || target == "wasm-edge" {
				outputBinary = "service.wasm"
			} else {
				outputBinary = "service.exe"
			}
		}
		buildServ(buildArgs[0], outputBinary, target, goos, goarch, tags)

	case "run":
		runCmd := flag.NewFlagSet("run", flag.ExitOnError)
		watchFlag := runCmd.Bool("watch", false, "Watch files and restart")
		hotFlag := runCmd.Bool("hot", false, "Watch files and hot-reload without restart (zero downtime)")
		profileFlag := runCmd.Bool("profile", false, "Enable CPU and memory profiling")
		envFlag := runCmd.String("env", "", "Environment profile (e.g., staging, production)")
		offlineFlag := runCmd.Bool("offline", false, "Skip compile-time infrastructure reachability checks")
		if err := runCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		if *offlineFlag {
			compiler.SkipInfraReachability = true
		}
		args := runCmd.Args()
		if len(args) < 1 {
			args = []string{"."}
		}

		if *hotFlag {
			runServHot(args[0], *envFlag)
		} else if *watchFlag {
			runServWatch(args[0], *envFlag)
		} else {
			runServ(args[0], args[1:], *profileFlag, *envFlag)
		}

	case "dockerize":
		dockerCmd := flag.NewFlagSet("dockerize", flag.ExitOnError)
		if err := dockerCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := dockerCmd.Args()
		if len(args) < 1 {
			args = []string{"."}
		}
		dockerizeServ(args[0])

	case "deploy":
		deployCmd := flag.NewFlagSet("deploy", flag.ExitOnError)
		targetFlag := deployCmd.String("target", "", "Deploy target: fly, railway, render, k8s, docker")
		if err := deployCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		if *targetFlag == "" {
			fmt.Println("Usage: serv deploy --target <fly|railway|render|k8s|docker> [file.srv]")
			os.Exit(1)
		}
		args := deployCmd.Args()
		if len(args) < 1 {
			args = []string{"."}
		}
		deployServ(args[0], *targetFlag)

	case "test":
		testCmd := flag.NewFlagSet("test", flag.ExitOnError)
		coverFlag := testCmd.Bool("cover", false, "Report test coverage")
		filterFlag := testCmd.String("filter", "", "Filter tests by name")
		integrationFlag := testCmd.Bool("integration", false, "Run with live infrastructure services")
		watchFlag := testCmd.Bool("watch", false, "Watch for changes and re-run tests")
		if err := testCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := testCmd.Args()
		if len(args) < 1 {
			args = []string{"."}
		}
		if *watchFlag {
			runTestsWatch(args[0], *coverFlag, *filterFlag, *integrationFlag)
		} else if *integrationFlag {
			if !runIntegrationTests(args[0], *coverFlag, *filterFlag) {
				os.Exit(1)
			}
		} else {
			if !runTests(args[0], *coverFlag, *filterFlag) {
				os.Exit(1)
			}
		}

	case "lint":
		lintCmd := flag.NewFlagSet("lint", flag.ExitOnError)
		if err := lintCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := lintCmd.Args()
		if len(args) < 1 {
			args = []string{"."}
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
		initCmd := flag.NewFlagSet("init", flag.ExitOnError)
		fullStackFlag := initCmd.Bool("full-stack", false, "Generate docker-compose.yml with all Servverse services")
		if err := initCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		if *fullStackFlag {
			runInitFullStack()
		} else {
			initProject()
		}

	case "new":
		newCmd := flag.NewFlagSet("new", flag.ExitOnError)
		templateFlag := newCmd.String("template", "api", "Template to scaffold: api, worker, event-processor, full-stack")
		if err := newCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := newCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv new <project-name> [--template <api|worker|event-processor|full-stack>]")
			os.Exit(1)
		}
		createNewProject(args[0], *templateFlag)

	case "create":
		createCmd := flag.NewFlagSet("create", flag.ExitOnError)
		fixFlag := createCmd.Bool("fix", false, "Repairs failures automatically using serv test results")
		if err := createCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := createCmd.Args()
		if len(args) < 1 {
			fmt.Println("Usage: serv create [--fix] \"<prompt describing your service>\"")
			os.Exit(1)
		}
		runAIScaffold(args[0], *fixFlag)

	case "debug":
		targetFile := "."
		if len(os.Args) >= 3 {
			targetFile = os.Args[2]
		}
		debugServ(targetFile)

	case "audit":
		runAudit()

	case "doctor":
		doctorCmd := flag.NewFlagSet("doctor", flag.ExitOnError)
		integrationFlag := doctorCmd.Bool("integration", false, "Run full pipeline docker integration checks")
		if err := doctorCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		runDoctor(*integrationFlag)

	case "upgrade":
		runUpgrade()


	case "cache":
		if len(os.Args) >= 3 && os.Args[2] == "inspect" {
			runCacheInspect()
		} else {
			fmt.Println("Usage: serv cache inspect [--host <host>]")
		}

	case "cron":
		if len(os.Args) >= 3 && os.Args[2] == "list" {
			runCronList()
		} else {
			fmt.Println("Usage: serv cron list [--host <host>]")
		}

	case "status":
		runStatus()

	case "monitor":
		target := "8080"
		if len(os.Args) >= 3 {
			target = os.Args[2]
		}
		runMonitor(target)

	case "docs", "doc":
		runDocs()

	case "generate":
		// DX.21: support --from-openapi alias for 'serv generate routes'
		for _, arg := range os.Args[2:] {
			if arg == "--from-openapi" {
				// Rewrite args: serv generate --from-openapi <spec> → serv generate routes <spec>
				newArgs := []string{os.Args[0], "generate", "routes"}
				for _, a := range os.Args[2:] {
					if a != "--from-openapi" {
						newArgs = append(newArgs, a)
					}
				}
				os.Args = newArgs
				break
			}
		}
		runGenerate()

	case "bench":
		runBench()

	case "observability", "obs":
		runObservabilityCmd()

	case "tunnel":
		runTunnelCmd()

	case "config":
		runConfigCmd()

	case "trace":
		runTraceCmd()

	case "playground":
		runPlayground()

	case "queue":
		subcmd := ""
		if len(os.Args) >= 3 {
			subcmd = os.Args[2]
		}
		switch subcmd {
		case "tail":
			runQueueTail()
		case "list":
			runQueueList()
		case "dlq":
			runQueueDLQ()
		default:
			fmt.Println("Usage:")
			fmt.Println("  serv queue tail <topic> [--host <url>] [--limit <n>]")
			fmt.Println("  serv queue list [--host <url>]")
			fmt.Println("  serv queue dlq inspect <topic> [--host <url>] [--replay]")
		}

	case "mesh":
		subcmd := ""
		if len(os.Args) >= 3 {
			subcmd = os.Args[2]
		}
		switch subcmd {
		case "inspect":
			runMeshInspect()
		case "routes":
			runMeshRoutes()
		default:
			fmt.Println("Usage:")
			fmt.Println("  serv mesh inspect [--host <url>] [--service <name>]")
			fmt.Println("  serv mesh routes [--host <url>]")
		}

	case "dev":
		runDevCmd()

	case "migrate":
		migrateCmd := flag.NewFlagSet("migrate", flag.ExitOnError)
		dbFlag := migrateCmd.String("db", "", "Database connection string (e.g. sqlite://mydb.db). Falls back to $DATABASE_URL")
		rollbackFlag := migrateCmd.Bool("rollback", false, "Roll back structural schema changes (e.g., dropping columns or tables)")
		if err := migrateCmd.Parse(os.Args[2:]); err != nil {
			fmt.Printf("Error parsing arguments: %v\n", err)
			os.Exit(1)
		}
		args := migrateCmd.Args()
		target := "."
		if len(args) >= 1 {
			target = args[0]
		}
		runMigrate(target, *dbFlag, *rollbackFlag)

	case "lsp-action":
		runLspActionCmd(os.Args[2:])

	default:
		printUsage()
	}
}

func printUsage() {
	fmt.Println("Serv: A Programming Language for Background Services")
	fmt.Println("Usage:")
	fmt.Println("  serv init [name]                           Create a new Serv project")
	fmt.Println("  serv new <name> [--template <template>]    Create a new Serv project from a template (api, worker, event-processor, full-stack)")
	fmt.Println("  serv create \"<prompt>\"                     AI-scaffold a new Serv file from a natural language description")
	fmt.Println("  serv docs generate <file.srv> [-o <out>]   Autogenerate OpenAPI 3.1 specifications from routes")
	fmt.Println("  serv generate client <file.srv> [--lang <lang>] [-o <out>] Autogenerate client SDKs (typescript/python/go) from routes")

	fmt.Println("  serv build <file.srv> [--target <target>] [-o <output>] Compile Serv code to target (native/wasm)")
	fmt.Println("  serv run <file.srv> [--watch]              Compile and run Serv code immediately (with optional hot reload)")
	fmt.Println("  serv test [--cover] [--integration] <file.srv> Run tests (--integration starts live infra)")
	fmt.Println("  serv lint <file.srv>                       Validate syntax and check for errors")
	fmt.Println("  serv fmt <file.srv>                        Format a Serv file")
	fmt.Println("  serv repl                                  Interactive shell for quick experiments")
	fmt.Println("  serv add <go-package>                      Generate .srv.d declaration for a Go package")
	fmt.Println("  serv install <package-name>                Install a third-party Serv module")
	fmt.Println("  serv publish <package-dir>                 Publish a Serv module to the registry")
	fmt.Println("  serv debug <file.srv>                       Debug a Serv file (requires dlv: go install github.com/go-delve/delve/cmd/dlv@latest)")
	fmt.Println("  serv dockerize <file.srv>                  Generate a Dockerfile for the Serv service")
	fmt.Println("  serv deploy --target <target> [file.srv]   Generate deploy config (fly, railway, render, k8s, docker)")
	fmt.Println("  serv monitor [port-or-url]                 Terminal htop-style live dashboard for a running service")
	fmt.Println("  serv tunnel <port> [options]               Expose a local service via the ServTunnel relay server")
	fmt.Println("  serv dev [file.srv] [--dashboard]         Start full dev environment (with optional terminal TUI)")
	fmt.Println("  serv config init                          Create starter .serv/config.yaml")
	fmt.Println("  serv config propagate [--dry-run]         Push configuration settings to active services")
	fmt.Println("  serv audit                                 Audit Go/Serv dependencies for vulnerabilities")
	fmt.Println("  serv doctor                                Run compatibility and health checks on all Servverse services")
	fmt.Println("  serv status                                Print live health, uptime, and latency stats for all services")
	fmt.Println("  serv migrate [file.srv] [--db <conn>]         Apply declarative `table` schema migrations to the database")
	fmt.Println("  serv queue tail <topic> [--host <url>] [--limit <n>]   Tail recent messages on a ServQueue topic")
	fmt.Println("  serv queue list [--host <url>]                         List all ServQueue topics and consumer counts")
	fmt.Println("  serv mesh inspect [--host <url>] [--service <name>]   Inspect ServMesh registry and instance list")
	fmt.Println("  serv mesh routes [--host <url>]                        Show active routing rules and circuit-breaker state")
	fmt.Println("  serv init --full-stack                                 Generate docker-compose.yml with all Servverse services")
	fmt.Println("  serv bench <file.srv> [--rps <n>] [--duration <s>]       Run built-in load test against declared routes")
	fmt.Println("  serv generate --from-openapi <spec.yaml> [-o <out.srv>]  Generate .srv route stubs from OpenAPI spec")
	fmt.Println("  serv observability init                                   Create .serv/observability.yaml template")
	fmt.Println("  serv observability apply [--dry-run]                     Provision alert rules, SLOs, and dashboards")
	fmt.Println("  serv playground [--port <p>]                             Start the hosted browser-based editor")
	fmt.Println("  serv lsp-action --file <file> --line <line> [--type <type>] Resolve LSP code action recommendation")
}
