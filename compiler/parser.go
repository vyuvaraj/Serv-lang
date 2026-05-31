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
	TOKEN_ASSIGN:   ASSIGN,
	TOKEN_EQ:       COMPARE,
	TOKEN_NEQ:      COMPARE,
	TOKEN_LT:       COMPARE,
	TOKEN_GT:       COMPARE,
	TOKEN_LTE:      COMPARE,
	TOKEN_GTE:      COMPARE,
	TOKEN_PLUS:     SUM,
	TOKEN_MINUS:    SUM,
	TOKEN_ASTERISK: PRODUCT,
	TOKEN_SLASH:    PRODUCT,
	TOKEN_LPAREN:   CALL,
	TOKEN_DOT:      MEMBER,
	TOKEN_LBRACKET: INDEX,
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
	p.registerInfix(TOKEN_LBRACKET, p.parseIndexExpression)
	p.registerInfix(TOKEN_ASSIGN, p.parseAssignmentExpression)

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
	default:
		return p.parseExpressionStatement()
	}
}

func (p *Parser) parseImportStatement() Statement {
	stmt := &ImportStmt{Token: p.curToken.Type}

	if !p.expectPeek(TOKEN_STRING) {
		return nil
	}
	stmt.Path = p.curToken.Literal

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
	stmt := &LetStmt{Token: p.curToken}

	if !p.expectPeek(TOKEN_IDENT) {
		return nil
	}
	stmt.Name = p.curToken.Literal

	// Optional type annotation after name: let x : int = 5
	if p.peekToken.Type == TOKEN_COLON {
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
	stmt := &FnDecl{Token: p.curToken}

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
		// possible type after param
		if p.peekToken.Type == TOKEN_COLON {
			p.nextToken() // skip ':'
			p.nextToken() // type identifier
			stmt.ParamTypes = append(stmt.ParamTypes, p.curToken.Literal)
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
				stmt.ParamTypes = append(stmt.ParamTypes, p.curToken.Literal)
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
	return &Identifier{Token: p.curToken, Value: p.curToken.Literal}
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
	m := &MapLiteral{Token: p.curToken, Pairs: make(map[string]Expression)}

	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken() // move to key
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
	for p.peekToken.Type != TOKEN_RBRACE && p.peekToken.Type != TOKEN_EOF {
		p.nextToken()
		if p.curToken.Type == TOKEN_IDENT {
			stmt.Members = append(stmt.Members, p.curToken.Literal)
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
