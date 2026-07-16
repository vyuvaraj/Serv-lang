package compiler

import (
	"fmt"
	"strings"
)

// analyzeTypeMismatches performs basic type checking on function calls within a function body.
func analyzeTypeMismatches(fn *FnDecl, program *Program) []Diagnostic {
	var diags []Diagnostic

	// Build a map of function signatures from the program
	funcSigs := make(map[string]*FnDecl)
	structSigs := make(map[string]*StructDecl)
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *FnDecl:
			funcSigs[s.Name] = s
		case *StructDecl:
			structSigs[s.Name] = s
		case *ExportStmt:
			if inner, ok := s.Inner.(*FnDecl); ok {
				funcSigs[inner.Name] = inner
			}
			if inner, ok := s.Inner.(*StructDecl); ok {
				structSigs[inner.Name] = inner
			}
		}
	}

	// Walk the function body looking for call expressions and null safety violations
	for _, stmt := range fn.Body.Statements {
		diags = append(diags, checkCallTypes(stmt, funcSigs, fn)...)
		diags = append(diags, checkReturnTypes(stmt, fn)...)
		diags = append(diags, checkStructLiteralTypes(stmt, structSigs)...)
		diags = append(diags, checkNullSafety(stmt)...)
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
				typeParamSet := make(map[string]bool)
				for _, tp := range callee.TypeParams {
					typeParamSet[tp] = true
				}
				if enclosingFn != nil {
					for _, tp := range enclosingFn.TypeParams {
						typeParamSet[tp] = true
					}
				}

				for i, arg := range e.Arguments {
					if i >= len(callee.ParamTypes) || callee.ParamTypes[i] == "" {
						continue
					}
					expectedType := callee.ParamTypes[i]
					isTypeParam := typeParamSet[expectedType]
					if strings.HasPrefix(expectedType, "[]") && typeParamSet[strings.TrimPrefix(expectedType, "[]")] {
						isTypeParam = true
					}
					if isTypeParam {
						continue
					}

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
	// nil is compatible with optional types (T?)
	if actual == "nil" {
		if strings.HasSuffix(expected, "?") {
			return true
		}
		// nil is compatible with union types that include nil-like behavior
		if strings.Contains(expected, "|") {
			return true
		}
		// nil is NOT compatible with non-optional concrete types
		if expected == "int" || expected == "float" || expected == "string" || expected == "bool" {
			return false
		}
		// For untyped params, allow nil
		return true
	}
	// Optional types accept their base type: string? accepts string
	if strings.HasSuffix(expected, "?") {
		baseExpected := strings.TrimSuffix(expected, "?")
		if actual == baseExpected {
			return true
		}
	}
	// Union types: check if actual matches any member
	if strings.Contains(expected, "|") {
		members := strings.Split(expected, "|")
		for _, m := range members {
			m = strings.TrimSpace(m)
			if typesCompatible(actual, m) {
				return true
			}
		}
		return false
	}
	// float accepts int
	if expected == "float" && actual == "int" {
		return true
	}
	// string is compatible with error type
	if expected == "error" && actual == "string" {
		return true
	}
	// Allow any -> any (no constraint)
	if expected == "any" || expected == "" {
		return true
	}
	return false
}

// checkNullSafety checks for nil assignments to non-optional typed variables.
func checkNullSafety(stmt Statement) []Diagnostic {
	var diags []Diagnostic

	switch s := stmt.(type) {
	case *LetStmt:
		// If the variable has an explicit type annotation that is NOT optional, nil is not allowed
		if s.Type != "" && !strings.HasSuffix(s.Type, "?") && !strings.Contains(s.Type, "|") {
			if _, isNil := s.Value.(*NilLiteral); isNil {
				diags = append(diags, Diagnostic{
					Line:     s.Token.Line,
					Col:      s.Token.Col,
					Severity: "error",
					Message:  fmt.Sprintf("cannot assign nil to non-optional type '%s' (use '%s?' to allow nil)", s.Type, s.Type),
				})
			}
		}
	case *IfStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				diags = append(diags, checkNullSafety(inner)...)
			}
		}
		if s.ElseBody != nil {
			for _, inner := range s.ElseBody.Statements {
				diags = append(diags, checkNullSafety(inner)...)
			}
		}
	case *ForStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				diags = append(diags, checkNullSafety(inner)...)
			}
		}
	}

	return diags
}

