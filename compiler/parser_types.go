package compiler

import (
	"fmt"
	"strings"
)

// parseStructDeclaration parses: struct Name { field: type, ... }
func (p *Parser) parseStructDeclaration() Statement {
	stmt := &StructDecl{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	// Optional type parameters: struct Box[T] { ... } or struct Pair[T, U] { ... }
	if p.peekToken.Type == TOKEN_LBRACKET {
		p.nextToken() // consume '['
		stmt.TypeParams = []string{}
		stmt.TypeConstraints = []string{}
		for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
			p.nextToken()
			if p.curToken.Type == TOKEN_IDENT {
				stmt.TypeParams = append(stmt.TypeParams, p.curToken.Literal)
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

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Fields = []StructField{}
	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken()
		if p.curToken.Type != TOKEN_IDENT && !isKeywordToken(p.curToken.Type) {
			p.addError(fmt.Sprintf("expected field name in struct, got %s", p.curToken.Type))
			return nil
		}
		p.curToken.Type = TOKEN_IDENT
		fieldName := p.curToken.Literal

		if !p.expectPeek(TOKEN_COLON) {
			return nil
		}
		p.nextToken() // type identifier
		fieldType := p.parseTypeAnnotation()

		stmt.Fields = append(stmt.Fields, StructField{Name: fieldName, Type: fieldType})

		// Optional comma between fields
		if p.peekToken.Type == TOKEN_COMMA {
			p.nextToken()
		}
	}

	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}

	return stmt
}

// parseInterfaceDeclaration parses: interface Name { fn method(params) -> returnType; ... }
func (p *Parser) parseInterfaceDeclaration() Statement {
	stmt := &InterfaceDecl{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Methods = []InterfaceMethod{}
	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken() // should be 'fn'
		if p.curToken.Type != TOKEN_FN {
			p.addError(fmt.Sprintf("expected 'fn' in interface body, got %s", p.curToken.Type))
			return nil
		}

		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		method := InterfaceMethod{Name: p.curToken.Literal}

		if !p.expectPeek(TOKEN_LPAREN) {
			return nil
		}

		// Parse parameter list
		method.Params = []string{}
		method.ParamTypes = []string{}
		if p.peekToken.Type != TOKEN_RPAREN {
			p.nextToken()
			method.Params = append(method.Params, p.curToken.Literal)
			if p.peekToken.Type == TOKEN_COLON {
				p.nextToken()
				p.nextToken()
				method.ParamTypes = append(method.ParamTypes, p.curToken.Literal)
			} else {
				method.ParamTypes = append(method.ParamTypes, "")
			}
			for p.peekToken.Type == TOKEN_COMMA {
				p.nextToken()
				p.nextToken()
				method.Params = append(method.Params, p.curToken.Literal)
				if p.peekToken.Type == TOKEN_COLON {
					p.nextToken()
					p.nextToken()
					method.ParamTypes = append(method.ParamTypes, p.curToken.Literal)
				} else {
					method.ParamTypes = append(method.ParamTypes, "")
				}
			}
		}

		if !p.expectPeek(TOKEN_RPAREN) {
			return nil
		}

		// Optional return type
		if p.peekToken.Type == TOKEN_RET_ARROW {
			p.nextToken()
			p.nextToken()
			method.ReturnType = p.curToken.Literal
		}

		stmt.Methods = append(stmt.Methods, method)
	}

	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}

	return stmt
}

func (p *Parser) parseEnumStatement() Statement {
	stmt := &EnumStmt{Token: p.curToken}
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Members = []string{}
	stmt.Values = make(map[string]Expression)
	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken()
		if p.curToken.Type == TOKEN_IDENT {
			memberName := p.curToken.Literal
			stmt.Members = append(stmt.Members, memberName)
			// Check for explicit value: Member = expr
			if p.peekToken.Type == TOKEN_ASSIGN {
				p.nextToken() // consume '='
				p.nextToken() // move to value expression
				stmt.Values[memberName] = p.parseExpression(LOWEST)
			}
		}
		if p.peekToken.Type == TOKEN_COMMA {
			p.nextToken()
		}
	}
	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}
	return stmt
}

// parseTypeAliasStatement parses: type Name = baseType
func (p *Parser) parseTypeAliasStatement() Statement {
	stmt := &TypeAliasStmt{Token: p.curToken}
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal
	if !p.expectPeek(TOKEN_ASSIGN) {
		return nil
	}
	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.BaseType = p.curToken.Literal
	return stmt
}

// parseValidateStatement parses: validate { required "key", optional "key" }
func (p *Parser) parseValidateStatement() Statement {
	stmt := &ValidateStmt{Token: p.curToken}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Required = []string{}
	stmt.Optional = []string{}
	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken()
		if p.curToken.Type == TOKEN_IDENT {
			kind := p.curToken.Literal // "required" or "optional"
			if !p.expectPeek(TOKEN_STRING) {
				return nil
			}
			key := p.curToken.Literal
			switch kind {
			case "required":
				stmt.Required = append(stmt.Required, key)
			case "optional":
				stmt.Optional = append(stmt.Optional, key)
			}
		}
		if p.peekToken.Type == TOKEN_COMMA {
			p.nextToken()
		}
	}
	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}
	return stmt
}

// parseTypeAnnotation reads a type annotation which can be:
// - simple: int, string, T
// - array: []int, []T, []Box[T]
// - optional: int?, string?, User?
// - union: int | string, User | error
// - generic: Box[T], Pair[int, string]
func (p *Parser) parseTypeAnnotation() string {
	var baseType string

	if p.curToken.Type == TOKEN_LBRACKET {
		// Array type: []T
		if p.peekToken.Type == TOKEN_RBRACKET {
			p.nextToken() // consume ']'
			p.nextToken() // move to element type
			baseType = "[]" + p.curToken.Literal
			if p.peekToken.Type == TOKEN_LBRACKET {
				p.nextToken() // consume '['
				var args []string
				for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
					p.nextToken()
					args = append(args, p.parseTypeAnnotation())
					if p.peekToken.Type == TOKEN_COMMA {
						p.nextToken()
					}
				}
				if p.expectPeek(TOKEN_RBRACKET) {
					baseType = baseType + "[" + strings.Join(args, ", ") + "]"
				}
			}
		} else {
			baseType = p.curToken.Literal
		}
	} else {
		baseType = p.curToken.Literal
		if p.peekToken.Type == TOKEN_LBRACKET {
			p.nextToken() // consume '['
			var args []string
			for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
				p.nextToken()
				args = append(args, p.parseTypeAnnotation())
				if p.peekToken.Type == TOKEN_COMMA {
					p.nextToken()
				}
			}
			if p.expectPeek(TOKEN_RBRACKET) {
				baseType = baseType + "[" + strings.Join(args, ", ") + "]"
			}
		}
	}

	// Check for optional: T?
	if p.peekToken.Type == TOKEN_QUESTION {
		p.nextToken() // consume '?'
		baseType = baseType + "?"
	}

	// Check for union: T | U
	if p.peekToken.Type == TOKEN_PIPE {
		p.nextToken() // consume '|'
		p.nextToken() // move to next type
		rightType := p.parseTypeAnnotation()
		baseType = baseType + "|" + rightType
	}

	return baseType
}
