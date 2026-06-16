package compiler

import "fmt"

// analyzeUnusedVars finds variables declared in a function body that are never referenced.
func analyzeUnusedVars(fn *FnDecl) []Diagnostic {
	// Root scope of the function parameters
	rootScope := NewScope(nil)
	for _, p := range fn.Params {
		rootScope.Insert(&Symbol{
			Name:  p,
			Type:  "parameter",
			Token: fn.Token,
			Used:  true, // Parameters don't generate warnings
		})
	}

	var diags []Diagnostic
	if fn.Body != nil {
		for _, stmt := range fn.Body.Statements {
			analyzeStmtUnused(stmt, rootScope, &diags)
		}
	}

	collectUnusedDiagnostics(rootScope, &diags)
	return diags
}

// analyzeBlock checks for unused variables in a standalone block (routes, etc).
func analyzeBlock(block *BlockStmt, parentParams []string) []Diagnostic {
	var diags []Diagnostic
	if block == nil {
		return diags
	}

	rootScope := NewScope(nil)
	for _, p := range parentParams {
		rootScope.Insert(&Symbol{
			Name:  p,
			Type:  "parameter",
			Token: Token{},
			Used:  true,
		})
	}

	for _, stmt := range block.Statements {
		analyzeStmtUnused(stmt, rootScope, &diags)
	}

	collectUnusedDiagnostics(rootScope, &diags)
	return diags
}

// analyzeDeadImports checks for imports whose symbols are never used in the program.
func analyzeDeadImports(program *Program) []Diagnostic {
	var diags []Diagnostic

	// Collect all identifier references in the program (outside of import statements)
	allRefs := make(map[string]bool)
	for _, stmt := range program.Statements {
		switch stmt.(type) {
		case *ImportStmt, *GoPackageImport, *DeclareModuleStmt:
			continue
		}
		collectStmtIdentifiers(stmt, allRefs)
	}

	// Check each import
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *ImportStmt:
			if len(s.Names) > 0 {
				for _, name := range s.Names {
					if !allRefs[name] {
						diags = append(diags, Diagnostic{
							Line:     0,
							Col:      0,
							Severity: "warning",
							Message:  fmt.Sprintf("imported symbol '%s' from \"%s\" is never used", name, s.Path),
						})
					}
				}
			}
		case *GoPackageImport:
			if !allRefs[s.Alias] {
				diags = append(diags, Diagnostic{
					Line:     s.Token.Line,
					Col:      s.Token.Col,
					Severity: "warning",
					Message:  fmt.Sprintf("imported package '%s' is never used", s.Alias),
				})
			}
		}
	}

	return diags
}

// collectUnusedDiagnostics recursively walks the scope tree and collects warnings for unused variables.
func collectUnusedDiagnostics(scope *Scope, diags *[]Diagnostic) {
	if scope == nil {
		return
	}
	for _, sym := range scope.Symbols {
		if sym.Type == "variable" && !sym.Used && sym.Name != "_" && sym.Name != "self" {
			*diags = append(*diags, Diagnostic{
				Line:     sym.Token.Line,
				Col:      sym.Token.Col,
				Severity: "warning",
				Message:  fmt.Sprintf("variable '%s' is declared but never used", sym.Name),
			})
		}
	}
	for _, child := range scope.Children {
		collectUnusedDiagnostics(child, diags)
	}
}

