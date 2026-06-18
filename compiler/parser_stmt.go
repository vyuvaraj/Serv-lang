package compiler

func (p *Parser) parseImportStatement() Statement {
	// Check for Go package import: import alias from "go/package/path"
	if p.peekToken.Type == TOKEN_IDENT {
		alias := p.peekToken.Literal
		p.nextToken() // consume alias
		if !p.expectPeek(TOKEN_FROM) {
			return nil
		}
		if !p.expectPeek(TOKEN_STRING) {
			return nil
		}
		return &GoPackageImport{Token: p.curToken, Alias: alias, Path: p.curToken.Literal}
	}

	stmt := &ImportStmt{Token: p.curToken.Type}

	// Check for named import: import { Name1, Name2 } from "path"
	if p.peekToken.Type == TOKEN_LBRACE {
		p.nextToken() // consume '{'
		stmt.Names = []string{}
		for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
			p.nextToken()
			if p.curToken.Type == TOKEN_IDENT {
				stmt.Names = append(stmt.Names, p.curToken.Literal)
			}
			if p.peekToken.Type == TOKEN_COMMA {
				p.nextToken()
			}
		}
		if !p.expectPeek(TOKEN_RBRACE) {
			return nil
		}
		if !p.expectPeek(TOKEN_FROM) {
			return nil
		}
		if !p.expectPeek(TOKEN_STRING) {
			return nil
		}
		stmt.Path = p.curToken.Literal
	} else {
		// Simple import: import "path"
		if !p.expectPeek(TOKEN_STRING) {
			return nil
		}
		stmt.Path = p.curToken.Literal
	}

	return stmt
}

func (p *Parser) parseExternStatement() Statement {
	stmt := &ExternFnStmt{Token: p.curToken}

	if !p.expectPeek(TOKEN_FN) {
		return nil
	}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	stmt.Params = []string{}
	if p.peekToken.Type != TOKEN_RPAREN {
		p.nextToken()
		stmt.Params = append(stmt.Params, p.curToken.Literal)

		for p.peekToken.Type == TOKEN_COMMA {
			p.nextToken() // move to comma
			p.nextToken() // move to param
			stmt.Params = append(stmt.Params, p.curToken.Literal)
		}
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_FROM) {
		return nil
	}

	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.Source = p.curToken.Literal

	return stmt
}

func (p *Parser) parseLetStatement() Statement {
	letToken := p.curToken

	// Destructuring: let { field1, field2 } = expr
	if p.peekToken.Type == TOKEN_LBRACE {
		p.nextToken() // consume '{'
		stmt := &DestructureLetStmt{Token: letToken}
		stmt.Fields = []string{}
		for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
			p.nextToken()
			if p.curToken.Type == TOKEN_IDENT {
				stmt.Fields = append(stmt.Fields, p.curToken.Literal)
			}
			if p.peekToken.Type == TOKEN_COMMA {
				p.nextToken()
			}
		}
		if !p.expectPeek(TOKEN_RBRACE) {
			return nil
		}
		if !p.expectPeek(TOKEN_ASSIGN) {
			return nil
		}
		p.nextToken()
		stmt.Value = p.parseExpression(LOWEST)
		return stmt
	}

	stmt := &LetStmt{Token: letToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal
	stmt.Names = []string{stmt.Name}

	// Check for multi-return: let val, err = expr
	for p.peekToken.Type == TOKEN_COMMA {
		p.nextToken() // consume ','
		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		stmt.Names = append(stmt.Names, p.curToken.Literal)
	}

	// Optional type annotation after name: let x : int = 5
	// (only for single-name lets)
	if p.peekToken.Type == TOKEN_COLON && len(stmt.Names) == 1 {
		p.nextToken() // skip ':'
		p.nextToken() // type identifier
		stmt.Type = p.parseTypeAnnotation()
	}

	if !p.expectPeek(TOKEN_ASSIGN) {
		return nil
	}

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	return stmt
}

