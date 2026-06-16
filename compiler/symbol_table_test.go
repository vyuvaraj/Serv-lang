package compiler

import (
	"testing"
)

func TestScopeHierarchy(t *testing.T) {
	// Create root scope
	root := NewScope(nil)

	// Insert into root
	symX := &Symbol{Name: "x", Type: "variable"}
	if !root.Insert(symX) {
		t.Fatalf("expected successful insert of 'x' in root scope")
	}

	// Double insert in same scope should fail
	if root.Insert(symX) {
		t.Fatalf("expected duplicate insert of 'x' in root scope to fail")
	}

	// Create child scope
	child := NewScope(root)

	// Child lookup of parent's variable should succeed
	lookupX, found := child.Lookup("x")
	if !found || lookupX != symX {
		t.Fatalf("expected to find parent variable 'x' in child scope lookup")
	}

	// Shadow 'x' in child scope
	symShadowX := &Symbol{Name: "x", Type: "variable"}
	if !child.Insert(symShadowX) {
		t.Fatalf("expected successful insert of shadowed 'x' in child scope")
	}

	// Child lookup of shadowed 'x' should yield child's symbol, not root's symbol
	lookupShadowX, found := child.Lookup("x")
	if !found || lookupShadowX != symShadowX {
		t.Fatalf("expected to find shadowed 'x' in child scope lookup")
	}

	// Root lookup of 'x' should still yield parent's symbol
	lookupRootX, found := root.Lookup("x")
	if !found || lookupRootX != symX {
		t.Fatalf("expected root scope lookup of 'x' to find parent symbol")
	}

	// Local lookup in child scope for 'x' should find child symbol
	lookupLocalChildX, found := child.LookupLocal("x")
	if !found || lookupLocalChildX != symShadowX {
		t.Fatalf("expected local lookup in child to find shadowed child symbol")
	}

	// Local lookup in root scope for 'x' should find parent symbol
	lookupLocalRootX, found := root.LookupLocal("x")
	if !found || lookupLocalRootX != symX {
		t.Fatalf("expected local lookup in root to find root symbol")
	}
}

func TestUnusedVarsShadowingDiagnostics(t *testing.T) {
	// Source code simulating:
	// fn main() {
	//     let x = 1
	//     {
	//         let x = 2
	//         print(x)
	//     }
	// }
	// In this scenario, outer 'x' is unused and should produce a warning,
	// whereas inner 'x' is used and should not.
	
	program := &Program{
		Statements: []Statement{
			&FnDecl{
				Name: "main",
				Body: &BlockStmt{
					Statements: []Statement{
						&LetStmt{
							Name:  "x",
							Value: &IntegerLiteral{Value: 1},
							Token: Token{Line: 2, Col: 5},
						},
						&BlockStmt{
							Statements: []Statement{
								&LetStmt{
									Name:  "x",
									Value: &IntegerLiteral{Value: 2},
									Token: Token{Line: 4, Col: 9},
								},
								&ExprStmt{
									Value: &CallExpr{
										Function: &Identifier{Value: "print"},
										Arguments: []Expression{
											&Identifier{Value: "x"},
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}

	diags := Analyze(program)

	// We expect exactly one warning pointing to outer x on line 2
	var warnings []Diagnostic
	for _, d := range diags {
		if d.Severity == "warning" {
			warnings = append(warnings, d)
		}
	}

	if len(warnings) != 1 {
		t.Fatalf("expected exactly 1 warning, got %d: %v", len(warnings), warnings)
	}

	expectedMsg := "variable 'x' is declared but never used"
	if warnings[0].Message != expectedMsg {
		t.Errorf("expected warning message %q, got %q", expectedMsg, warnings[0].Message)
	}

	if warnings[0].Line != 2 {
		t.Errorf("expected warning on line 2, got line %d", warnings[0].Line)
	}
}