// analyzeStmtUnused walks the statement tree, declaring variables in the current scope level
// and pushing child scopes for nested blocks.
func analyzeStmtUnused(stmt Statement, scope *Scope, diags *[]Diagnostic) {
	if stmt == nil {
		return
	}
	switch s := stmt.(type) {
	case *LetStmt:
		collectRefsInExpr(s.Value, scope)
		sym := &Symbol{
			Name:  s.Name,
			Type:  "variable",
			Token: s.Token,
		}
		if !scope.Insert(sym) {
			scope.Symbols[s.Name] = sym // shadow/overwrite
		}
	case *DestructureLetStmt:
		collectRefsInExpr(s.Value, scope)
		for _, f := range s.Fields {
			sym := &Symbol{
				Name:  f,
				Type:  "variable",
				Token: s.Token,
			}
			scope.Symbols[f] = sym // shadow/overwrite
		}
	case *ReturnStmt:
		collectRefsInExpr(s.Value, scope)
	case *ExprStmt:
		collectRefsInExpr(s.Value, scope)
	case *IfStmt:
		collectRefsInExpr(s.Condition, scope)
		if s.Body != nil {
			child := NewScope(scope)
			for _, inner := range s.Body.Statements {
				analyzeStmtUnused(inner, child, diags)
			}
		}
		if s.ElseBody != nil {
			child := NewScope(scope)
			for _, inner := range s.ElseBody.Statements {
				analyzeStmtUnused(inner, child, diags)
			}
		}
	case *ForStmt:
		collectRefsInExpr(s.Iterable, scope)
		child := NewScope(scope)
		if s.Variable != "" {
			child.Insert(&Symbol{
				Name:  s.Variable,
				Type:  "variable",
				Token: s.Token,
				Used:  true, // loop variable
			})
		}
		if s.KeyVar != "" {
			child.Insert(&Symbol{
				Name:  s.KeyVar,
				Type:  "variable",
				Token: s.Token,
				Used:  true, // loop key variable
			})
		}
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				analyzeStmtUnused(inner, child, diags)
			}
		}
	case *TryCatchStmt:
		if s.TryBody != nil {
			child := NewScope(scope)
			for _, inner := range s.TryBody.Statements {
				analyzeStmtUnused(inner, child, diags)
			}
		}
		if s.CatchBody != nil {
			child := NewScope(scope)
			if s.Param != "" {
				child.Insert(&Symbol{
					Name:  s.Param,
					Type:  "parameter",
					Token: s.Token,
					Used:  true,
				})
			}
			for _, inner := range s.CatchBody.Statements {
				analyzeStmtUnused(inner, child, diags)
			}
		}
	case *MatchStmt:
		collectRefsInExpr(s.Value, scope)
		for _, c := range s.Cases {
			child := NewScope(scope)
			if c.Value != nil {
				collectRefsInExpr(c.Value, child)
			}
			if c.Body != nil {
				for _, inner := range c.Body.Statements {
					analyzeStmtUnused(inner, child, diags)
				}
			}
		}
	case *PublishStmt:
		collectRefsInExpr(s.Topic, scope)
		collectRefsInExpr(s.Value, scope)
	case *SpawnStmt:
		collectRefsInExpr(s.Call, scope)
	case *ActorDecl:
		if s.Body != nil {
			child := NewScope(scope)
			for _, p := range s.Params {
				child.Insert(&Symbol{
					Name:  p,
					Type:  "parameter",
					Token: s.Token,
					Used:  true,
				})
			}
			for _, inner := range s.Body.Statements {
				analyzeStmtUnused(inner, child, diags)
			}
		}
	}
}

// collectRefsInExpr recursively traverses an expression tree and marks any referenced symbols as used.
func collectRefsInExpr(expr Expression, scope *Scope) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *Identifier:
		if sym, found := scope.Lookup(e.Value); found {
			sym.Used = true
		}
	case *AssignExpr:
		if sym, found := scope.Lookup(e.Name); found {
			sym.Used = true
		}
		collectRefsInExpr(e.Value, scope)
	case *CompoundAssignExpr:
		if sym, found := scope.Lookup(e.Name); found {
			sym.Used = true
		}
		collectRefsInExpr(e.Value, scope)
	case *MemberExpr:
		collectRefsInExpr(e.Object, scope)
	case *OptionalMemberExpr:
		collectRefsInExpr(e.Object, scope)
	case *MemberAssignExpr:
		collectRefsInExpr(e.Object, scope)
		collectRefsInExpr(e.Value, scope)
	case *CallExpr:
		collectRefsInExpr(e.Function, scope)
		for _, arg := range e.Arguments {
			collectRefsInExpr(arg, scope)
		}
	case *InfixExpr:
		collectRefsInExpr(e.Left, scope)
		collectRefsInExpr(e.Right, scope)
	case *PrefixExpr:
		collectRefsInExpr(e.Right, scope)
	case *IndexExpr:
		collectRefsInExpr(e.Left, scope)
		collectRefsInExpr(e.Index, scope)
	case *SliceExpr:
		collectRefsInExpr(e.Left, scope)
		collectRefsInExpr(e.Start, scope)
		collectRefsInExpr(e.End, scope)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			collectRefsInExpr(el, scope)
		}
	case *MapLiteral:
		for _, v := range e.Pairs {
			collectRefsInExpr(v, scope)
		}
		for _, s := range e.Spreads {
			collectRefsInExpr(s.Value, scope)
		}
	case *StructLiteral:
		for _, v := range e.Fields {
			collectRefsInExpr(v, scope)
		}
	case *FnLiteral:
		child := NewScope(scope)
		for _, p := range e.Params {
			child.Insert(&Symbol{
				Name:  p,
				Type:  "parameter",
				Token: e.Token,
				Used:  true,
			})
		}
		if e.IsArrow {
			collectRefsInExpr(e.ArrowExpr, child)
		} else if e.Body != nil {
			for _, s := range e.Body.Statements {
				analyzeStmtUnused(s, child, nil)
			}
		}
	case *AwaitExpr:
		collectRefsInExpr(e.Value, scope)
	case *AssertExpr:
		collectRefsInExpr(e.Cond, scope)
	case *ErrorPropExpr:
		collectRefsInExpr(e.Value, scope)
	case *FStringLiteral:
		collectFStringRefsInUnused(e.Value, scope)
	case *SelfExpr:
		if sym, found := scope.Lookup("self"); found {
			sym.Used = true
		}
	case *SpawnExpr:
		collectRefsInExpr(e.Call, scope)
	}
}

