package compiler

import "fmt"

// analyzeTypeMismatches performs basic type checking on function calls within a function body.
func analyzeTypeMismatches(fn *FnDecl, program *Program) []Diagnostic {
	var diags []Diagnostic

	// Build a map of function signatures from the program
	funcSigs := make(map[string]*FnDecl)
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *FnDecl:
			funcSigs[s.Name] = s
		case *ExportStmt:
			if inner, ok := s.Inner.(*FnDecl); ok {
				funcSigs[inner.Name] = inner
			}
		}
	}

	// Walk the function body looking for call expressions
	for _, stmt := range fn.Body.Statements {
		diags = append(diags, checkCallTypes(stmt, funcSigs, fn)...)
	}

	return diags
}

// checkCallTypes recursively checks function call argument types.
func checkCallTypes(stmt Statement, funcSigs map[string]*FnDecl, enclosingFn *FnDecl) []Diagnostic {
	var diags []Diagnostic

	switch s := stmt.(type) {
	case *ExprStmt:
		diags = append(diags, checkExprCallTypes(s.Value, funcSigs, enclosingFn)...)
	case *LetStmt:
		diags = append(diags, checkExprCallTypes(s.Value, funcSigs, enclosingFn)...)
	case *ReturnStmt:
		diags = append(diags, checkExprCallTypes(s.Value, funcSigs, enclosingFn)...)
	case *IfStmt:
		diags = append(diags, checkExprCallTypes(s.Condition, funcSigs, enclosingFn)...)
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				diags = append(diags, checkCallTypes(inner, funcSigs, enclosingFn)...)
			}
		}
		if s.ElseBody != nil {
			for _, inner := range s.ElseBody.Statements {
				diags = append(diags, checkCallTypes(inner, funcSigs, enclosingFn)...)
			}
		}
	case *ForStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				diags = append(diags, checkCallTypes(inner, funcSigs, enclosingFn)...)
			}
		}
	}

	return diags
}

// checkExprCallTypes checks a single expression for type mismatches in function calls.
func checkExprCallTypes(expr Expression, funcSigs map[string]*FnDecl, enclosingFn *FnDecl) []Diagnostic {
	var diags []Diagnostic
	if expr == nil {
		return diags
	}

	switch e := expr.(type) {
	case *CallExpr:
		if ident, ok := e.Function.(*Identifier); ok {
			if callee, exists := funcSigs[ident.Value]; exists {
				// Check argument count
				if len(callee.Params) > 0 && len(e.Arguments) != len(callee.Params) {
					diags = append(diags, Diagnostic{
						Line:     e.Token.Line,
						Col:      e.Token.Col,
						Severity: "error",
						Message:  fmt.Sprintf("function '%s' expects %d arguments, got %d", ident.Value, len(callee.Params), len(e.Arguments)),
					})
				}

				// Check argument types (when both are known)
				for i, arg := range e.Arguments {
					if i >= len(callee.ParamTypes) || callee.ParamTypes[i] == "" {
						continue
					}
					expectedType := callee.ParamTypes[i]
					actualType := inferLiteralType(arg)
					if actualType != "" && !typesCompatible(actualType, expectedType) {
						diags = append(diags, Diagnostic{
							Line:     e.Token.Line,
							Col:      e.Token.Col,
							Severity: "error",
							Message:  fmt.Sprintf("argument %d of '%s' expects type '%s', got '%s'", i+1, ident.Value, expectedType, actualType),
						})
					}
				}
			}
		}
		// Recurse into arguments
		for _, arg := range e.Arguments {
			diags = append(diags, checkExprCallTypes(arg, funcSigs, enclosingFn)...)
		}
	case *InfixExpr:
		diags = append(diags, checkExprCallTypes(e.Left, funcSigs, enclosingFn)...)
		diags = append(diags, checkExprCallTypes(e.Right, funcSigs, enclosingFn)...)
	case *MemberExpr:
		diags = append(diags, checkExprCallTypes(e.Object, funcSigs, enclosingFn)...)
	}

	return diags
}

// inferLiteralType returns the type of a literal expression, or "" if unknown.
func inferLiteralType(expr Expression) string {
	switch expr.(type) {
	case *IntegerLiteral:
		return "int"
	case *FloatLiteral:
		return "float"
	case *StringLiteral, *FStringLiteral:
		return "string"
	case *BooleanLiteral:
		return "bool"
	case *NilLiteral:
		return "nil"
	}
	return ""
}

// typesCompatible checks if an actual type can be passed where expected type is required.
func typesCompatible(actual, expected string) bool {
	if actual == expected {
		return true
	}
	// nil is compatible with any type
	if actual == "nil" {
		return true
	}
	// float accepts int
	if expected == "float" && actual == "int" {
		return true
	}
	// Allow any -> any (no constraint)
	if expected == "any" || expected == "" {
		return true
	}
	return false
}
