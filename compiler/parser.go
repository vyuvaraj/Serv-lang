package compiler

import (
	"fmt"
	"strconv"
)

// Precedences
const (
	_ int = iota
	LOWEST
	ASSIGN      // =
	COMPARE     // ==, !=, <, >, <=, >=
	SUM         // +
	PRODUCT     // *
	CALL        // func(x)
	MEMBER      // obj.field
	INDEX       // array[index]
)

var precedences = map[TokenType]int{
	TOKEN_ARROW:        ASSIGN, // => has same precedence as assignment (low)
	TOKEN_ASSIGN:       ASSIGN,
	TOKEN_EQ:           COMPARE,
	TOKEN_NEQ:          COMPARE,
	TOKEN_LT:           COMPARE,
	TOKEN_GT:           COMPARE,
	TOKEN_LTE:          COMPARE,
	TOKEN_GTE:          COMPARE,
	TOKEN_PLUS:         SUM,
	TOKEN_MINUS:        SUM,
	TOKEN_ASTERISK:     PRODUCT,
	TOKEN_SLASH:        PRODUCT,
	TOKEN_LPAREN:       CALL,
	TOKEN_DOT:          MEMBER,
	TOKEN_QUESTION_DOT: MEMBER,
	TOKEN_LBRACKET:     INDEX,
}

type Parser struct {
	l         *Lexer
	curToken  Token
	peekToken Token
	errors    []string

	prefixParseFns map[TokenType]prefixParseFn
	infixParseFns  map[TokenType]infixParseFn
}

type (
	prefixParseFn func() Expression
	infixParseFn  func(Expression) Expression
)

func NewParser(l *Lexer) *Parser {
	p := &Parser{l: l, errors: []string{}}

	p.prefixParseFns = make(map[TokenType]prefixParseFn)
	p.registerPrefix(TOKEN_IDENT, p.parseIdentifier)
	p.registerPrefix(TOKEN_INT, p.parseIntegerLiteral)
	p.registerPrefix(TOKEN_FLOAT, p.parseFloatLiteral)
	p.registerPrefix(TOKEN_STRING, p.parseStringLiteral)
	p.registerPrefix(TOKEN_DURATION, p.parseDurationLiteral)
	p.registerPrefix(TOKEN_LBRACE, p.parseMapLiteral)
	p.registerPrefix(TOKEN_LBRACKET, p.parseArrayLiteral)
	p.registerPrefix(TOKEN_LPAREN, p.parseGroupedExpression)
	p.registerPrefix(TOKEN_FSTRING, p.parseFStringLiteral)
	p.registerPrefix(TOKEN_CACHE, p.parseCacheIdentifier)
	p.registerPrefix(TOKEN_ASSERT, p.parseAssertExpression)
	p.registerPrefix(TOKEN_TRUE, p.parseBooleanLiteral)
	p.registerPrefix(TOKEN_FALSE, p.parseBooleanLiteral)
	p.registerPrefix(TOKEN_NIL, p.parseNilLiteral)
	p.registerPrefix(TOKEN_SELF, p.parseSelfExpression)
	p.registerPrefix(TOKEN_AWAIT, p.parseAwaitExpression)
	p.registerPrefix(TOKEN_FN, p.parseFnLiteral)

	p.infixParseFns = make(map[TokenType]infixParseFn)
	p.registerInfix(TOKEN_PLUS, p.parseInfixExpression)
	p.registerInfix(TOKEN_MINUS, p.parseInfixExpression)
	p.registerInfix(TOKEN_ASTERISK, p.parseInfixExpression)
	p.registerInfix(TOKEN_SLASH, p.parseInfixExpression)
	p.registerInfix(TOKEN_EQ, p.parseInfixExpression)
	p.registerInfix(TOKEN_NEQ, p.parseInfixExpression)
	p.registerInfix(TOKEN_LT, p.parseInfixExpression)
	p.registerInfix(TOKEN_GT, p.parseInfixExpression)
	p.registerInfix(TOKEN_LTE, p.parseInfixExpression)
	p.registerInfix(TOKEN_GTE, p.parseInfixExpression)
	p.registerInfix(TOKEN_LPAREN, p.parseCallExpression)
	p.registerInfix(TOKEN_DOT, p.parseMemberExpression)
	p.registerInfix(TOKEN_QUESTION_DOT, p.parseOptionalMemberExpression)
	p.registerInfix(TOKEN_LBRACKET, p.parseIndexExpression)
	p.registerInfix(TOKEN_ASSIGN, p.parseAssignmentExpression)
	p.registerInfix(TOKEN_ARROW, p.parseArrowFunction)

	// Read two tokens so curToken and peekToken are both set
	p.nextToken()
	p.nextToken()

	return p
}

func (p *Parser) registerPrefix(tokenType TokenType, fn prefixParseFn) {
	p.prefixParseFns[tokenType] = fn
}

