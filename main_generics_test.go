package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"serv/compiler"
)

func TestGenericsUnitParser(t *testing.T) {
	input := `
struct Box[T] {
	value: T
}

fn identity[T](x: T) -> T {
	return x
}
`
	l := compiler.NewLexer(input)
	p := compiler.NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("parser errors: %v", p.Errors())
	}

	if len(prog.Statements) != 2 {
		t.Fatalf("expected 2 statements, got %d", len(prog.Statements))
	}

	// 1. Verify StructDecl
	structDecl, ok := prog.Statements[0].(*compiler.StructDecl)
	if !ok {
		t.Fatalf("expected StructDecl, got %T", prog.Statements[0])
	}
	if structDecl.Name != "Box" {
		t.Errorf("expected struct name Box, got %s", structDecl.Name)
	}
	if len(structDecl.TypeParams) != 1 || structDecl.TypeParams[0] != "T" {
		t.Errorf("expected TypeParams [T], got %v", structDecl.TypeParams)
	}

	// 2. Verify FnDecl
	fnDecl, ok := prog.Statements[1].(*compiler.FnDecl)
	if !ok {
		t.Fatalf("expected FnDecl, got %T", prog.Statements[1])
	}
	if fnDecl.Name != "identity" {
		t.Errorf("expected function name identity, got %s", fnDecl.Name)
	}
	if len(fnDecl.TypeParams) != 1 || fnDecl.TypeParams[0] != "T" {
		t.Errorf("expected TypeParams [T], got %v", fnDecl.TypeParams)
	}
}

func TestGenericsIntegration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_generics_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
struct Box[T] {
	value: T
}

fn identity[T](x: T) -> T {
	return x
}

fn getValue[T](b: Box[T]) -> T {
	return b.value
}

fn addOne[T: Numeric](x: T) -> T {
	return x + 1
}

test "generics execution test" {
	// Generic function
	let x = identity[int](42)
	assert x == 42

	let s = identity[string]("hello")
	assert s == "hello"

	// Generic struct
	let b = Box[int]{ value: 100 }
	assert b.value == 100

	let val = getValue[int](b)
	assert val == 100

	// Constraints
	let num = addOne[int](5)
	assert num == 6
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

	t.Logf("Generics Execution Output:\n%s", output)

	if !strings.Contains(output, "PASS") {
		t.Error("Expected tests to pass successfully")
	}
	if strings.Contains(output, "FAIL") {
		t.Error("Expected no tests to fail")
	}
}
