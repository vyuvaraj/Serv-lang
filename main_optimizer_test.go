package main

import (
	"os"
	"testing"
	"serv/compiler"
)

func TestOptimizerConstantFolding(t *testing.T) {
	input := `
	let val1 = 2 + 3
	let val2 = 10 - 4
	let val3 = 3 * 4
	let val4 = 12 / 3
	let val5 = 13 % 5
	let val6 = 2.5 + 1.5
	let val7 = "foo" + "bar"
	let val8 = 5 > 3
	let val9 = 5 == 5
	let val10 = !true
	let val11 = -5
	`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	opt := compiler.Optimize(prog)

	// Verify folded results
	expected := []struct {
		name  string
		value string
	}{
		{"val1", "5"},
		{"val2", "6"},
		{"val3", "12"},
		{"val4", "4"},
		{"val5", "3"},
		{"val6", "4.000000"}, // float string format
		{"val7", "\"foobar\""},
		{"val8", "true"},
		{"val9", "true"},
		{"val10", "false"},
		{"val11", "-5"},
	}

	for i, exp := range expected {
		letStmt, ok := opt.Statements[i].(*compiler.LetStmt)
		if !ok {
			t.Fatalf("expected LetStmt at index %d, got %T", i, opt.Statements[i])
		}
		if letStmt.Name != exp.name {
			t.Errorf("expected var name %s, got %s", exp.name, letStmt.Name)
		}
		if letStmt.Value.String() != exp.value {
			t.Errorf("var %s: expected folded value %s, got %s", exp.name, exp.value, letStmt.Value.String())
		}
	}
}

func TestOptimizerDeadBranchElimination(t *testing.T) {
	input := `
	if true {
		let x = 1
	} else {
		let y = 2
	}

	if false {
		let a = 1
	} else {
		let b = 2
	}
	`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	opt := compiler.Optimize(prog)

	if len(opt.Statements) != 2 {
		t.Fatalf("expected 2 optimized statements, got %d: %v", len(opt.Statements), opt.Statements)
	}

	// First statement should fold to the 'if true' body: BlockStmt containing let x = 1
	block1, ok := opt.Statements[0].(*compiler.BlockStmt)
	if !ok || len(block1.Statements) != 1 {
		t.Fatalf("expected statement 1 to fold to BlockStmt with 1 statement, got %T", opt.Statements[0])
	}
	let1, ok := block1.Statements[0].(*compiler.LetStmt)
	if !ok || let1.Name != "x" {
		t.Errorf("expected inner statement to be 'let x = 1', got %s", block1.Statements[0].String())
	}

	// Second statement should fold to the 'if false' else body: BlockStmt containing let b = 2
	block2, ok := opt.Statements[1].(*compiler.BlockStmt)
	if !ok || len(block2.Statements) != 1 {
		t.Fatalf("expected statement 2 to fold to BlockStmt with 1 statement, got %T", opt.Statements[1])
	}
	let2, ok := block2.Statements[0].(*compiler.LetStmt)
	if !ok || let2.Name != "b" {
		t.Errorf("expected inner statement to be 'let b = 2', got %s", block2.Statements[0].String())
	}
}

func TestOptimizerUnreachableCodeElimination(t *testing.T) {
	input := `
	fn testFunc() {
		let x = 1
		return x
		let y = 2
	}
	`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	opt := compiler.Optimize(prog)

	fnDecl, ok := opt.Statements[0].(*compiler.FnDecl)
	if !ok {
		t.Fatalf("expected FnDecl, got %T", opt.Statements[0])
	}

	// Body should only contain: let x = 1, return x (let y = 2 is unreachable and discarded)
	if len(fnDecl.Body.Statements) != 2 {
		t.Errorf("expected 2 statements in function body, got %d: %v", len(fnDecl.Body.Statements), fnDecl.Body.Statements)
	}

	if let, ok := fnDecl.Body.Statements[0].(*compiler.LetStmt); !ok || let.Name != "x" {
		t.Errorf("expected first statement to be 'let x = 1', got %s", fnDecl.Body.Statements[0].String())
	}

	if _, ok := fnDecl.Body.Statements[1].(*compiler.ReturnStmt); !ok {
		t.Errorf("expected second statement to be 'return x', got %s", fnDecl.Body.Statements[1].String())
	}
}

func TestOptimizerIntegration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_opt_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
	test "optimizer basic and branch integration test" {
		let folded = 100 + 50
		assert folded == 150

		let flag = true
		if flag {
			let result = "branch_taken"
			assert result == "branch_taken"
		} else {
			assert false
		}
	}
	`
	if _, err := tmpFile.WriteString(srvContent); err != nil {
		t.Fatalf("failed to write srv file: %v", err)
	}
	tmpFile.Close()

	runTests(tmpFile.Name(), false, "")
}
