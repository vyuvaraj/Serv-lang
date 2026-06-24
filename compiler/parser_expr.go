package compiler

import (
	"fmt"
	"strconv"
)

func (p *Parser) parseExpression(precedence int) Expression {
	prefix := p.prefixParseFns[p.curToken.Type]
	if prefix == nil {
		p.addError(fmt.Sprintf("no prefix parse function for %s found", p.curToken.Type))
		return nil
	}
	leftExp := prefix()

	for p.peekToken.Type != TOKEN_EOF && precedence < p.peekPrecedence() {
		infix := p.infixParseFns[p.peekToken.Type]
		if infix == nil {
			return leftExp
		}

		p.nextToken()
		leftExp = infix(leftExp)
	}

	return leftExp
}

func (p *Parser) parsePrefixExpression() Expression {
	expr := &PrefixExpr{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
	}
	p.nextToken()
	expr.Right = p.parseExpression(PRODUCT) // high precedence so -x binds tightly
	return expr
}

func (p *Parser) parseIdentifier() Expression {
	ident := &Identifier{Token: p.curToken, Value: p.curToken.Literal}

	// Check if this is a generic call: name[Type1, Type2](args...)
	if p.peekToken.Type == TOKEN_LBRACKET {
		if p.isGenericCallAhead() {
			p.nextToken() // consume '['
			typeArgs := []string{}
			for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
				p.nextToken()
				if p.curToken.Type == TOKEN_IDENT {
					typeArgs = append(typeArgs, p.curToken.Literal)
				}
				if p.peekToken.Type == TOKEN_COMMA {
					p.nextToken()
				}
			}
			p.nextToken() // consume ']'
			// Now expect '('
			if p.peekToken.Type == TOKEN_LPAREN {
				p.nextToken() // consume '('
				call := &CallExpr{Token: p.curToken, Function: ident, TypeArgs: typeArgs}
				call.Arguments = p.parseExpressionList(TOKEN_RPAREN)
				return call
			}
			// If no '(' follows, treat as regular index (shouldn't happen with isGenericCallAhead)
		} else if p.isGenericStructLiteralAhead() {
			p.nextToken() // consume '['
			typeArgs := []string{}
			for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
				p.nextToken()
				if p.curToken.Type == TOKEN_IDENT {
					typeArgs = append(typeArgs, p.curToken.Literal)
				}
				if p.peekToken.Type == TOKEN_COMMA {
					p.nextToken()
				}
			}
			p.nextToken() // consume ']'
			// Now expect '{'
			if p.peekToken.Type == TOKEN_LBRACE {
				p.nextToken() // consume '{'
				lit := p.parseStructLiteral(ident.Value).(*StructLiteral)
				lit.TypeArgs = typeArgs
				return lit
			}
		}
	}

	// Check if this is a struct literal: TypeName { field: value, ... }
	if p.peekToken.Type == TOKEN_LBRACE {
		if p.isStructLiteralAhead() {
			p.nextToken() // consume '{'
			return p.parseStructLiteral(ident.Value)
		}
	}

	return ident
}

func (p *Parser) isGenericCallAhead() bool {
	// Clone lexer to look ahead: [IDENT(, IDENT)*](
	lCopy := NewLexer(p.l.input)
	lCopy.position = p.l.position
	lCopy.readPosition = p.l.readPosition
	lCopy.ch = p.l.ch
	lCopy.line = p.l.line
	lCopy.col = p.l.col

	// peekToken is '['. Read tokens until we find ']' or give up
	depth := 0
	for i := 0; i < 20; i++ { // limit lookahead
		t := lCopy.NextToken()
		if t.Type == TOKEN_LBRACKET {
			depth++
		} else if t.Type == TOKEN_RBRACKET {
			if depth > 0 {
				depth--
			} else {
				// Found matching ']', check if '(' follows
				next := lCopy.NextToken()
				return next.Type == TOKEN_LPAREN
			}
		} else if t.Type == TOKEN_EOF {
			return false
		} else if t.Type != TOKEN_IDENT && t.Type != TOKEN_COMMA {
			// If we see anything other than IDENT or COMMA inside brackets, it's not a generic call
			return false
		}
	}
	return false
}