// collectFStringRefsInUnused parses f-string interpolations and marks any referenced variables as used.
func collectFStringRefsInUnused(str string, scope *Scope) {
	runes := []rune(str)
	i := 0
	for i < len(runes) {
		if runes[i] == '{' {
			i++
			exprText := ""
			for i < len(runes) && runes[i] != '}' {
				exprText += string(runes[i])
				i++
			}
			if i < len(runes) {
				i++
			}
			lexer := NewLexer(exprText)
			parser := NewParser(lexer)
			expr := parser.parseExpression(LOWEST)
			if expr != nil {
				collectRefsInExpr(expr, scope)
			}
		} else {
			i++
		}
	}
}

// collectStmtIdentifiers collects all identifier names referenced in a statement tree.
// Used by dead import detection to find all symbol references in the program.
func collectStmtIdentifiers(stmt Statement, refs map[string]bool) {
	switch s := stmt.(type) {
	case *LetStmt:
		collectExprIdentifiers(s.Value, refs)
	case *DestructureLetStmt:
		collectExprIdentifiers(s.Value, refs)
	case *ReturnStmt:
		collectExprIdentifiers(s.Value, refs)
	case *ExprStmt:
		collectExprIdentifiers(s.Value, refs)
	case *FnDecl:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *MethodDecl:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *RouteStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *EveryStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *CronStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *SubscribeStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *MiddlewareDecl:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *TestStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *WsStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *IfStmt:
		collectExprIdentifiers(s.Condition, refs)
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
		if s.ElseBody != nil {
			for _, inner := range s.ElseBody.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *ForStmt:
		collectExprIdentifiers(s.Iterable, refs)
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *TryCatchStmt:
		if s.TryBody != nil {
			for _, inner := range s.TryBody.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
		if s.CatchBody != nil {
			for _, inner := range s.CatchBody.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *MatchStmt:
		collectExprIdentifiers(s.Value, refs)
		for _, c := range s.Cases {
			if c.Body != nil {
				for _, inner := range c.Body.Statements {
					collectStmtIdentifiers(inner, refs)
				}
			}
		}
	case *PublishStmt:
		collectExprIdentifiers(s.Topic, refs)
		collectExprIdentifiers(s.Value, refs)
	case *SpawnStmt:
		collectExprIdentifiers(s.Call, refs)
	case *ActorDecl:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectStmtIdentifiers(inner, refs)
			}
		}
	case *ExportStmt:
		collectStmtIdentifiers(s.Inner, refs)
	}
}

// collectExprIdentifiers collects all identifier names from an expression.
func collectExprIdentifiers(expr Expression, refs map[string]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *Identifier:
		refs[e.Value] = true
	case *CallExpr:
		collectExprIdentifiers(e.Function, refs)
		for _, arg := range e.Arguments {
			collectExprIdentifiers(arg, refs)
		}
	case *MemberExpr:
		collectExprIdentifiers(e.Object, refs)
	case *OptionalMemberExpr:
		collectExprIdentifiers(e.Object, refs)
	case *InfixExpr:
		collectExprIdentifiers(e.Left, refs)
		collectExprIdentifiers(e.Right, refs)
	case *PrefixExpr:
		collectExprIdentifiers(e.Right, refs)
	case *IndexExpr:
		collectExprIdentifiers(e.Left, refs)
		collectExprIdentifiers(e.Index, refs)
	case *SliceExpr:
		collectExprIdentifiers(e.Left, refs)
		collectExprIdentifiers(e.Start, refs)
		collectExprIdentifiers(e.End, refs)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			collectExprIdentifiers(el, refs)
		}
	case *MapLiteral:
		for _, v := range e.Pairs {
			collectExprIdentifiers(v, refs)
		}
		for _, s := range e.Spreads {
			collectExprIdentifiers(s.Value, refs)
		}
	case *StructLiteral:
		refs[e.TypeName] = true
		for _, v := range e.Fields {
			collectExprIdentifiers(v, refs)
		}
	case *AssignExpr:
		refs[e.Name] = true
		collectExprIdentifiers(e.Value, refs)
	case *CompoundAssignExpr:
		refs[e.Name] = true
		collectExprIdentifiers(e.Value, refs)
	case *MemberAssignExpr:
		collectExprIdentifiers(e.Object, refs)
		collectExprIdentifiers(e.Value, refs)
	case *FnLiteral:
		if e.IsArrow {
			collectExprIdentifiers(e.ArrowExpr, refs)
		} else if e.Body != nil {
			for _, s := range e.Body.Statements {
				collectStmtIdentifiers(s, refs)
			}
		}
	case *AwaitExpr:
		collectExprIdentifiers(e.Value, refs)
	case *AssertExpr:
		collectExprIdentifiers(e.Cond, refs)
	case *ErrorPropExpr:
		collectExprIdentifiers(e.Value, refs)
	case *SpawnExpr:
		collectExprIdentifiers(e.Call, refs)
	}
}
