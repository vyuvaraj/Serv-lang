package compiler

import (
	"strings"
	"testing"
)

func TestInterServiceContracts(t *testing.T) {
	input := `
app OrderService {
	struct Order { id: string }
	route "GET" "/order" (req) -> Order {
		return Order{id: "1"}
	}
}

app InventoryService {
	fn check() {
		let res: int = http.get("serv://OrderService/order")
	}
}
`
	l := NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}

	diags := Analyze(prog)
	foundErr := false
	for _, d := range diags {
		if d.Severity == "error" && strings.Contains(d.Message, "inter-service contract mismatch") {
			foundErr = true
			break
		}
	}

	if !foundErr {
		t.Errorf("expected compile-time type-safety warning for inter-service contract mismatch, got none")
	}
}

func TestInfrastructureReachabilityCheck(t *testing.T) {
	// Bypass reachability skip flag to check warnings triggers
	SkipInfraReachability = false
	defer func() { SkipInfraReachability = true }()

	input := `
database "postgresql://localhost:9999/nonexistent"
cache "redis://localhost:9998"
`
	l := NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}

	diags := Analyze(prog)
	foundDBWarning := false
	foundCacheWarning := false
	for _, d := range diags {
		if d.Severity == "warning" {
			if strings.Contains(d.Message, "database target") && strings.Contains(d.Message, "unreachable") {
				foundDBWarning = true
			}
			if strings.Contains(d.Message, "cache target") && strings.Contains(d.Message, "unreachable") {
				foundCacheWarning = true
			}
		}
	}

	if !foundDBWarning {
		t.Errorf("expected db reachability warning for unreachable port, got none")
	}
	if !foundCacheWarning {
		t.Errorf("expected cache reachability warning for unreachable port, got none")
	}
}

func TestDeadRoutesWarning(t *testing.T) {
	input := `
app OrderService {
	route "GET" "/order" (req) {
		return "ok"
	}
	route "GET" "/unused" (req) {
		return "ok"
	}
}

app InventoryService {
	fn check() {
		let res = http.get("serv://OrderService/order")
	}
}
`
	l := NewLexer(input)
	p := NewParser(l)
	prog := p.ParseProgram()
	if len(p.Errors()) > 0 {
		t.Fatalf("unexpected parser errors: %v", p.Errors())
	}

	diags := Analyze(prog)
	foundWarning := false
	for _, d := range diags {
		if d.Severity == "warning" && strings.Contains(d.Message, "Route '/unused' declared in service 'OrderService' is never called") {
			foundWarning = true
			break
		}
	}

	if !foundWarning {
		t.Errorf("expected warning for dead route '/unused', got none")
	}
}