func (p *Parser) isGenericStructLiteralAhead() bool {
	lCopy := NewLexer(p.l.input)
	lCopy.position = p.l.position
	lCopy.readPosition = p.l.readPosition
	lCopy.ch = p.l.ch
	lCopy.line = p.l.line
	lCopy.col = p.l.col

	depth := 0
	for i := 0; i < 20; i++ { // limit lookahead
		t := lCopy.NextToken()
		if t.Type == TOKEN_LBRACKET {
			depth++
		} else if t.Type == TOKEN_RBRACKET {
			if depth > 0 {
				depth--
			} else {
				// Found matching ']', check if '{' follows
				next := lCopy.NextToken()
				return next.Type == TOKEN_LBRACE
			}
		} else if t.Type == TOKEN_EOF {
			return false
		} else if t.Type != TOKEN_IDENT && t.Type != TOKEN_COMMA {
			return false
		}
	}
	return false
}

func (p *Parser) isStructLiteralAhead() bool {
	// Clone lexer state to peek further
	lCopy := NewLexer(p.l.input)
	lCopy.position = p.l.position
	lCopy.readPosition = p.l.readPosition
	lCopy.ch = p.l.ch
	lCopy.line = p.l.line
	lCopy.col = p.l.col

	// peekToken is '{'. After that we expect IDENT COLON (struct literal) or something else
	t1 := lCopy.NextToken() // first token after '{'
	if t1.Type == TOKEN_RBRACE {
		return true // empty struct literal: Type {}
	}
	if t1.Type != TOKEN_IDENT {
		return false
	}
	t2 := lCopy.NextToken()
	return t2.Type == TOKEN_COLON
}

func (p *Parser) parseStructLiteral(typeName string) Expression {
	lit := &StructLiteral{
		Token:    p.curToken,
		TypeName: typeName,
		Fields:   make(map[string]Expression),
		KeyOrder: []string{},
	}

	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken() // field name
		if p.curToken.Type != TOKEN_IDENT {
			p.addError(fmt.Sprintf("expected field name in struct literal, got %s", p.curToken.Type))
			return nil
		}
		fieldName := p.curToken.Literal

		if !p.expectPeek(TOKEN_COLON) {
			return nil
		}

		p.nextToken() // value expression
		val := p.parseExpression(LOWEST)
		lit.Fields[fieldName] = val
		lit.KeyOrder = append(lit.KeyOrder, fieldName)

		if p.peekToken.Type == TOKEN_COMMA {
			p.nextToken()
		}
	}

	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}

	return lit
}

func (p *Parser) parseIntegerLiteral() Expression {
	lit := &IntegerLiteral{Token: p.curToken}

	value, err := strconv.ParseInt(p.curToken.Literal, 0, 64)
	if err != nil {
		p.addError(fmt.Sprintf("could not parse %q as integer", p.curToken.Literal))
		return nil
	}

	lit.Value = value
	return lit
}

