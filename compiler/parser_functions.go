package compiler

func (p *Parser) parseFnDeclaration() Statement {
	fnToken := p.curToken

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	firstName := p.curToken.Literal

	// Check if this is a method declaration: fn TypeName.methodName(...)
	if p.peekToken.Type == TOKEN_DOT {
		p.nextToken() // consume '.'
		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		methodName := p.curToken.Literal
		return p.parseMethodDeclaration(fnToken, firstName, methodName)
	}

	// Regular function declaration
	stmt := &FnDecl{Token: fnToken}
	stmt.Name = firstName

	// Optional type parameters: fn name[T, U](...) or fn name[T: Comparable, U: Numeric](...)
	if p.peekToken.Type == TOKEN_LBRACKET {
		p.nextToken() // consume '['
		stmt.TypeParams = []string{}
		stmt.TypeConstraints = []string{}
		for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
			p.nextToken()
			if p.curToken.Type == TOKEN_IDENT {
				stmt.TypeParams = append(stmt.TypeParams, p.curToken.Literal)
				// Check for constraint: T: Comparable
				if p.peekToken.Type == TOKEN_COLON {
					p.nextToken() // consume ':'
					p.nextToken() // constraint name
					stmt.TypeConstraints = append(stmt.TypeConstraints, p.curToken.Literal)
				} else {
					stmt.TypeConstraints = append(stmt.TypeConstraints, "")
				}
			}
			if p.peekToken.Type == TOKEN_COMMA {
				p.nextToken()
			}
		}
		if !p.expectPeek(TOKEN_RBRACKET) {
			return nil
		}
	}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	stmt.Params = []string{}
	stmt.ParamTypes = []string{}
	if p.peekToken.Type != TOKEN_RPAREN {
		p.nextToken()
		stmt.Params = append(stmt.Params, p.curToken.Literal)
		// possible type after param
		if p.peekToken.Type == TOKEN_COLON {
			p.nextToken() // skip ':'
			p.nextToken() // type identifier
			stmt.ParamTypes = append(stmt.ParamTypes, p.parseTypeAnnotation())
		} else {
			stmt.ParamTypes = append(stmt.ParamTypes, "")
		}

		for p.peekToken.Type == TOKEN_COMMA {
			p.nextToken() // comma
			p.nextToken() // identifier
			stmt.Params = append(stmt.Params, p.curToken.Literal)
			if p.peekToken.Type == TOKEN_COLON {
				p.nextToken() // skip ':'
				p.nextToken() // type identifier
				stmt.ParamTypes = append(stmt.ParamTypes, p.parseTypeAnnotation())
			} else {
				stmt.ParamTypes = append(stmt.ParamTypes, "")
			}
		}
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	// Optional return type: fn foo() -> int { ... }
	if p.peekToken.Type == TOKEN_RET_ARROW {
		p.nextToken() // skip '->'
		p.nextToken() // type identifier
		stmt.ReturnType = p.parseTypeAnnotation()
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseMethodDeclaration(fnToken Token, typeName, methodName string) Statement {
	stmt := &MethodDecl{Token: fnToken, TypeName: typeName, Name: methodName}

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
			p.nextToken() // type identifier
			stmt.ParamTypes = append(stmt.ParamTypes, p.parseTypeAnnotation())
		} else {
			stmt.ParamTypes = append(stmt.ParamTypes, "")
		}

		for p.peekToken.Type == TOKEN_COMMA {
			p.nextToken()
			p.nextToken()
			stmt.Params = append(stmt.Params, p.curToken.Literal)
			if p.peekToken.Type == TOKEN_COLON {
				p.nextToken()
				p.nextToken()
				stmt.ParamTypes = append(stmt.ParamTypes, p.parseTypeAnnotation())
			} else {
				stmt.ParamTypes = append(stmt.ParamTypes, "")
			}
		}
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if p.peekToken.Type == TOKEN_RET_ARROW {
		p.nextToken() // skip '->'
		p.nextToken() // type identifier
		stmt.ReturnType = p.parseTypeAnnotation()
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()
	return stmt
}

// parseFnLiteral parses: fn(x, y) { body }
func (p *Parser) parseFnLiteral() Expression {
	lit := &FnLiteral{Token: p.curToken}

	if !p.expectPeek(TOKEN_LPAREN) {
		return nil
	}

	lit.Params = []string{}
	lit.ParamTypes = []string{}
	if p.peekToken.Type != TOKEN_RPAREN {
		p.nextToken()
		lit.Params = append(lit.Params, p.curToken.Literal)
		if p.peekToken.Type == TOKEN_COLON {
			p.nextToken()
			p.nextToken()
			lit.ParamTypes = append(lit.ParamTypes, p.parseTypeAnnotation())
		} else {
			lit.ParamTypes = append(lit.ParamTypes, "")
		}
		for p.peekToken.Type == TOKEN_COMMA {
			p.nextToken()
			p.nextToken()
			lit.Params = append(lit.Params, p.curToken.Literal)
			if p.peekToken.Type == TOKEN_COLON {
				p.nextToken()
				p.nextToken()
				lit.ParamTypes = append(lit.ParamTypes, p.parseTypeAnnotation())
			} else {
				lit.ParamTypes = append(lit.ParamTypes, "")
			}
		}
	}

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	lit.Body = p.parseBlockStatement()
	return lit
}

// parseArrowFunction parses: x => expr (left side is the parameter identifier)
func (p *Parser) parseArrowFunction(left Expression) Expression {
	lit := &FnLiteral{Token: p.curToken, IsArrow: true}

	// Left side is the parameter(s)
	if ident, ok := left.(*Identifier); ok {
		lit.Params = []string{ident.Value}
	} else {
		lit.Params = []string{}
	}
	lit.ParamTypes = make([]string, len(lit.Params))

	// Parse the expression after =>
	p.nextToken()
	lit.ArrowExpr = p.parseExpression(LOWEST)

	return lit
}
