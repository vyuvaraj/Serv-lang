package compiler

import (
	"fmt"
	"strconv"
)

func (p *Parser) parseBrokerStatement() Statement {
	stmt := &BrokerStmt{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	return stmt
}

func (p *Parser) parseServerStatement() Statement {
	stmt := &ServerStmt{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)

	// Optional TLS: server "8080" tls "cert.pem" "key.pem"
	if p.peekToken.Type == TOKEN_IDENT && p.peekToken.Literal == "tls" {
		p.nextToken() // consume 'tls'
		if !p.expectPeek(TOKEN_STRING) {
			return nil
		}
		stmt.TLS = true
		stmt.CertFile = p.curToken.Literal
		if !p.expectPeek(TOKEN_STRING) {
			return nil
		}
		stmt.KeyFile = p.curToken.Literal
	}

	return stmt
}

func (p *Parser) parseRouteStatement() Statement {
	stmt := &RouteStmt{Token: p.curToken}

	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.Method = p.curToken.Literal

	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.Path = p.curToken.Literal

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

	if p.peekToken.Type == TOKEN_LIMIT {
		p.nextToken() // move to limit
		if !p.expectPeek(TOKEN_INT) {
			return nil
		}
		val, err := strconv.Atoi(p.curToken.Literal)
		if err != nil {
			return nil
		}
		stmt.LimitRate = val

		if !p.expectPeek(TOKEN_SLASH) {
			return nil
		}

		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		stmt.LimitPeriod = p.curToken.Literal
	}

	// Optional middleware: use [auth, logging]
	if p.peekToken.Type == TOKEN_USE {
		p.nextToken() // consume 'use'
		if !p.expectPeek(TOKEN_LBRACKET) {
			return nil
		}
		stmt.Middlewares = []string{}
		for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
			p.nextToken()
			if p.curToken.Type == TOKEN_IDENT {
				stmt.Middlewares = append(stmt.Middlewares, p.curToken.Literal)
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

	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseToolStatement() Statement {
	stmt := &ToolStmt{Token: p.curToken}

	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.Description = p.curToken.Literal

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

	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseMigrationStatement() Statement {
	stmt := &MigrationStmt{Token: p.curToken}

	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseEveryStatement() Statement {
	stmt := &EveryStmt{Token: p.curToken}
	p.nextToken()
	stmt.Interval = p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseCronStatement() Statement {
	stmt := &CronStmt{Token: p.curToken}
	p.nextToken()
	stmt.Cron = p.parseExpression(LOWEST)

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseSubscribeStatement() Statement {
	stmt := &SubscribeStmt{Token: p.curToken}
	p.nextToken()
	stmt.Topic = p.parseExpression(LOWEST)

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

	stmt.Body = p.parseBlockStatement()
	return stmt
}

// parseWsStatement parses: ws "/path" (conn) { body }
func (p *Parser) parseWsStatement() Statement {
	stmt := &WsStmt{Token: p.curToken}

	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.Path = p.curToken.Literal

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

	stmt.Body = p.parseBlockStatement()
	return stmt
}

// parseDeclareStatement parses: declare module "pkg/path" { fn Name(params) -> type; ... }
func (p *Parser) parseDeclareStatement() Statement {
	stmt := &DeclareModuleStmt{Token: p.curToken}

	// Expect 'module'
	if !p.expectPeek(TOKEN_MODULE) {
		return nil
	}

	// Package path string
	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.PkgPath = p.curToken.Literal

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Functions = []DeclareModuleFunc{}
	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken()
		if p.curToken.Type != TOKEN_FN {
			p.addError(fmt.Sprintf("expected 'fn' in declare module body, got %s", p.curToken.Type))
			return nil
		}

		// Check for fn! (multi-return / failable function)
		multiReturn := false
		if p.peekToken.Type == TOKEN_BANG {
			multiReturn = true
			p.nextToken() // consume '!'
		}

		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		fn := DeclareModuleFunc{Name: p.curToken.Literal, MultiReturn: multiReturn}

		if !p.expectPeek(TOKEN_LPAREN) {
			return nil
		}

		fn.Params = []string{}
		fn.ParamTypes = []string{}
		if p.peekToken.Type != TOKEN_RPAREN {
			p.nextToken()
			fn.Params = append(fn.Params, p.curToken.Literal)
			if p.peekToken.Type == TOKEN_COLON {
				p.nextToken()
				p.nextToken()
				fn.ParamTypes = append(fn.ParamTypes, p.curToken.Literal)
			} else {
				fn.ParamTypes = append(fn.ParamTypes, "")
			}
			for p.peekToken.Type == TOKEN_COMMA {
				p.nextToken()
				p.nextToken()
				fn.Params = append(fn.Params, p.curToken.Literal)
				if p.peekToken.Type == TOKEN_COLON {
					p.nextToken()
					p.nextToken()
					fn.ParamTypes = append(fn.ParamTypes, p.curToken.Literal)
				} else {
					fn.ParamTypes = append(fn.ParamTypes, "")
				}
			}
		}

		if !p.expectPeek(TOKEN_RPAREN) {
			return nil
		}

		if p.peekToken.Type == TOKEN_RET_ARROW {
			p.nextToken()
			p.nextToken()
			fn.ReturnType = p.curToken.Literal
		}

		stmt.Functions = append(stmt.Functions, fn)
	}

	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}

	return stmt
}

// parseMiddlewareDeclaration parses: middleware name(req) { body }
func (p *Parser) parseMiddlewareDeclaration() Statement {
	stmt := &MiddlewareDecl{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal

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

	stmt.Body = p.parseBlockStatement()
	return stmt
}

func (p *Parser) parseDatabaseStatement() Statement {
	stmt := &DatabaseStmt{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	return stmt
}

func (p *Parser) parseCacheStatement() Statement {
	stmt := &CacheStmt{Token: p.curToken}
	p.nextToken()
	stmt.Value = p.parseExpression(LOWEST)
	return stmt
}

// parseExportStatement parses: export <statement>
// Wraps the inner statement in an ExportStmt node.
func (p *Parser) parseExportStatement() Statement {
	exportToken := p.curToken
	p.nextToken() // move past 'export' to the actual statement

	inner := p.parseStatement()
	if inner == nil {
		return nil
	}

	return &ExportStmt{Token: exportToken, Inner: inner}
}

func (p *Parser) parseCorsStatement() Statement {
	stmt := &CorsStmt{Token: p.curToken}
	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	if !p.expectPeek(TOKEN_IDENT) || p.curToken.Literal != "origins" {
		p.addError("expected 'origins' keyword in cors block")
		return nil
	}

	if !p.expectPeek(TOKEN_COLON) {
		return nil
	}

	p.nextToken()
	if p.curToken.Type == TOKEN_LBRACKET {
		for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
			p.nextToken()
			if p.curToken.Type == TOKEN_STRING {
				stmt.Origins = append(stmt.Origins, p.curToken.Literal)
			}
			if p.peekToken.Type == TOKEN_COMMA {
				p.nextToken()
			}
		}
		if !p.expectPeek(TOKEN_RBRACKET) {
			return nil
		}
	} else if p.curToken.Type == TOKEN_STRING {
		stmt.Origins = []string{p.curToken.Literal}
	} else {
		p.addError("expected string or list of strings for cors origins")
		return nil
	}

	if !p.expectPeek(TOKEN_RBRACE) {
		return nil
	}
	return stmt
}

func (p *Parser) parseRateLimitStatement() Statement {
	stmt := &RateLimitStmt{Token: p.curToken}
	if !p.expectPeek(TOKEN_INT) {
		return nil
	}
	val, err := strconv.Atoi(p.curToken.Literal)
	if err != nil {
		return nil
	}
	stmt.LimitRate = val

	if !p.expectPeek(TOKEN_SLASH) {
		return nil
	}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.LimitPeriod = p.curToken.Literal
	return stmt
}