func (p *Parser) registerInfix(tokenType TokenType, fn infixParseFn) {
	p.infixParseFns[tokenType] = fn
}

func (p *Parser) nextToken() {
	p.curToken = p.peekToken
	p.peekToken = p.l.NextToken()
}

func (p *Parser) Errors() []string {
	return p.errors
}

func (p *Parser) addError(msg string) {
	p.errors = append(p.errors, fmt.Sprintf("[Line %d, Col %d] %s", p.curToken.Line, p.curToken.Col, msg))
}

func (p *Parser) ParseProgram() *Program {
	program := &Program{}
	program.Statements = []Statement{}

	for p.curToken.Type != TOKEN_EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			program.Statements = append(program.Statements, stmt)
		}
		p.nextToken()
	}

	return program
}

func (p *Parser) parseStatement() Statement {
	switch p.curToken.Type {
	case TOKEN_IMPORT:
		return p.parseImportStatement()
	case TOKEN_EXTERN:
		return p.parseExternStatement()
	case TOKEN_BROKER:
		return p.parseBrokerStatement()
	case TOKEN_SERVER:
		return p.parseServerStatement()
	case TOKEN_ROUTE:
		return p.parseRouteStatement()
	case TOKEN_TOOL:
		return p.parseToolStatement()
	case TOKEN_EVERY:
		return p.parseEveryStatement()
	case TOKEN_CRON:
		return p.parseCronStatement()
	case TOKEN_SUBSCRIBE:
		return p.parseSubscribeStatement()
	case TOKEN_PUBLISH:
		return p.parsePublishStatement()
	case TOKEN_SPAWN:
		return p.parseSpawnStatement()
	case TOKEN_LET:
		return p.parseLetStatement()
	case TOKEN_RETURN:
		return p.parseReturnStatement()
	case TOKEN_FN:
		return p.parseFnDeclaration()
	case TOKEN_TRY:
		return p.parseTryCatchStatement()
	case TOKEN_DATABASE:
		return p.parseDatabaseStatement()
	case TOKEN_CACHE:
		if p.peekToken.Type == TOKEN_DOT {
			return p.parseExpressionStatement()
		}
		return p.parseCacheStatement()
	case TOKEN_MATCH:
		return p.parseMatchStatement()
	case TOKEN_TEST:
		return p.parseTestStatement()
	case TOKEN_ENUM:
		return p.parseEnumStatement()
	case TOKEN_MIGRATION:
		return p.parseMigrationStatement()
	case TOKEN_IF:
		return p.parseIfStatement()
	case TOKEN_FOR:
		return p.parseForStatement()
	case TOKEN_STRUCT:
		return p.parseStructDeclaration()
	case TOKEN_INTERFACE:
		return p.parseInterfaceDeclaration()
	case TOKEN_MIDDLEWARE:
		return p.parseMiddlewareDeclaration()
	case TOKEN_DECLARE:
		return p.parseDeclareStatement()
	case TOKEN_WS:
		return p.parseWsStatement()
	case TOKEN_TYPE:
		return p.parseTypeAliasStatement()
	case TOKEN_VALIDATE:
		return p.parseValidateStatement()
	case TOKEN_EXPORT:
		return p.parseExportStatement()
	default:
		return p.parseExpressionStatement()
	}
}

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
		stmt.Type = p.curToken.Literal
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

	// Optional type parameters: fn name[T, U](...)
	if p.peekToken.Type == TOKEN_LBRACKET {
		p.nextToken() // consume '['
		stmt.TypeParams = []string{}
		for p.peekToken.Type != TOKEN_RBRACKET && p.peekToken.Type != TOKEN_EOF {
			p.nextToken()
			if p.curToken.Type == TOKEN_IDENT {
				stmt.TypeParams = append(stmt.TypeParams, p.curToken.Literal)
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
			stmt.ParamTypes = append(stmt.ParamTypes, p.curToken.Literal)
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
				stmt.ParamTypes = append(stmt.ParamTypes, p.curToken.Literal)
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
		stmt.ReturnType = p.curToken.Literal
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Body = p.parseBlockStatement()
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
			c.Value = p.parseExpression(LOWEST)
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

func (p *Parser) parseFStringLiteral() Expression {
	return &FStringLiteral{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseCacheIdentifier() Expression {
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
}

func (p *Parser) parseBlockStatement() *BlockStmt {
	block := &BlockStmt{Token: p.curToken}
	block.Statements = []Statement{}

	p.nextToken() // skip '{'

	for p.curToken.Type != TOKEN_RBRACE && p.curToken.Type != TOKEN_EOF {
		stmt := p.parseStatement()
		if stmt != nil {
			block.Statements = append(block.Statements, stmt)
		}
		p.nextToken()
	}

	return block
}

func (p *Parser) parseExpressionStatement() *ExprStmt {
	stmt := &ExprStmt{Token: p.curToken}
	stmt.Value = p.parseExpression(LOWEST)
	return stmt
}

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

// parseTestStatement parses a test block: test name { ... } or test "description" { ... }
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

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()
	return stmt
}


// parseAssertExpression parses an assert expression: assert <cond>
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
	default:
		p.addError("left side of assignment must be an identifier or member expression")
		return nil
	}
}

func (p *Parser) expectPeek(t TokenType) bool {
	if p.peekToken.Type == t {
		p.nextToken()
		return true
	}
	p.peekError(t)
	return false
}

func (p *Parser) isParamListFollowedByBrace() bool {
	// Clone lexer state
	lCopy := NewLexer(p.l.input)
	lCopy.position = p.l.position
	lCopy.readPosition = p.l.readPosition
	lCopy.ch = p.l.ch
	lCopy.line = p.l.line
	lCopy.col = p.l.col

	// peekToken is currently '('. Next token on copy should be IDENT
	t1 := lCopy.NextToken()
	if t1.Type != TOKEN_IDENT {
		return false
	}
	t2 := lCopy.NextToken()
	if t2.Type != TOKEN_RPAREN {
		return false
	}
	t3 := lCopy.NextToken()
	if t3.Type != TOKEN_LBRACE {
		return false
	}
	return true
}

func (p *Parser) peekPrecedence() int {
	if p.peekToken.Type == TOKEN_LPAREN {
		if p.isParamListFollowedByBrace() {
			return LOWEST
		}
	}
	if p, ok := precedences[p.peekToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) curPrecedence() int {
	if p, ok := precedences[p.curToken.Type]; ok {
		return p
	}
	return LOWEST
}

func (p *Parser) peekError(t TokenType) {
	p.errors = append(p.errors, fmt.Sprintf("[Line %d, Col %d] expected next token to be %s, got %s instead", p.peekToken.Line, p.peekToken.Col, t, p.peekToken.Type))
}

func (p *Parser) parseIndexExpression(left Expression) Expression {
	expr := &IndexExpr{Token: p.curToken, Left: left}
	p.nextToken() // skip '['
	expr.Index = p.parseExpression(LOWEST)
	if !p.expectPeek(TOKEN_RBRACKET) {
		return nil
	}
	return expr
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
			if kind == "required" {
				stmt.Required = append(stmt.Required, key)
			} else if kind == "optional" {
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
func (p *Parser) parseForStatement() Statement {
	stmt := &ForStmt{Token: p.curToken}

	p.nextToken()

	// Check if this is "for x in collection" pattern
	if p.curToken.Type == TOKEN_IDENT && p.peekToken.Type == TOKEN_IN {
		stmt.Variable = p.curToken.Literal
		stmt.IsRange = true
		p.nextToken() // skip 'in'
		p.nextToken() // move to iterable expression
		stmt.Iterable = p.parseExpression(LOWEST)
	} else {
		// Condition-based loop: for <condition> { ... }
		stmt.IsRange = false
		stmt.Iterable = p.parseExpression(LOWEST)
	}

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}
	stmt.Body = p.parseBlockStatement()

	return stmt
}

func (p *Parser) parseBooleanLiteral() Expression {
	return &BooleanLiteral{Token: p.curToken, Value: p.curToken.Type == TOKEN_TRUE}
}

func (p *Parser) parseNilLiteral() Expression {
	return &NilLiteral{Token: p.curToken}
}

// parseTypeAnnotation reads a type annotation which can be:
// - simple: int, string, T
// - array: []int, []T
// - pointer: *User (future)
func (p *Parser) parseTypeAnnotation() string {
	if p.curToken.Type == TOKEN_LBRACKET {
		// Array type: []T
		if p.peekToken.Type == TOKEN_RBRACKET {
			p.nextToken() // consume ']'
			p.nextToken() // move to element type
			return "[]" + p.curToken.Literal
		}
	}
	return p.curToken.Literal
}

func (p *Parser) parseSelfExpression() Expression {
	return &SelfExpr{Token: p.curToken}
}

func (p *Parser) parseAwaitExpression() Expression {
	expr := &AwaitExpr{Token: p.curToken}
	p.nextToken()
	expr.Value = p.parseExpression(LOWEST)
	return expr
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

// parseStructDeclaration parses: struct Name { field: type, ... }
func (p *Parser) parseStructDeclaration() Statement {
	stmt := &StructDecl{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	if !p.expectPeek(TOKEN_LBRACE) {
		return nil
	}

	stmt.Fields = []StructField{}
	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken()
		if p.curToken.Type != TOKEN_IDENT {
			p.addError(fmt.Sprintf("expected field name in struct, got %s", p.curToken.Type))
			return nil
		}
		fieldName := p.curToken.Literal

		if !p.expectPeek(TOKEN_COLON) {
			return nil
		}
		p.nextToken() // type identifier
		fieldType := p.curToken.Literal

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

		if !p.expectPeek(TOKEN_IDENT) {
			return nil
		}
		fn := DeclareModuleFunc{Name: p.curToken.Literal}

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
