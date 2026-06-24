package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"serv/compiler"
)

func TestPipeParser(t *testing.T) {
	input := `
fn double(x: int) -> int {
	return x * 2
}
fn add(x: int, y: int) -> int {
	return x + y
}
fn run() {
	let val = 5 |> double() |> add(10)
}
`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	// Verify parser output matches expected desugared form.
	// 5 |> double() |> add(10) should parse to: add(double(5), 10)
	// Statements[2] is fn run() { ... }
	fnRun, ok := prog.Statements[2].(*compiler.FnDecl)
	if !ok {
		t.Fatalf("expected FnDecl, got %T", prog.Statements[2])
	}
	letStmt, ok := fnRun.Body.Statements[0].(*compiler.LetStmt)
	if !ok {
		t.Fatalf("expected LetStmt, got %T", fnRun.Body.Statements[0])
	}
	
	// Value should be a CallExpr: add(double(5), 10)
	callAdd, ok := letStmt.Value.(*compiler.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for add, got %T", letStmt.Value)
	}
	
	fnIdent, ok := callAdd.Function.(*compiler.Identifier)
	if !ok || fnIdent.Value != "add" {
		t.Fatalf("expected function name 'add', got %v", callAdd.Function)
	}
	if len(callAdd.Arguments) != 2 {
		t.Fatalf("expected 2 arguments for add, got %d", len(callAdd.Arguments))
	}
	
	// First argument should be: double(5)
	callDouble, ok := callAdd.Arguments[0].(*compiler.CallExpr)
	if !ok {
		t.Fatalf("expected CallExpr for double, got %T", callAdd.Arguments[0])
	}
	doubleIdent, ok := callDouble.Function.(*compiler.Identifier)
	if !ok || doubleIdent.Value != "double" {
		t.Fatalf("expected function name 'double', got %v", callDouble.Function)
	}
	if len(callDouble.Arguments) != 1 {
		t.Fatalf("expected 1 argument for double, got %d", len(callDouble.Arguments))
	}
	
	// Second argument to add should be: 10
	intLit, ok := callAdd.Arguments[1].(*compiler.IntegerLiteral)
	if !ok || intLit.Value != 10 {
		t.Fatalf("expected integer literal 10, got %v", callAdd.Arguments[1])
	}
}

func TestPipeIntegration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_pipe_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
fn double(x: int) -> int {
	return x * 2
}

fn add(x: int, y: int) -> int {
	return x + y
}

struct Helper {
	multiplier: int
}

fn Helper.multiply(x: int) -> int {
	return x * self.multiplier
}

test "pipe operator integration test" {
	// Standard calls
	let x = 5 |> double() |> add(10)
	assert x == 20

	// Implicit call wrapping (no parens)
	let y = 5 |> double |> add(3)
	assert y == 13

	// Member function calls
	let h = Helper{multiplier: 3}
	let z = 4 |> h.multiply()
	assert z == 12

	// Implicit member function calls
	let w = 4 |> h.multiply
	assert w == 12
}
`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	// Intercept stdout to check test runner output
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

	t.Logf("Pipe Execution Output:\n%s", output)

	if !strings.Contains(output, "PASS") {
		t.Error("Expected tests to pass successfully")
	}
	if strings.Contains(output, "FAIL") {
		t.Error("Expected no tests to fail")
	}
}