func (p *Parser) parseStringLiteral() Expression {
	return &StringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseFloatLiteral() Expression {
	lit := &FloatLiteral{Token: p.curToken}
	value, err := strconv.ParseFloat(p.curToken.Literal, 64)
	if err != nil {
		p.addError(fmt.Sprintf("could not parse %q as float", p.curToken.Literal))
		return nil
	}
	lit.Value = value
	return lit
}

func (p *Parser) parseDurationLiteral() Expression {
	return &DurationLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseArrayLiteral() Expression {
	a := &ArrayLiteral{Token: p.curToken, Elements: []Expression{}}
	if p.peekToken.Type == TOKEN_RBRACKET {
		p.nextToken()
		return a
	}
	p.nextToken() // skip '['
	a.Elements = append(a.Elements, p.parseExpression(LOWEST))
	for p.peekToken.Type == TOKEN_COMMA {
		p.nextToken() // move to comma
		p.nextToken() // move to expression
		a.Elements = append(a.Elements, p.parseExpression(LOWEST))
	}
	if !p.expectPeek(TOKEN_RBRACKET) {
		return nil
	}
	return a
}

func (p *Parser) parseMapLiteral() Expression {
	m := &MapLiteral{Token: p.curToken, Pairs: make(map[string]Expression), KeyOrder: []string{}}

	entryIndex := 0
	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken() // move to key or spread

		// Spread operator: ...expr
		if p.curToken.Type == TOKEN_SPREAD {
			p.nextToken() // move to the expression after '...'
			spreadExpr := p.parseExpression(LOWEST)
			m.Spreads = append(m.Spreads, SpreadEntry{Position: entryIndex, Value: spreadExpr})
			entryIndex++
			if p.peekToken.Type == TOKEN_COMMA {
				p.nextToken()
			}
			continue
		}

		key := p.curToken.Literal

		// If it's a string, we strip quotes (lexer already stripped quotes from TOKEN_STRING literal)
		// We expect either identifier or string for keys
		if p.curToken.Type != TOKEN_STRING && p.curToken.Type != TOKEN_IDENT {
			p.addError(fmt.Sprintf("expected map key, got %s", p.curToken.Type))
			return nil
		}

		if !p.expectPeek(TOKEN_COLON) {
			return nil
		}

		p.nextToken() // skip ':'
		val := p.parseExpression(LOWEST)
		m.Pairs[key] = val
		m.KeyOrder = append(m.KeyOrder, key)
		entryIndex++

		if p.peekToken.Type == TOKEN_COMMA {
			p.nextToken()
		}
	}

	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}

	return m
}

func (p *Parser) parseGroupedExpression() Expression {
	p.nextToken()
	exp := p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_RPAREN) {
		return nil
	}
	return exp
}

func (p *Parser) parseInfixExpression(left Expression) Expression {
	expr := &InfixExpr{
		Token:    p.curToken,
		Operator: p.curToken.Literal,
		Left:     left,
	}

	precedence := p.curPrecedence()
	p.nextToken()
	expr.Right = p.parseExpression(precedence)

	return expr
}

func (p *Parser) parseCallExpression(function Expression) Expression {
	exp := &CallExpr{Token: p.curToken, Function: function}
	exp.Arguments = p.parseExpressionList(TOKEN_RPAREN)
	return exp
}

func (p *Parser) parseExpressionList(end TokenType) []Expression {
	list := []Expression{}

	if p.peekToken.Type == end {
		p.nextToken()
		return list
	}

	p.nextToken()
	list = append(list, p.parseExpression(LOWEST))

	for p.peekToken.Type == TOKEN_COMMA {
		p.nextToken()
		p.nextToken()
		list = append(list, p.parseExpression(LOWEST))
	}

	if !p.expectPeek(end) {
		return nil
	}

	return list
}

func (p *Parser) parseAssertExpression() Expression {
	expr := &AssertExpr{Token: p.curToken}
	p.nextToken()
	expr.Cond = p.parseExpression(LOWEST)
	return expr
}

func (p *Parser) parseMemberExpression(left Expression) Expression {
	expr := &MemberExpr{Token: p.curToken, Object: left}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	expr.Field = p.curToken.Literal

	return expr
}

func (p *Parser) parseOptionalMemberExpression(left Expression) Expression {
	expr := &OptionalMemberExpr{Token: p.curToken, Object: left}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	expr.Field = p.curToken.Literal

	return expr
}

func (p *Parser) parseAssignmentExpression(left Expression) Expression {
	switch l := left.(type) {
	case *Identifier:
		expr := &AssignExpr{Token: p.curToken, Name: l.Value}
		p.nextToken()
		expr.Value = p.parseExpression(LOWEST)
		return expr
	case *MemberExpr:
		expr := &MemberAssignExpr{Token: p.curToken, Object: l.Object, Field: l.Field}
		p.nextToken()
		expr.Value = p.parseExpression(LOWEST)
		return expr
	case *IndexExpr:
		expr := &IndexAssignExpr{Token: p.curToken, Left: l}
		p.nextToken()
		expr.Value = p.parseExpression(LOWEST)
		return expr
	default:
		p.addError("left side of assignment must be an identifier, member expression, or index expression")
		return nil
	}
}

