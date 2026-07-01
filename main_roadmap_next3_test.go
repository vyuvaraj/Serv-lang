package main

import (
	"encoding/json"
	"os"
	"strings"
	"testing"
	"serv/compiler"
)

func TestApiVersioningRoutePrefixing(t *testing.T) {
	src := `
	version "v1" {
		route "GET" "/users" (req) {
			return "ok"
		}
		version "v2" {
			route "POST" "/items" (req) {
				return "ok"
			}
		}
	}
	route "GET" "/healthz" (req) {
		return "ok"
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	// Verify route paths in AST statements
	var v1UsersFound, v2ItemsFound, healthzFound bool
	for _, stmt := range prog.Statements {
		if vBlock, ok := stmt.(*compiler.VersionBlockStmt); ok {
			for _, sub := range vBlock.Statements {
				if r, ok := sub.(*compiler.RouteStmt); ok {
					if r.Method == "GET" && r.Path == "/v1/users" {
						v1UsersFound = true
					}
				} else if innerBlock, ok := sub.(*compiler.VersionBlockStmt); ok {
					for _, innerSub := range innerBlock.Statements {
						if r, ok := innerSub.(*compiler.RouteStmt); ok {
							if r.Method == "POST" && r.Path == "/v1/v2/items" {
								v2ItemsFound = true
							}
						}
					}
				}
			}
		} else if r, ok := stmt.(*compiler.RouteStmt); ok {
			if r.Method == "GET" && r.Path == "/healthz" {
				healthzFound = true
			}
		}
	}

	if !v1UsersFound {
		t.Errorf("expected route /v1/users to be generated")
	}
	if !v2ItemsFound {
		t.Errorf("expected nested route /v1/v2/items to be generated")
	}
	if !healthzFound {
		t.Errorf("expected top-level route /healthz to be generated")
	}
}

func TestResilientFunctionCodegen(t *testing.T) {
	src := `
	resilient fn callPayment(amount: int) retries 3 timeout 5s circuit_breaker {
		return amount + 10
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	codegen := compiler.NewCodegen(prog)
	generated, err := codegen.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(generated, `runtime.ResilientCall("callPayment", _wrapper, 3, "5s", true)`) {
		t.Errorf("expected ResilientCall wrapper in generated code, got: %s", generated)
	}
}

func TestResiliencyAndCrossCompilationIntegration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_roadmap_next3_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
	let attemptCount = 0
	let runCount = 0

	resilient fn tryTask() retries 3 {
		attemptCount = attemptCount + 1
		if attemptCount < 3 {
			return { "error": "failed attempt" }
		}
		return "success"
	}

	resilient fn slowTask() timeout 50ms {
		time.sleep(150)
		return "done"
	}

	resilient fn failingTask() retries 0 circuit_breaker {
		runCount = runCount + 1
		return { "error": "failing-cb" }
	}

	test "resilient retries test" {
		let res = tryTask()
		assert res == "success"
		assert attemptCount == 3
	}

	test "resilient timeout test" {
		let res, err = slowTask()
		assert err != nil
	}

	test "resilient circuit breaker test" {
		// 1st failure
		let r1, e1 = failingTask()
		assert e1 != nil

		// 2nd failure
		let r2, e2 = failingTask()
		assert e2 != nil

		// 3rd failure (trips the breaker)
		let r3, e3 = failingTask()
		assert e3 != nil

		assert runCount == 3

		// 4th call: fails fast
		let r4, e4 = failingTask()
		assert e4 != nil
		assert runCount == 3
	}
	`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	// 1. Run integration tests for resiliency
	runTests(tmpFile.Name(), false, "")

	// 2. Verify Cross-Compilation target build succeeds
	binPath, err := buildServNoExit(tmpFile.Name(), "test_service_linux", "", "linux", "amd64")
	if err != nil {
		t.Fatalf("failed cross-compiling to linux/amd64: %v", err)
	}
	defer os.Remove(binPath)
	if _, err := os.Stat(binPath); err != nil {
		t.Errorf("expected compiled binary at %s, but got: %v", binPath, err)
	}
}

func TestDependencyInjectionAndGraphQL(t *testing.T) {
	src := `
	interface UserStore {
		fn load() -> string
	}
	inject myStore: UserStore

	graphql "/api/graphql" {
		route "POST" "/query" (req) {
			return "graphql-response"
		}
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	// Verify AST contains InjectStmt and GraphQLStmt
	var injectFound, graphqlFound bool
	for _, stmt := range prog.Statements {
		if _, ok := stmt.(*compiler.InjectStmt); ok {
			injectFound = true
		}
		if _, ok := stmt.(*compiler.GraphQLStmt); ok {
			graphqlFound = true
		}
	}

	if !injectFound {
		t.Errorf("expected InjectStmt in AST")
	}
	if !graphqlFound {
		t.Errorf("expected GraphQLStmt in AST")
	}

	// Verify codegen outputs correctly
	codegen := compiler.NewCodegen(prog)
	generated, err := codegen.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(generated, "dependency injection wired: var myStore UserStore") {
		t.Errorf("expected dependency injection comment in generated code")
	}
	if !strings.Contains(generated, `graphql handler registered at "/api/graphql"`) {
		t.Errorf("expected graphql handler comment in generated code")
	}
}

func TestCompileTimeMacrosAndLspActions(t *testing.T) {
	src := `
	@derive(Serialize, Validate)
	struct User {
		id: int,
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	// Verify AST contains MacroStmt
	var macroFound bool
	for _, stmt := range prog.Statements {
		if _, ok := stmt.(*compiler.MacroStmt); ok {
			macroFound = true
		}
	}

	if !macroFound {
		t.Errorf("expected MacroStmt in AST")
	}

	// Verify codegen outputs correctly
	codegen := compiler.NewCodegen(prog)
	generated, err := codegen.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	if !strings.Contains(generated, "compile-time macro expanded: @derive(Serialize, Validate)") {
		t.Errorf("expected macro expansion comment in generated code")
	}

	// Test LSP action runner helper does not panic
	tmpFile, err := os.CreateTemp("", "test_lsp_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())
	tmpFile.Close()

	// Capture execution
	runLspActionCmd([]string{"--file", tmpFile.Name(), "--line", "10", "--type", "quickfix"})
}

func TestDomainDrivenDecompositionLinter(t *testing.T) {
	src := `
	fn auth_private_validate() {
		return "valid"
	}
	route "POST" "/api/v1/billing/pay" (req) {
		let x = auth_private_validate()
		return "paid"
	}
	`
	lexer := compiler.NewLexer(src)
	parser := compiler.NewParser(lexer)
	prog := parser.ParseProgram()
	if len(parser.Errors()) > 0 {
		t.Fatalf("parser errors: %v", parser.Errors())
	}

	diags := compiler.Analyze(prog)
	var boundaryViolationFound bool
	for _, d := range diags {
		if strings.Contains(d.Message, "Domain boundary violation") {
			boundaryViolationFound = true
		}
	}

	if !boundaryViolationFound {
		t.Errorf("Expected domain boundary violation warning, but none found")
	}
}

func TestScaffoldingCLI(t *testing.T) {
	apiFile := "user_api.srv"
	defer os.Remove(apiFile)
	runGenerateAPIScaffold("User")
	content, err := os.ReadFile(apiFile)
	if err != nil {
		t.Fatalf("failed to read api scaffold file: %v", err)
	}
	if !strings.Contains(string(content), "route \"GET\" \"/api/v1/user\"") {
		t.Errorf("scaffold missing route: %s", string(content))
	}

	dbFile := "user_db.srv"
	defer os.Remove(dbFile)
	runGenerateDBScaffold("User")
	content, err = os.ReadFile(dbFile)
	if err != nil {
		t.Fatalf("failed to read db scaffold file: %v", err)
	}
	if !strings.Contains(string(content), "database UserDb") {
		t.Errorf("scaffold missing database block: %s", string(content))
	}

	wfFile := "user_workflow.srv"
	defer os.Remove(wfFile)
	runGenerateWorkflowScaffold("User")
	content, err = os.ReadFile(wfFile)
	if err != nil {
		t.Fatalf("failed to read workflow scaffold file: %v", err)
	}
	if !strings.Contains(string(content), "workflow UserFlow (data)") {
		t.Errorf("scaffold missing workflow block: %s", string(content))
	}
}

func TestSandboxConfigGeneration(t *testing.T) {
	srvFile := "mock_service.srv"
	code := `
database CustomerDb {
    engine: "postgres"
    connection: "postgresql://prod-db:5432/customers"
}
queue MyQueue {
    broker: "stomp://prod-broker:61613"
}
store MyStore {}
`
	if err := os.WriteFile(srvFile, []byte(code), 0644); err != nil {
		t.Fatalf("failed to write mock service file: %v", err)
	}
	defer os.Remove(srvFile)

	sandboxFile := "sandbox_test_config.json"
	defer os.Remove(sandboxFile)

	origArgs := os.Args
	defer func() { os.Args = origArgs }()
	os.Args = []string{"serv", "generate", "sandbox", srvFile, "-o", sandboxFile}

	runGenerateSandbox(srvFile)

	content, err := os.ReadFile(sandboxFile)
	if err != nil {
		t.Fatalf("failed to read sandbox config file: %v", err)
	}

	var parsed map[string]interface{}
	if err := json.Unmarshal(content, &parsed); err != nil {
		t.Fatalf("failed to parse sandbox config: %v", err)
	}

	if parsed["environment"] != "sandbox_digital_twin" {
		t.Errorf("expected sandbox_digital_twin environment, got %v", parsed["environment"])
	}

	dbs := parsed["databases"].(map[string]interface{})
	if dbs["CustomerDb"] == nil {
		t.Errorf("missing CustomerDb sandbox mapping")
	} else {
		custDb := dbs["CustomerDb"].(map[string]interface{})
		if custDb["engine"] != "sqlite" {
			t.Errorf("expected engine to be sqlite, got %v", custDb["engine"])
		}
	}

	queues := parsed["queues"].(map[string]interface{})
	if len(queues) == 0 {
		t.Errorf("expected queues mapping to be present")
	}

	storage := parsed["storage"].(map[string]interface{})
	if storage["SandboxStore"] == nil {
		t.Errorf("expected SandboxStore storage mapping")
	}
}
