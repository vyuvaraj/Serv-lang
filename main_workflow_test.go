package main

import (
	"bytes"
	"os"
	"strings"
	"testing"

	"serv/compiler"
)

// ---------------------------------------------------------------------------
// Unit tests — parser
// ---------------------------------------------------------------------------

func TestWorkflowParser(t *testing.T) {
	input := `
workflow OrderProcessing(order) {
	let payment = await processPayment(order)
	let shipment = await arrangeShipment(order)
	return shipment
}
`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}

	wf, ok := prog.Statements[0].(*compiler.WorkflowDecl)
	if !ok {
		t.Fatalf("expected WorkflowDecl, got %T", prog.Statements[0])
	}
	if wf.Name != "OrderProcessing" {
		t.Errorf("expected workflow name OrderProcessing, got %s", wf.Name)
	}
	if wf.Param != "order" {
		t.Errorf("expected param 'order', got %q", wf.Param)
	}
	if wf.Body == nil {
		t.Error("expected non-nil body")
	}
	if len(wf.Body.Statements) != 3 {
		t.Errorf("expected 3 body statements, got %d", len(wf.Body.Statements))
	}
}

func TestWorkflowNoParamParser(t *testing.T) {
	input := `
workflow Bootstrap() {
	let result = await fetchConfig()
	return result
}
`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}
	if len(prog.Statements) != 1 {
		t.Fatalf("expected 1 statement, got %d", len(prog.Statements))
	}
	wf, ok := prog.Statements[0].(*compiler.WorkflowDecl)
	if !ok {
		t.Fatalf("expected WorkflowDecl, got %T", prog.Statements[0])
	}
	if wf.Name != "Bootstrap" {
		t.Errorf("expected name Bootstrap, got %s", wf.Name)
	}
}

// ---------------------------------------------------------------------------
// Unit test — codegen produces expected Go output
// ---------------------------------------------------------------------------

func TestWorkflowCodegen(t *testing.T) {
	input := `
workflow SimpleFlow(data) {
	let step1 = await processStep(data)
	return step1
}
`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	cg := compiler.NewCodegen(prog)
	code, err := cg.Generate()
	if err != nil {
		t.Fatalf("codegen error: %v", err)
	}

	// The generated code should contain the workflow function signature.
	if !strings.Contains(code, "workflow_SimpleFlow") {
		t.Errorf("expected 'workflow_SimpleFlow' in generated code, got:\n%s", code)
	}
	// The generated code should register the workflow.
	if !strings.Contains(code, `runtime.RegisterWorkflow("SimpleFlow"`) {
		t.Errorf("expected RegisterWorkflow call, got:\n%s", code)
	}
	// The await inside should be wrapped in _wfCtx.Step(...)
	if !strings.Contains(code, "_wfCtx.Step(") {
		t.Errorf("expected '_wfCtx.Step(' in generated code for await, got:\n%s", code)
	}
	// Must NOT use runtime.Await inside a workflow.
	if strings.Contains(code, "runtime.Await(") {
		t.Errorf("should NOT emit runtime.Await() inside workflow, got:\n%s", code)
	}

	t.Logf("Generated workflow code:\n%s", code)
}

// ---------------------------------------------------------------------------
// Integration test — end-to-end compile + run
// ---------------------------------------------------------------------------

func TestWorkflowIntegration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_workflow_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
fn processPayment(order) {
	return "payment-ok"
}

fn arrangeShipment(order) {
	return "shipment-arranged"
}

workflow OrderProcessing(order) {
	let payment = await processPayment(order)
	let shipment = await arrangeShipment(order)
	return shipment
}

test "workflow codegen compiles and runs" {
	let id = workflow.start("OrderProcessing", "order-001")
	assert id != nil
}
`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	// Intercept stdout to check test runner output.
	oldStdout := os.Stdout
	r, w, _ := os.Pipe()
	os.Stdout = w

	done := make(chan string)
	go func() {
		var buf bytes.Buffer
		_, _ = buf.ReadFrom(r)
		done <- buf.String()
	}()

	runTests(tmpFile.Name(), false, "")

	w.Close()
	os.Stdout = oldStdout
	output := <-done

	t.Logf("Workflow Execution Output:\n%s", output)

	if !strings.Contains(output, "PASS") {
		t.Error("Expected workflow tests to pass")
	}
	if strings.Contains(output, "FAIL") {
		t.Error("Expected no workflow tests to fail")
	}
}