func (p *Parser) parseReturnStatement() Statement {
	stmt := &ReturnStmt{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	return stmt
}

func (p *Parser) parseYieldStatement() Statement {
	stmt := &YieldStmt{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	return stmt
}


// parseTestStatement parses a test block: test name { ... } or test "description" { ... }
// Also supports: test "name" timeout 5s { ... }
func (p *Parser) parseTestStatement() Statement {
	stmt := &TestStmt{Token: p.curToken}

	// Accept either an identifier or a quoted string as the test name
	if p.peekToken.Type == TOKEN_IDENT || p.peekToken.Type == TOKEN_STRING {
		p.nextToken()
		stmt.Name = p.curToken.Literal
	} else {
		p.peekError(TOKEN_IDENT)
		return nil
	}

	// Optional timeout: test "name" timeout 5s { ... }
	if p.peekToken.Type == TOKEN_TIMEOUT {
		p.nextToken() // consume 'timeout'
		switch p.peekToken.Type {
		case TOKEN_DURATION:
			p.nextToken()
			stmt.Timeout = p.curToken.Literal
		case TOKEN_INT:
			p.nextToken()
			// Assume seconds if just a number
			stmt.Timeout = p.curToken.Literal + "s"
		default:
			p.nextToken()
			stmt.Timeout = p.curToken.Literal
		}
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parsePublishStatement() Statement {
	stmt := &PublishStmt{Token: p.curToken}
	p.nextToken()
	stmt.Topic = p.parseExpression(LOWEST)

	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	return stmt
}

func (p *Parser) parseSpawnStatement() Statement {
	stmt := &SpawnStmt{Token: p.curToken}
	if p.peekToken.Type == TOKEN_LPAREN {
		p.nextToken() // skip 'spawn' and move to '('
		p.nextToken() // skip '('
		stmt.Limit = p.parseExpression(LOWEST)
		if !p.expectPeek(TOKEN_RPAREN) {
			return nil
		}
	}
	p.nextToken()
	stmt.Call = p.parseExpression(LOWEST)
	return stmt
}

func (p *Parser) parseBeforeEachStatement() Statement {
	stmt := &BeforeEachStmt{Token: p.curToken}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseAfterEachStatement() Statement {
	stmt := &AfterEachStmt{Token: p.curToken}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

// mock targetCall { body }
func (p *Parser) parseMockStatement() Statement {
	stmt := &MockStmt{Token: p.curToken}
	p.nextToken() // consume 'mock'
	stmt.Target = p.parseExpression(LOWEST)
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseActorDeclaration() Statement {
	stmt := &ActorDecl{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	stmt.Params = []string{}
	stmt.ParamTypes = []string{}
	if p.peekToken.Type != TOKEN_RPAREN {
		p.nextToken()
		stmt.Params = append(stmt.Params, p.curToken.Literal)
		if p.peekToken.Type == TOKEN_COLON {
			p.nextToken() // skip ':'
			p.nextToken() // type
			stmt.ParamTypes = append(stmt.ParamTypes, p.parseTypeAnnotation())
		} else {
			stmt.ParamTypes = append(stmt.ParamTypes, "")
		}

		for p.peekToken.Type == TOKEN_COMMA {
			p.nextToken() // comma
			p.nextToken() // ident
			stmt.Params = append(stmt.Params, p.curToken.Literal)
			if p.peekToken.Type == TOKEN_COLON {
				p.nextToken() // skip ':'
				p.nextToken() // type
				stmt.ParamTypes = append(stmt.ParamTypes, p.parseTypeAnnotation())
			} else {
				stmt.ParamTypes = append(stmt.ParamTypes, "")
			}
		}
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseWorkflowDeclaration() Statement {
	stmt := &WorkflowDecl{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	if p.peekToken.Type != TOKEN_RPAREN {
		p.nextToken()
		stmt.Param = p.curToken.Literal
		if p.peekToken.Type == TOKEN_COLON {
			p.nextToken() // skip ':'
			p.nextToken() // skip type (or use it if needed)
			p.parseTypeAnnotation()
		}
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()
	return stmt
}

