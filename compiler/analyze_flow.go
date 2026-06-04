package compiler

// blockHasReturn checks if a block definitely returns on all paths.
func blockHasReturn(block *BlockStmt) bool {
	if block == nil || len(block.Statements) == 0 {
		return false
	}

	lastStmt := block.Statements[len(block.Statements)-1]
	switch s := lastStmt.(type) {
	case *ReturnStmt:
		return true
	case *IfStmt:
		// Both branches must return
		if s.ElseBody == nil {
			return false
		}
		return blockHasReturn(s.Body) && blockHasReturn(s.ElseBody)
	case *MatchStmt:
		// All cases must return, and there must be a default
		hasDefault := false
		for _, c := range s.Cases {
			if c.Value == nil {
				hasDefault = true
			}
			if !blockHasReturn(c.Body) {
				return false
			}
		}
		return hasDefault
	}

	// Check if any earlier statement is a return (unreachable code after it, but still returns)
	for _, s := range block.Statements {
		if _, ok := s.(*ReturnStmt); ok {
			return true
		}
	}

	return false
}

// analyzeUnreachableCode detects statements that appear after return/break/continue.
func analyzeUnreachableCode(block *BlockStmt) []Diagnostic {
	var diags []Diagnostic
	if block == nil {
		return diags
	}

	terminated := false
	for _, stmt := range block.Statements {
		if terminated {
			tok := stmtToken(stmt)
			if tok.Line > 0 {
				diags = append(diags, Diagnostic{
					Line:     tok.Line,
					Col:      tok.Col,
					Severity: "warning",
					Message:  "unreachable code after return/break/continue",
				})
			}
			break // Only report the first unreachable statement
		}

		switch stmt.(type) {
		case *ReturnStmt:
			terminated = true
		case *BreakStmt:
			terminated = true
		case *ContinueStmt:
			terminated = true
		}

		// Recurse into nested blocks
		switch s := stmt.(type) {
		case *IfStmt:
			diags = append(diags, analyzeUnreachableCode(s.Body)...)
			if s.ElseBody != nil {
				diags = append(diags, analyzeUnreachableCode(s.ElseBody)...)
			}
		case *ForStmt:
			diags = append(diags, analyzeUnreachableCode(s.Body)...)
		case *TryCatchStmt:
			diags = append(diags, analyzeUnreachableCode(s.TryBody)...)
			diags = append(diags, analyzeUnreachableCode(s.CatchBody)...)
		case *MatchStmt:
			for _, c := range s.Cases {
				diags = append(diags, analyzeUnreachableCode(c.Body)...)
			}
		}
	}

	return diags
}
