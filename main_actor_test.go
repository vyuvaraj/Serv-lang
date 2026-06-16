package main

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"serv/compiler"
)

func TestActorUnitParser(t *testing.T) {
	input := `
actor Counter(initialVal: int) {
	let count = initialVal
	fn receive(msg: string) -> int {
		return count
	}
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

	actorDecl, ok := prog.Statements[0].(*compiler.ActorDecl)
	if !ok {
		t.Fatalf("expected ActorDecl, got %T", prog.Statements[0])
	}
	if actorDecl.Name != "Counter" {
		t.Errorf("expected actor name Counter, got %s", actorDecl.Name)
	}
	if len(actorDecl.Params) != 1 || actorDecl.Params[0] != "initialVal" {
		t.Errorf("expected params [initialVal], got %v", actorDecl.Params)
	}
	if len(actorDecl.ParamTypes) != 1 || actorDecl.ParamTypes[0] != "int" {
		t.Errorf("expected param types [int], got %v", actorDecl.ParamTypes)
	}
}

func TestActorIntegration(t *testing.T) {
	tmpFile, err := os.CreateTemp("", "test_actor_*.srv")
	if err != nil {
		t.Fatalf("failed to create temp file: %v", err)
	}
	defer os.Remove(tmpFile.Name())

	srvContent := `
actor Counter(initialVal: int) {
	let count = initialVal

	fn receive(msg: string) -> int {
		match msg {
			"increment" => { count += 1 }
			"decrement" => { count -= 1 }
			"get" => { return count }
		}
		return count
	}
}

actor Multiplier(factor: int) {
	fn receive(val: int) -> int {
		return val * factor
	}
}

test "actor basic execution test" {
	let pid = spawn Counter(10)
	assert pid != nil

	let val1 = ask(pid, "get")
	assert val1 == 10

	send(pid, "increment")
	send(pid, "increment")

	let val2 = ask(pid, "get")
	assert val2 == 12

	send(pid, "decrement")
	let val3 = ask(pid, "get")
	assert val3 == 11
}

test "multiple actors test" {
	let a1 = spawn Multiplier(2)
	let a2 = spawn Multiplier(5)

	let r1 = ask(a1, 10)
	assert r1 == 20

	let r2 = ask(a2, 10)
	assert r2 == 50
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

	t.Logf("Actor Execution Output:\n%s", output)

	if !strings.Contains(output, "PASS") {
		t.Error("Expected tests to pass successfully")
	}
	if strings.Contains(output, "FAIL") {
		t.Error("Expected no tests to fail")
	}
}
