package compiler

import (
	"fmt"
)

// Precedences
const (
	_ int = iota
	LOWEST
	ASSIGN  // =
	COMPARE // ==, !=, <, >, <=, >=
	SUM     // +
	PRODUCT // *
	CALL    // func(x)
	MEMBER  // obj.field
	INDEX   // array[index]
)

var precedences = map[TokenType]int{
	TOKEN_ARROW:           ASSIGN,
	TOKEN_ASSIGN:          ASSIGN,
	TOKEN_PLUS_ASSIGN:     ASSIGN,
	TOKEN_MINUS_ASSIGN:    ASSIGN,
	TOKEN_ASTERISK_ASSIGN: ASSIGN,
	TOKEN_SLASH_ASSIGN:    ASSIGN,
	TOKEN_PERCENT_ASSIGN:  ASSIGN,
	TOKEN_EQ:              COMPARE,
	TOKEN_NEQ:             COMPARE,
	TOKEN_LT:              COMPARE,
	TOKEN_GT:              COMPARE,
	TOKEN_LTE:             COMPARE,
	TOKEN_GTE:             COMPARE,
	TOKEN_PIPE:            SUM,
	TOKEN_CARET:           SUM,
	TOKEN_PLUS:            SUM,
	TOKEN_MINUS:           SUM,
	TOKEN_AMPERSAND:       PRODUCT,
	TOKEN_SHIFT_LEFT:      PRODUCT,
	TOKEN_SHIFT_RIGHT:     PRODUCT,
	TOKEN_ASTERISK:        PRODUCT,
	TOKEN_SLASH:           PRODUCT,
	TOKEN_PERCENT:         PRODUCT,
	TOKEN_LPAREN:          CALL,
	TOKEN_DOT:             MEMBER,
	TOKEN_QUESTION_DOT:    MEMBER,
	TOKEN_LBRACKET:        INDEX,
	TOKEN_QUESTION:        CALL,
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
	p.registerPrefix(TOKEN_WORKFLOW, p.parseWorkflowIdentifier)
	p.registerPrefix(TOKEN_ASSERT, p.parseAssertExpression)
	p.registerPrefix(TOKEN_TRUE, p.parseBooleanLiteral)
	p.registerPrefix(TOKEN_FALSE, p.parseBooleanLiteral)
	p.registerPrefix(TOKEN_NIL, p.parseNilLiteral)
	p.registerPrefix(TOKEN_SELF, p.parseSelfExpression)
	p.registerPrefix(TOKEN_AWAIT, p.parseAwaitExpression)
	p.registerPrefix(TOKEN_SPAWN, p.parseSpawnExpression)
	p.registerPrefix(TOKEN_FN, p.parseFnLiteral)
	p.registerPrefix(TOKEN_VALIDATE, p.parseValidateIdentifier)
	p.registerPrefix(TOKEN_MINUS, p.parsePrefixExpression)
	p.registerPrefix(TOKEN_BANG, p.parsePrefixExpression)

	p.infixParseFns = make(map[TokenType]infixParseFn)
	p.registerInfix(TOKEN_PLUS, p.parseInfixExpression)
	p.registerInfix(TOKEN_MINUS, p.parseInfixExpression)
	p.registerInfix(TOKEN_ASTERISK, p.parseInfixExpression)
	p.registerInfix(TOKEN_SLASH, p.parseInfixExpression)
	p.registerInfix(TOKEN_PERCENT, p.parseInfixExpression)
	p.registerInfix(TOKEN_AMPERSAND, p.parseInfixExpression)
	p.registerInfix(TOKEN_PIPE, p.parseInfixExpression)
	p.registerInfix(TOKEN_CARET, p.parseInfixExpression)
	p.registerInfix(TOKEN_SHIFT_LEFT, p.parseInfixExpression)
	p.registerInfix(TOKEN_SHIFT_RIGHT, p.parseInfixExpression)
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
	p.registerInfix(TOKEN_PLUS_ASSIGN, p.parseCompoundAssignExpression)
	p.registerInfix(TOKEN_MINUS_ASSIGN, p.parseCompoundAssignExpression)
	p.registerInfix(TOKEN_ASTERISK_ASSIGN, p.parseCompoundAssignExpression)
	p.registerInfix(TOKEN_SLASH_ASSIGN, p.parseCompoundAssignExpression)
	p.registerInfix(TOKEN_PERCENT_ASSIGN, p.parseCompoundAssignExpression)
	p.registerInfix(TOKEN_ARROW, p.parseArrowFunction)
	p.registerInfix(TOKEN_QUESTION, p.parseErrorPropExpression)

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
	case TOKEN_MOCK:
		return p.parseMockStatement()
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
	case TOKEN_ACTOR:
		return p.parseActorDeclaration()
	case TOKEN_WORKFLOW:
		if p.peekToken.Type == TOKEN_DOT {
			// workflow.start(...) — used as an expression, not a declaration
			return p.parseExpressionStatement()
		}
		return p.parseWorkflowDeclaration()
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
		if p.peekToken.Type == TOKEN_LPAREN {
			return p.parseExpressionStatement()
		}
		return p.parseValidateStatement()
	case TOKEN_EXPORT:
		return p.parseExportStatement()
	case TOKEN_BREAK:
		return &BreakStmt{Token: p.curToken}
	case TOKEN_CONTINUE:
		return &ContinueStmt{Token: p.curToken}
	case TOKEN_CORS:
		return p.parseCorsStatement()
	case TOKEN_RATE_LIMIT:
		return p.parseRateLimitStatement()
	case TOKEN_BEFORE_EACH:
		return p.parseBeforeEachStatement()
	case TOKEN_AFTER_EACH:
		return p.parseAfterEachStatement()
	default:
		return p.parseExpressionStatement()
	}
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
