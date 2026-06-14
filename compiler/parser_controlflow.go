package compiler

// parseIfStatement parses: if <condition> { ... } else { ... }
func (p *Parser) parseIfStatement() Statement {
	stmt := &IfStmt{Token: p.curToken}

	p.nextToken()
	stmt.Condition = p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	// Check for else
	if p.peekToken.Type == TOKEN_ELSE {
		p.nextToken() // consume 'else'

		// else if ...
		if p.peekToken.Type == TOKEN_IF {
			p.nextToken() // consume 'if'
			elseIfStmt := p.parseIfStatement()
			if elseIfStmt != nil {
				stmt.ElseBody = &BlockStmt{
					Token:      p.curToken,
					Statements: []Statement{elseIfStmt},
				}
			}
		} else {
			if !p.expectPeek(TOKEN_LBRACE) {
				return nil
			}
			stmt.ElseBody = p.parseBlockStatement()
		}
	}

	return stmt
}

// parseForStatement parses: for <var> in <expr> { ... } or for <condition> { ... }
// Also: for key, value in map { ... }
func (p *Parser) parseForStatement() Statement {
	stmt := &ForStmt{Token: p.curToken}

	p.nextToken()

	// Check if this is "for x in collection" or "for k, v in collection" pattern
	if p.curToken.Type == TOKEN_IDENT {
		switch p.peekToken.Type {
		case TOKEN_IN:
			// for x in collection
			stmt.Variable = p.curToken.Literal
			stmt.IsRange = true
			p.nextToken() // skip 'in'
			p.nextToken() // move to iterable expression
			stmt.Iterable = p.parseExpression(LOWEST)
		case TOKEN_COMMA:
			// for key, value in collection
			stmt.KeyVar = p.curToken.Literal
			p.nextToken() // skip ','
			if !p.expectPeek(TOKEN_IDENT) {
				return nil
			}
			stmt.Variable = p.curToken.Literal
			stmt.IsRange = true
			if !p.expectPeek(TOKEN_IN) {
				return nil
			}
			p.nextToken() // move to iterable expression
			stmt.Iterable = p.parseExpression(LOWEST)
		default:
			// Condition-based loop: for <condition> { ... }
			stmt.IsRange = false
			stmt.Iterable = p.parseExpression(LOWEST)
		}
	} else {
		// Condition-based loop starting with non-identifier
		stmt.IsRange = false
		stmt.Iterable = p.parseExpression(LOWEST)
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseMatchStatement() Statement {
	stmt := &MatchStmt{Token: p.curToken}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Cases = []MatchCase{}
	p.nextToken() // skip '{'

	for p.curToken.Type != TOKEN_RBRACE && p.curToken.Type != TOKEN_EOF {
		var c MatchCase
		if p.curToken.Type == TOKEN_IDENT && p.curToken.Literal == "_" {
			c.Value = nil
			p.nextToken() // skip "_"
		} else {
			// Parse at ASSIGN precedence so we stop before => (which has ASSIGN precedence)
			c.Value = p.parseExpression(ASSIGN)
			p.nextToken()
		}

		if p.curToken.Type != TOKEN_ARROW {
			p.addError("expected => after match case value")
			return nil
		}
		p.nextToken() // skip "=>"

		if p.curToken.Type != TOKEN_LBRACE {
			p.addError("expected { after match case arrow")
			return nil
		}

		c.Body = p.parseBlockStatement()
		stmt.Cases = append(stmt.Cases, c)
		p.nextToken() // skip '}'
	}

	return stmt
}

func (p *Parser) parseTryCatchStatement() Statement {
	stmt := &TryCatchStmt{Token: p.curToken}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.TryBody = p.parseBlockStatement()

	if !p.expectPeek(TOKEN_CATCH) {
		return nil
	}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Param = p.curToken.Literal

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.CatchBody = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseAwaitExpression() Expression {
	expr := &AwaitExpr{Token: p.curToken}
	p.nextToken()
	expr.Value = p.parseExpression(LOWEST)
	return expr
}
