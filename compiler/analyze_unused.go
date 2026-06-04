package compiler

import "fmt"

// analyzeUnusedVars finds variables declared in a function body that are never referenced.
func analyzeUnusedVars(fn *FnDecl) []Diagnostic {
	var diags []Diagnostic

	// Collect all let declarations in the function
	declared := make(map[string]Token) // name -> declaration token
	for _, stmt := range fn.Body.Statements {
		collectDeclarations(stmt, declared)
	}

	// Collect all identifier references in the function body
	referenced := make(map[string]bool)
	for _, stmt := range fn.Body.Statements {
		collectReferences(stmt, referenced)
	}

	// Also mark function params as declared (but they don't generate warnings)
	paramSet := make(map[string]bool)
	for _, p := range fn.Params {
		paramSet[p] = true
	}

	// Report unused variables (skip params and special names)
	for name, tok := range declared {
		if paramSet[name] {
			continue
		}
		if name == "_" || name == "self" {
			continue
		}
		if !referenced[name] {
			diags = append(diags, Diagnostic{
				Line:     tok.Line,
				Col:      tok.Col,
				Severity: "warning",
				Message:  fmt.Sprintf("variable '%s' is declared but never used", name),
			})
		}
	}

	return diags
}

// analyzeBlock checks for unused variables in a standalone block (routes, etc).
func analyzeBlock(block *BlockStmt, parentParams []string) []Diagnostic {
	var diags []Diagnostic
	if block == nil {
		return diags
	}

	declared := make(map[string]Token)
	for _, stmt := range block.Statements {
		collectDeclarations(stmt, declared)
	}

	referenced := make(map[string]bool)
	for _, stmt := range block.Statements {
		collectReferences(stmt, referenced)
	}

	paramSet := make(map[string]bool)
	for _, p := range parentParams {
		paramSet[p] = true
	}

	for name, tok := range declared {
		if paramSet[name] || name == "_" || name == "self" {
			continue
		}
		if !referenced[name] {
			diags = append(diags, Diagnostic{
				Line:     tok.Line,
				Col:      tok.Col,
				Severity: "warning",
				Message:  fmt.Sprintf("variable '%s' is declared but never used", name),
			})
		}
	}

	return diags
}

// analyzeDeadImports checks for imports whose symbols are never used in the program.
func analyzeDeadImports(program *Program, definedSymbols map[string]bool) []Diagnostic {
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

// collectDeclarations walks a statement tree and records let declarations.
func collectDeclarations(stmt Statement, declared map[string]Token) {
	switch s := stmt.(type) {
	case *LetStmt:
		declared[s.Name] = s.Token
	case *DestructureLetStmt:
		for _, f := range s.Fields {
			declared[f] = s.Token
		}
	case *ForStmt:
		if s.Variable != "" {
			declared[s.Variable] = s.Token
		}
		if s.KeyVar != "" {
			declared[s.KeyVar] = s.Token
		}
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectDeclarations(inner, declared)
			}
		}
	case *IfStmt:
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectDeclarations(inner, declared)
			}
		}
		if s.ElseBody != nil {
			for _, inner := range s.ElseBody.Statements {
				collectDeclarations(inner, declared)
			}
		}
	case *TryCatchStmt:
		if s.TryBody != nil {
			for _, inner := range s.TryBody.Statements {
				collectDeclarations(inner, declared)
			}
		}
		if s.CatchBody != nil {
			declared[s.Param] = s.Token
			for _, inner := range s.CatchBody.Statements {
				collectDeclarations(inner, declared)
			}
		}
	case *MatchStmt:
		for _, c := range s.Cases {
			if c.Body != nil {
				for _, inner := range c.Body.Statements {
					collectDeclarations(inner, declared)
				}
			}
		}
	}
}