func checkReturnTypes(stmt Statement, enclosingFn *FnDecl) []Diagnostic {
	var diags []Diagnostic
	if enclosingFn == nil || enclosingFn.ReturnType == "" || enclosingFn.ReturnType == "any" {
		return diags
	}

	switch s := stmt.(type) {
	case *ReturnStmt:
		if s.Value == nil {
			diags = append(diags, Diagnostic{
				Line:     s.Token.Line,
				Col:      s.Token.Col,
				Severity: "error",
				Message:  fmt.Sprintf("function '%s' expects return type '%s', got empty return", enclosingFn.Name, enclosingFn.ReturnType),
			})
			return diags
		}
		actualType := inferLiteralType(s.Value)
		if actualType != "" && !typesCompatible(actualType, enclosingFn.ReturnType) {
			diags = append(diags, Diagnostic{
				Line:     s.Token.Line,
				Col:      s.Token.Col,
				Severity: "error",
				Message:  fmt.Sprintf("function '%s' expects return type '%s', got '%s'", enclosingFn.Name, enclosingFn.ReturnType, actualType),
			})
		}
	case *IfStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				diags = append(diags, checkReturnTypes(inner, enclosingFn)...)
			}
		}
		if s.ElseBody != nil {
			for _, inner := range s.ElseBody.Statements {
				diags = append(diags, checkReturnTypes(inner, enclosingFn)...)
			}
		}
	case *ForStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				diags = append(diags, checkReturnTypes(inner, enclosingFn)...)
			}
		}
	}
	return diags
}

func checkStructLiteralTypes(stmt Statement, structSigs map[string]*StructDecl) []Diagnostic {
	var diags []Diagnostic

	switch s := stmt.(type) {
	case *LetStmt:
		diags = append(diags, checkExprStructLiteral(s.Value, structSigs)...)
	case *ExprStmt:
		diags = append(diags, checkExprStructLiteral(s.Value, structSigs)...)
	case *ReturnStmt:
		diags = append(diags, checkExprStructLiteral(s.Value, structSigs)...)
	case *IfStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				diags = append(diags, checkStructLiteralTypes(inner, structSigs)...)
			}
		}
		if s.ElseBody != nil {
			for _, inner := range s.ElseBody.Statements {
				diags = append(diags, checkStructLiteralTypes(inner, structSigs)...)
			}
		}
	case *ForStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				diags = append(diags, checkStructLiteralTypes(inner, structSigs)...)
			}
		}
	}
	return diags
}

func checkExprStructLiteral(expr Expression, structSigs map[string]*StructDecl) []Diagnostic {
	var diags []Diagnostic
	if expr == nil {
		return diags
	}

	switch e := expr.(type) {
	case *StructLiteral:
		if sDecl, ok := structSigs[e.TypeName]; ok {
			fields := make(map[string]string)
			for _, f := range sDecl.Fields {
				fields[f.Name] = f.Type
			}
			for k, val := range e.Fields {
				expectedType, ok := fields[k]
				if !ok {
					diags = append(diags, Diagnostic{
						Line:     e.Token.Line,
						Col:      e.Token.Col,
						Severity: "error",
						Message:  fmt.Sprintf("struct '%s' has no field '%s'", e.TypeName, k),
					})
					continue
				}
				actualType := inferLiteralType(val)
				if actualType != "" && !typesCompatible(actualType, expectedType) {
					diags = append(diags, Diagnostic{
						Line:     e.Token.Line,
						Col:      e.Token.Col,
						Severity: "error",
						Message:  fmt.Sprintf("field '%s.%s' expects type '%s', got '%s'", e.TypeName, k, expectedType, actualType),
					})
				}
			}
		}
	case *CallExpr:
		for _, arg := range e.Arguments {
			diags = append(diags, checkExprStructLiteral(arg, structSigs)...)
		}
	}
	return diags
}

