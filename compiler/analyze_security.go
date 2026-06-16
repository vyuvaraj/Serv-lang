package compiler

// analyzeSQLInjection scans the program's statements to find db.query(...) calls
// that use string concatenation or interpolation, suggesting SQL injection risks.
func analyzeSQLInjection(program *Program) []Diagnostic {
	var diags []Diagnostic
	for _, stmt := range program.Statements {
		diags = append(diags, checkStmtSQLInjection(stmt)...)
	}
	return diags
}

func checkStmtSQLInjection(stmt Statement) []Diagnostic {
	var diags []Diagnostic
	if stmt == nil {
		return diags
	}

	switch s := stmt.(type) {
	case *BlockStmt:
		for _, child := range s.Statements {
			diags = append(diags, checkStmtSQLInjection(child)...)
		}
	case *LetStmt:
		diags = append(diags, checkExprSQLInjection(s.Value)...)
	case *ReturnStmt:
		diags = append(diags, checkExprSQLInjection(s.Value)...)
	case *ExprStmt:
		diags = append(diags, checkExprSQLInjection(s.Value)...)
	case *IfStmt:
		diags = append(diags, checkExprSQLInjection(s.Condition)...)
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
		diags = append(diags, checkStmtSQLInjection(s.ElseBody)...)
	case *ForStmt:
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
	case *FnDecl:
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
	case *RouteStmt:
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
	case *EveryStmt:
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
	case *CronStmt:
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
	case *SubscribeStmt:
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
	case *MiddlewareDecl:
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
	case *TestStmt:
		diags = append(diags, checkStmtSQLInjection(s.Body)...)
	case *ExportStmt:
		diags = append(diags, checkStmtSQLInjection(s.Inner)...)
	case *TryCatchStmt:
		diags = append(diags, checkStmtSQLInjection(s.TryBody)...)
		diags = append(diags, checkStmtSQLInjection(s.CatchBody)...)
	case *MatchStmt:
		for _, c := range s.Cases {
			diags = append(diags, checkStmtSQLInjection(c.Body)...)
		}
	}
	return diags
}

func checkExprSQLInjection(expr Expression) []Diagnostic {
	var diags []Diagnostic
	if expr == nil {
		return diags
	}

	switch e := expr.(type) {
	case *CallExpr:
		// Check if it's db.query(...) or db.querySafe(...) or db.queryPage(...)
		if memExpr, ok := e.Function.(*MemberExpr); ok {
			if ident, ok := memExpr.Object.(*Identifier); ok && ident.Value == "db" {
				if memExpr.Field == "query" || memExpr.Field == "querySafe" || memExpr.Field == "queryPage" {
					if len(e.Arguments) > 0 {
						firstArg := e.Arguments[0]
						isConcatenation := false
						if infix, ok := firstArg.(*InfixExpr); ok && infix.Operator == "+" {
							isConcatenation = true
						}
						_, isFString := firstArg.(*FStringLiteral)
						if isConcatenation || isFString {
							diags = append(diags, Diagnostic{
								Line:     e.Token.Line,
								Col:      e.Token.Col,
								Severity: "error",
								Message:  "SQL injection risk detected. Use placeholders (?) instead of string concatenation/formatting for database query parameters.",
							})
						}
					}
				}
			}
		}

		// Recurse into call arguments
		for _, arg := range e.Arguments {
			diags = append(diags, checkExprSQLInjection(arg)...)
		}

	case *InfixExpr:
		diags = append(diags, checkExprSQLInjection(e.Left)...)
		diags = append(diags, checkExprSQLInjection(e.Right)...)

	case *PrefixExpr:
		diags = append(diags, checkExprSQLInjection(e.Right)...)

	case *IndexExpr:
		diags = append(diags, checkExprSQLInjection(e.Left)...)
		diags = append(diags, checkExprSQLInjection(e.Index)...)

	case *MemberExpr:
		diags = append(diags, checkExprSQLInjection(e.Object)...)

	case *OptionalMemberExpr:
		diags = append(diags, checkExprSQLInjection(e.Object)...)

	case *MapLiteral:
		for _, val := range e.Pairs {
			diags = append(diags, checkExprSQLInjection(val)...)
		}

	case *ArrayLiteral:
		for _, el := range e.Elements {
			diags = append(diags, checkExprSQLInjection(el)...)
		}
	}

	return diags
}