// collectReferences walks a statement tree and records all identifier usages.
func collectReferences(stmt Statement, referenced map[string]bool) {
	switch s := stmt.(type) {
	case *LetStmt:
		collectExprRefs(s.Value, referenced)
	case *DestructureLetStmt:
		collectExprRefs(s.Value, referenced)
	case *ReturnStmt:
		collectExprRefs(s.Value, referenced)
	case *ExprStmt:
		collectExprRefs(s.Value, referenced)
	case *IfStmt:
		collectExprRefs(s.Condition, referenced)
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectReferences(inner, referenced)
			}
		}
		if s.ElseBody != nil {
			for _, inner := range s.ElseBody.Statements {
				collectReferences(inner, referenced)
			}
		}
	case *ForStmt:
		collectExprRefs(s.Iterable, referenced)
		if s.Body != nil {
			for _, inner := range s.Body.Statements {
				collectReferences(inner, referenced)
			}
		}
	case *TryCatchStmt:
		if s.TryBody != nil {
			for _, inner := range s.TryBody.Statements {
				collectReferences(inner, referenced)
			}
		}
		if s.CatchBody != nil {
			for _, inner := range s.CatchBody.Statements {
				collectReferences(inner, referenced)
			}
		}
	case *MatchStmt:
		collectExprRefs(s.Value, referenced)
		for _, c := range s.Cases {
			if c.Value != nil {
				collectExprRefs(c.Value, referenced)
			}
			if c.Body != nil {
				for _, inner := range c.Body.Statements {
					collectReferences(inner, referenced)
				}
			}
		}
	case *PublishStmt:
		collectExprRefs(s.Topic, referenced)
		collectExprRefs(s.Value, referenced)
	case *SpawnStmt:
		collectExprRefs(s.Call, referenced)
	}
}

// collectExprRefs recursively collects all identifier references from an expression.
func collectExprRefs(expr Expression, referenced map[string]bool) {
	if expr == nil {
		return
	}
	switch e := expr.(type) {
	case *Identifier:
		referenced[e.Value] = true
	case *AssignExpr:
		referenced[e.Name] = true
		collectExprRefs(e.Value, referenced)
	case *CompoundAssignExpr:
		referenced[e.Name] = true
		collectExprRefs(e.Value, referenced)
	case *MemberExpr:
		collectExprRefs(e.Object, referenced)
	case *OptionalMemberExpr:
		collectExprRefs(e.Object, referenced)
	case *MemberAssignExpr:
		collectExprRefs(e.Object, referenced)
		collectExprRefs(e.Value, referenced)
	case *CallExpr:
		collectExprRefs(e.Function, referenced)
		for _, arg := range e.Arguments {
			collectExprRefs(arg, referenced)
		}
	case *InfixExpr:
		collectExprRefs(e.Left, referenced)
		collectExprRefs(e.Right, referenced)
	case *IndexExpr:
		collectExprRefs(e.Left, referenced)
		collectExprRefs(e.Index, referenced)
	case *SliceExpr:
		collectExprRefs(e.Left, referenced)
		collectExprRefs(e.Start, referenced)
		collectExprRefs(e.End, referenced)
	case *ArrayLiteral:
		for _, el := range e.Elements {
			collectExprRefs(el, referenced)
		}
	case *MapLiteral:
		for _, v := range e.Pairs {
			collectExprRefs(v, referenced)
		}
		for _, s := range e.Spreads {
			collectExprRefs(s.Value, referenced)
		}
	case *StructLiteral:
		for _, v := range e.Fields {
			collectExprRefs(v, referenced)
		}
	case *FnLiteral:
		if e.IsArrow {
			collectExprRefs(e.ArrowExpr, referenced)
		} else if e.Body != nil {
			for _, s := range e.Body.Statements {
				collectReferences(s, referenced)
			}
		}
	case *AwaitExpr:
		collectExprRefs(e.Value, referenced)
	case *AssertExpr:
		collectExprRefs(e.Cond, referenced)
	case *ErrorPropExpr:
		collectExprRefs(e.Value, referenced)
	case *FStringLiteral:
		collectFStringRefs(e.Value, referenced)
	case *SelfExpr:
		referenced["self"] = true
	}
}

// collectFStringRefs extracts identifier references from f-string interpolation braces.
func collectFStringRefs(str string, referenced map[string]bool) {
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
				collectExprRefs(expr, referenced)
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
	}
}