// analyzeInterfaceSatisfaction checks compile-time verification that structs implement declared interfaces (LANG.8)
func analyzeInterfaceSatisfaction(program *Program) []Diagnostic {
	var diags []Diagnostic

	// 1. Gather all interfaces
	interfaces := make(map[string]*InterfaceDecl)
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *InterfaceDecl:
			interfaces[s.Name] = s
		case *ExportStmt:
			if inner, ok := s.Inner.(*InterfaceDecl); ok {
				interfaces[inner.Name] = inner
			}
		}
	}

	// 2. Gather all structs
	structs := make(map[string]*StructDecl)
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *StructDecl:
			structs[s.Name] = s
		case *ExportStmt:
			if inner, ok := s.Inner.(*StructDecl); ok {
				structs[inner.Name] = inner
			}
		}
	}

	// 3. Gather all methods for each struct receiver
	methods := make(map[string]map[string]*MethodDecl)
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *MethodDecl:
			if _, exists := methods[s.TypeName]; !exists {
				methods[s.TypeName] = make(map[string]*MethodDecl)
			}
			methods[s.TypeName][s.Name] = s
		case *ExportStmt:
			if inner, ok := s.Inner.(*MethodDecl); ok {
				if _, exists := methods[inner.TypeName]; !exists {
					methods[inner.TypeName] = make(map[string]*MethodDecl)
				}
				methods[inner.TypeName][inner.Name] = inner
			}
		}
	}

	// Walk all statements (recursively through functions, route blocks)
	var walkStatements func(st Statement)
	walkStatements = func(st Statement) {
		if st == nil {
			return
		}
		switch s := st.(type) {
		case *BlockStmt:
			if s != nil {
				for _, inner := range s.Statements {
					if inner != nil {
						walkStatements(inner)
					}
				}
			}
		case *IfStmt:
			walkStatements(s.Body)
			walkStatements(s.ElseBody)
		case *ForStmt:
			walkStatements(s.Body)
		case *AppStmt:
			walkStatements(s.Body)
		case *RouteStmt:
			walkStatements(s.Body)
		case *FnDecl:
			walkStatements(s.Body)
		case *ExportStmt:
			walkStatements(s.Inner)
		case *EveryStmt:
			walkStatements(s.Body)
		case *CronStmt:
			walkStatements(s.Body)
		case *LetStmt:
			if s.Type != "" {
				if iface, isIface := interfaces[s.Type]; isIface {
					if structLit, isStructLit := s.Value.(*StructLiteral); isStructLit {
						strName := structLit.TypeName
						structMethods := methods[strName]
						for _, m := range iface.Methods {
							mDecl, hasMethod := structMethods[m.Name]
							if !hasMethod {
								diags = append(diags, Diagnostic{
									Line:     s.Token.Line,
									Col:      s.Token.Col,
									Severity: "error",
									Message:  fmt.Sprintf("struct '%s' does not implement interface '%s' (missing method '%s')", strName, iface.Name, m.Name),
								})
							} else {
								if len(mDecl.Params) != len(m.Params) {
									diags = append(diags, Diagnostic{
										Line:     s.Token.Line,
										Col:      s.Token.Col,
										Severity: "error",
										Message:  fmt.Sprintf("method '%s.%s' signature mismatch: expected %d parameters, got %d", strName, m.Name, len(m.Params), len(mDecl.Params)),
									})
								}
							}
						}
					}
				}
			}

			// CD.76 Inter-service contracts validation: check let response = http.get("serv://ServiceName/route")
			if s.Type != "" {
				if call, isCall := s.Value.(*CallExpr); isCall {
					if ident, isIdent := call.Function.(*MemberExpr); isIdent {
						if obj, isObj := ident.Object.(*Identifier); isObj && obj.Value == "http" && (ident.Field == "get" || ident.Field == "post") {
							if len(call.Arguments) > 0 {
								if strLit, isStr := call.Arguments[0].(*StringLiteral); isStr && strings.HasPrefix(strLit.Value, "serv://") {
									uriVal := strings.TrimPrefix(strLit.Value, "serv://")
									parts := strings.SplitN(uriVal, "/", 2)
									if len(parts) == 2 {
										serviceName := parts[0]
										routePath := "/" + parts[1]

										// Gather routes from other declared services/apps in the compiler tree
										foundService := false
										var matchedRoute *RouteStmt
										for _, mainStmt := range program.Statements {
											if app, isApp := mainStmt.(*AppStmt); isApp && app.Name == serviceName {
												foundService = true
												for _, appStmt := range app.Body.Statements {
													if r, isR := appStmt.(*RouteStmt); isR && r.Path == routePath {
														matchedRoute = r
														break
													}
												}
											}
										}

										if foundService {
											if matchedRoute == nil {
												diags = append(diags, Diagnostic{
													Line:     s.Token.Line,
													Col:      s.Token.Col,
													Severity: "error",
													Message:  fmt.Sprintf("service '%s' does not declare route '%s'", serviceName, routePath),
												})
											} else if matchedRoute.ReturnType != "" && matchedRoute.ReturnType != s.Type {
												diags = append(diags, Diagnostic{
													Line:     s.Token.Line,
													Col:      s.Token.Col,
									Severity: "error",
									Message:  fmt.Sprintf("inter-service contract mismatch: route '%s' on '%s' returns '%s', but assigning to '%s'", routePath, serviceName, matchedRoute.ReturnType, s.Type),
												})
											}
										}
									}
								}
							}
						}
					}
				}
			}
		}
	}

	for _, stmt := range program.Statements {
		walkStatements(stmt)
	}

	return diags
}