func (p *Parser) parseIndexExpression(left Expression) Expression {
	tok := p.curToken
	p.nextToken() // skip '['

	// Check for slice expression: arr[start:end], arr[:end], arr[start:]
	if p.curToken.Type == TOKEN_COLON {
		// arr[:end]
		p.nextToken()
		var endExpr Expression
		if p.curToken.Type != TOKEN_RBRACKET {
			endExpr = p.parseExpression(LOWEST)
		}
		if !p.expectPeek(TOKEN_RBRACKET) {
			return nil
		}
		return &SliceExpr{Token: tok, Left: left, Start: nil, End: endExpr}
	}

	startOrIndex := p.parseExpression(LOWEST)

	// Check if this is a slice expression
	if p.peekToken.Type == TOKEN_COLON {
		p.nextToken() // consume ':'
		p.nextToken() // move past ':'
		var endExpr Expression
		if p.curToken.Type != TOKEN_RBRACKET {
			endExpr = p.parseExpression(LOWEST)
			if !p.expectPeek(TOKEN_RBRACKET) {
				return nil
			}
		} else {
			// arr[start:] — curToken is already ']'
			// Don't expectPeek, we're already on ']'
		}
		return &SliceExpr{Token: tok, Left: left, Start: startOrIndex, End: endExpr}
	}

	// Regular index expression
	expr := &IndexExpr{Token: tok, Left: left, Index: startOrIndex}
	if !p.expectPeek(TOKEN_RBRACKET) {
		return nil
	}
	return expr
}

func (p *Parser) parseCompoundAssignExpression(left Expression) Expression {
	switch l := left.(type) {
	case *Identifier:
		expr := &CompoundAssignExpr{Token: p.curToken, Name: l.Value, Operator: p.curToken.Literal}
		p.nextToken()
		expr.Value = p.parseExpression(LOWEST)
		return expr
	default:
		p.addError("left side of compound assignment must be an identifier")
		return nil
	}
}

func (p *Parser) parseErrorPropExpression(left Expression) Expression {
	return &ErrorPropExpr{Token: p.curToken, Value: left}
}

func (p *Parser) parseFStringLiteral() Expression {
	return &FStringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseCacheIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseAiIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: "ai"}
}

func (p *Parser) parseMailIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: "mail"}
}

func (p *Parser) parseStoreIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: "store"}
}

func (p *Parser) parseSearchIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: "search"}
}



// parseWorkflowIdentifier allows the `workflow` keyword to be used in
// expression context as an identifier, e.g. workflow.start("Name", param).
func (p *Parser) parseWorkflowIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseBooleanLiteral() Expression {
	return &BooleanLiteral{Token: p.curToken, Value: p.curToken.Type == TOKEN_TRUE}
}

func (p *Parser) parseNilLiteral() Expression {
	return &NilLiteral{Token: p.curToken}
}

func (p *Parser) parseSelfExpression() Expression {
	return &SelfExpr{Token: p.curToken}
}

// parseValidateIdentifier treats 'validate' as a function identifier in expression context.
func (p *Parser) parseValidateIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: "validate"}
}

func (p *Parser) parseSpawnExpression() Expression {
	expr := &SpawnExpr{Token: p.curToken}
	if p.peekToken.Type == TOKEN_LPAREN {
		p.nextToken() // skip 'spawn' and move to '('
		p.nextToken() // skip '('
		expr.Limit = p.parseExpression(LOWEST)
		if !p.expectPeek(TOKEN_RPAREN) {
			return nil
		}
	}
	p.nextToken()
	expr.Call = p.parseExpression(LOWEST)
	return expr
}

func (p *Parser) parseAuthIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: "auth"}
}

func (p *Parser) parsePipeExpression(left Expression) Expression {
	precedence := p.curPrecedence()
	pipeTok := p.curToken
	p.nextToken() // consume |>

	right := p.parseExpression(precedence)
	if right == nil {
		return nil
	}

	if call, ok := right.(*CallExpr); ok {
		call.Arguments = append([]Expression{left}, call.Arguments...)
		return call
	}

	// Otherwise, wrap in CallExpr
	return &CallExpr{
		Token:     Token{Type: TOKEN_LPAREN, Literal: "(", Line: pipeTok.Line, Col: pipeTok.Col},
		Function:  right,
		Arguments: []Expression{left},
	}
}


