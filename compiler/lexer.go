package compiler


type TokenType string

const (
	TOKEN_EOF        TokenType = "EOF"
	TOKEN_ILLEGAL    TokenType = "ILLEGAL"
	TOKEN_IDENT      TokenType = "IDENT"
	TOKEN_INT        TokenType = "INT"
	TOKEN_FLOAT      TokenType = "FLOAT"
	TOKEN_STRING     TokenType = "STRING"
	TOKEN_DURATION   TokenType = "DURATION"

	// Keywords
	TOKEN_AUTH       TokenType = "AUTH"
	TOKEN_MAIL       TokenType = "MAIL"
	TOKEN_SEARCH     TokenType = "SEARCH"
	TOKEN_BROKER     TokenType = "BROKER"
	TOKEN_AI         TokenType = "AI"
	TOKEN_SERVER     TokenType = "SERVER"
	TOKEN_ROUTE      TokenType = "ROUTE"
	TOKEN_EVERY      TokenType = "EVERY"
	TOKEN_CRON       TokenType = "CRON"
	TOKEN_SUBSCRIBE  TokenType = "SUBSCRIBE"
	TOKEN_PUBLISH    TokenType = "PUBLISH"
	TOKEN_SPAWN      TokenType = "SPAWN"
	TOKEN_FN         TokenType = "FN"
	TOKEN_LET        TokenType = "LET"
	TOKEN_RETURN     TokenType = "RETURN"
	TOKEN_IMPORT     TokenType = "IMPORT"
	TOKEN_EXTERN     TokenType = "EXTERN"
	TOKEN_FROM       TokenType = "FROM"
	TOKEN_TRY        TokenType = "TRY"
	TOKEN_CATCH      TokenType = "CATCH"
	TOKEN_DATABASE   TokenType = "DATABASE"
	TOKEN_CACHE      TokenType = "CACHE"
	TOKEN_MATCH      TokenType = "MATCH"
	TOKEN_FSTRING    TokenType = "FSTRING"
	TOKEN_AND        TokenType = "AND"
	TOKEN_OR         TokenType = "OR"
	TOKEN_ENUM       TokenType = "ENUM"
	TOKEN_TOOL       TokenType = "TOOL"
	TOKEN_LIMIT      TokenType = "LIMIT"
	TOKEN_MIGRATION  TokenType = "MIGRATION"
	TOKEN_IF         TokenType = "IF"
	TOKEN_ELSE       TokenType = "ELSE"
	TOKEN_FOR        TokenType = "FOR"
	TOKEN_IN         TokenType = "IN"
	TOKEN_TRUE       TokenType = "TRUE"
	TOKEN_FALSE      TokenType = "FALSE"
	TOKEN_NIL        TokenType = "NIL"
	TOKEN_STRUCT     TokenType = "STRUCT"
	TOKEN_SELF       TokenType = "SELF"
	TOKEN_EXPORT     TokenType = "EXPORT"
	TOKEN_INTERFACE  TokenType = "INTERFACE"
	TOKEN_MIDDLEWARE TokenType = "MIDDLEWARE"
	TOKEN_USE        TokenType = "USE"
	TOKEN_AWAIT      TokenType = "AWAIT"
	TOKEN_DECLARE    TokenType = "DECLARE"
	TOKEN_MODULE     TokenType = "MODULE"
	TOKEN_AS         TokenType = "AS"
	TOKEN_WS         TokenType = "WS"
	TOKEN_CORS       TokenType = "CORS"
	TOKEN_RATE_LIMIT TokenType = "RATE_LIMIT"
	TOKEN_MOCK       TokenType = "MOCK"
	TOKEN_ACTOR      TokenType = "ACTOR"
	TOKEN_WORKFLOW   TokenType = "WORKFLOW"
	TOKEN_STREAM     TokenType = "STREAM"
	TOKEN_YIELD      TokenType = "YIELD"
	TOKEN_STORE      TokenType = "STORE"
	TOKEN_VERSION    TokenType = "VERSION"
	TOKEN_RESILIENT  TokenType = "RESILIENT"
	TOKEN_RETRIES    TokenType = "RETRIES"
	TOKEN_CIRCUIT_BREAKER TokenType = "CIRCUIT_BREAKER"

	// Operators & Delimiters
	TOKEN_ASSIGN     TokenType = "="
	TOKEN_ARROW      TokenType = "=>"
	TOKEN_RET_ARROW  TokenType = "->"
	TOKEN_PLUS       TokenType = "+"
	TOKEN_MINUS      TokenType = "-"
	TOKEN_ASTERISK   TokenType = "*"
	TOKEN_SLASH      TokenType = "/"
	TOKEN_PERCENT    TokenType = "%"
	TOKEN_COMMA      TokenType = ","
	TOKEN_LPAREN     TokenType = "("
	TOKEN_RPAREN     TokenType = ")"
	TOKEN_LBRACE     TokenType = "{"
	TOKEN_RBRACE     TokenType = "}"
	TOKEN_LBRACKET   TokenType = "["
	TOKEN_RBRACKET   TokenType = "]"
	TOKEN_DOT        TokenType = "."
	TOKEN_COLON      TokenType = ":"
	TOKEN_TEST       TokenType = "TEST"
	TOKEN_ASSERT     TokenType = "ASSERT"

	// Compound assignment operators
	TOKEN_PLUS_ASSIGN    TokenType = "+="
	TOKEN_MINUS_ASSIGN   TokenType = "-="
	TOKEN_ASTERISK_ASSIGN TokenType = "*="
	TOKEN_SLASH_ASSIGN   TokenType = "/="
	TOKEN_PERCENT_ASSIGN TokenType = "%="

	// Bitwise operators
	TOKEN_AMPERSAND  TokenType = "&"
	TOKEN_PIPE       TokenType = "|"
	TOKEN_PIPE_ARROW TokenType = "|>"
	TOKEN_CARET      TokenType = "^"
	TOKEN_SHIFT_LEFT TokenType = "<<"
	TOKEN_SHIFT_RIGHT TokenType = ">>"

	// Comparison operators
	TOKEN_EQ         TokenType = "=="
	TOKEN_NEQ        TokenType = "!="
	TOKEN_LT         TokenType = "<"
	TOKEN_GT         TokenType = ">"
	TOKEN_LTE        TokenType = "<="
	TOKEN_GTE        TokenType = ">="
	TOKEN_BANG       TokenType = "!"
	TOKEN_QUESTION_DOT TokenType = "?."
	TOKEN_QUESTION     TokenType = "?"
	TOKEN_SPREAD     TokenType = "..."
	TOKEN_TYPE       TokenType = "TYPE"
	TOKEN_VALIDATE   TokenType = "VALIDATE"

	// Keywords
	TOKEN_BREAK      TokenType = "BREAK"
	TOKEN_CONTINUE   TokenType = "CONTINUE"
	TOKEN_BEFORE_EACH TokenType = "BEFORE_EACH"
	TOKEN_AFTER_EACH  TokenType = "AFTER_EACH"
	TOKEN_TIMEOUT    TokenType = "TIMEOUT"
)

type Token struct {
	Type    TokenType
	Literal string
	Line    int
	Col     int
}

type Lexer struct {
	input        string
	position     int  // current position in input (points to current char)
	readPosition int  // current reading position in input (after current char)
	ch           byte // current char under examination
	line         int
	col          int
}

func NewLexer(input string) *Lexer {
	l := &Lexer{input: input, line: 1, col: 0}
	l.readChar()
	return l
}

func (l *Lexer) readChar() {
	if l.readPosition >= len(l.input) {
		l.ch = 0
	} else {
		l.ch = l.input[l.readPosition]
	}
	l.position = l.readPosition
	l.readPosition++
	l.col++
}

func (l *Lexer) peekChar() byte {
	if l.readPosition >= len(l.input) {
		return 0
	}
	return l.input[l.readPosition]
}

func (l *Lexer) NextToken() Token {
	l.skipWhitespace()

	var tok Token
	tok.Line = l.line
	tok.Col = l.col

	switch l.ch {
	case '+':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			tok.Type = TOKEN_PLUS_ASSIGN
			tok.Literal = "+="
			return tok
		}
		tok.Type = TOKEN_PLUS
		tok.Literal = string(l.ch)
	case '-':
		if l.peekChar() == '>' {
			l.readChar() // skip '-'
			l.readChar() // skip '>'
			tok.Type = TOKEN_RET_ARROW
			tok.Literal = "->"
			return tok
		}
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			tok.Type = TOKEN_MINUS_ASSIGN
			tok.Literal = "-="
			return tok
		}
		tok.Type = TOKEN_MINUS
		tok.Literal = string(l.ch)
	case '=':
		if l.peekChar() == '>' {
			l.readChar() // skip '='
			l.readChar() // skip '>'
			tok.Type = TOKEN_ARROW
			tok.Literal = "=>"
			return tok
		} else if l.peekChar() == '=' {
			l.readChar() // skip first '='
			l.readChar() // skip second '='
			tok.Type = TOKEN_EQ
			tok.Literal = "=="
			return tok
		}
		tok.Type = TOKEN_ASSIGN
		tok.Literal = string(l.ch)
	case '*':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			tok.Type = TOKEN_ASTERISK_ASSIGN
			tok.Literal = "*="
			return tok
		}
		tok.Type = TOKEN_ASTERISK
		tok.Literal = string(l.ch)
	case '/':
		if l.peekChar() == '/' {
			l.skipComment()
			return l.NextToken()
		}
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			tok.Type = TOKEN_SLASH_ASSIGN
			tok.Literal = "/="
			return tok
		}
		tok.Type = TOKEN_SLASH
		tok.Literal = string(l.ch)
	case '%':
		if l.peekChar() == '=' {
			l.readChar()
			l.readChar()
			tok.Type = TOKEN_PERCENT_ASSIGN
			tok.Literal = "%="
			return tok
		}
		tok.Type = TOKEN_PERCENT
		tok.Literal = string(l.ch)
	case '&':
		if l.peekChar() == '&' {
			l.readChar()
			l.readChar()
			tok.Type = TOKEN_AND
			tok.Literal = "&&"
			return tok
		}
		tok.Type = TOKEN_AMPERSAND
		tok.Literal = string(l.ch)
	case '|':
		if l.peekChar() == '>' {
			l.readChar()
			l.readChar()
			tok.Type = TOKEN_PIPE_ARROW
			tok.Literal = "|>"
			return tok
		} else if l.peekChar() == '|' {
			l.readChar()
			l.readChar()
			tok.Type = TOKEN_OR
			tok.Literal = "||"
			return tok
		}
		tok.Type = TOKEN_PIPE
		tok.Literal = string(l.ch)
	case '^':
		tok.Type = TOKEN_CARET
		tok.Literal = string(l.ch)
	case ',':
		tok.Type = TOKEN_COMMA
		tok.Literal = string(l.ch)
	case '.':
		if l.peekChar() == '.' {
			// Check for '...' (spread operator)
			if l.readPosition < len(l.input) && l.input[l.readPosition] == '.' {
				l.readChar() // skip first '.'
				l.readChar() // skip second '.'
				l.readChar() // skip third '.'
				tok.Type = TOKEN_SPREAD
				tok.Literal = "..."
				return tok
			}
		}
		tok.Type = TOKEN_DOT
		tok.Literal = string(l.ch)
	case '?':
		if l.peekChar() == '.' {
			l.readChar() // skip '?'
			l.readChar() // skip '.'
			tok.Type = TOKEN_QUESTION_DOT
			tok.Literal = "?."
			return tok
		}
		tok.Type = TOKEN_QUESTION
		tok.Literal = "?"
	case ':':
		tok.Type = TOKEN_COLON
		tok.Literal = string(l.ch)
	case '(':
		tok.Type = TOKEN_LPAREN
		tok.Literal = string(l.ch)
	case ')':
		tok.Type = TOKEN_RPAREN
		tok.Literal = string(l.ch)
	case '{':
		tok.Type = TOKEN_LBRACE
		tok.Literal = string(l.ch)
	case '}':
		tok.Type = TOKEN_RBRACE
		tok.Literal = string(l.ch)
	case '[':
		tok.Type = TOKEN_LBRACKET
		tok.Literal = string(l.ch)
	case ']':
		tok.Type = TOKEN_RBRACKET
		tok.Literal = string(l.ch)
	case 'f':
		if l.peekChar() == '"' {
			l.readChar() // skip 'f'
			tok.Type = TOKEN_FSTRING
			tok.Literal = l.readString()
			return tok
		}
		tok.Literal = l.readIdentifier()
		tok.Type = lookupIdent(tok.Literal)
		return tok
	case '`':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readRawString()
		return tok
	case '"':
		tok.Type = TOKEN_STRING
		tok.Literal = l.readString()
		return tok
	case 0:
		tok.Type = TOKEN_EOF
		tok.Literal = ""
	default:
		if isLetter(l.ch) {
			tok.Literal = l.readIdentifier()
			tok.Type = lookupIdent(tok.Literal)
			return tok
		} else if isDigit(l.ch) {
			lit, tokType := l.readNumberOrDurationOrFloat()
			tok.Type = tokType
			tok.Literal = lit
			return tok
		} else {
			switch l.ch {
			case '!':
				if l.peekChar() == '=' {
					l.readChar()
					l.readChar()
					tok.Type = TOKEN_NEQ
					tok.Literal = "!="
					return tok
				}
				tok.Type = TOKEN_BANG
				tok.Literal = "!"
			case '<':
				if l.peekChar() == '=' {
					l.readChar()
					l.readChar()
					tok.Type = TOKEN_LTE
					tok.Literal = "<="
					return tok
				}
				if l.peekChar() == '<' {
					l.readChar()
					l.readChar()
					tok.Type = TOKEN_SHIFT_LEFT
					tok.Literal = "<<"
					return tok
				}
				tok.Type = TOKEN_LT
				tok.Literal = "<"
			case '>':
				if l.peekChar() == '=' {
					l.readChar()
					l.readChar()
					tok.Type = TOKEN_GTE
					tok.Literal = ">="
					return tok
				}
				if l.peekChar() == '>' {
					l.readChar()
					l.readChar()
					tok.Type = TOKEN_SHIFT_RIGHT
					tok.Literal = ">>"
					return tok
				}
				tok.Type = TOKEN_GT
				tok.Literal = ">"
			default:
				tok.Type = TOKEN_ILLEGAL
				tok.Literal = string(l.ch)
			}
		}
	}

	l.readChar()
	return tok
}

func (l *Lexer) skipWhitespace() {
	for l.ch == ' ' || l.ch == '\t' || l.ch == '\n' || l.ch == '\r' {
		if l.ch == '\n' {
			l.line++
			l.col = 0
		}
		l.readChar()
	}
}

func (l *Lexer) skipComment() {
	// Skip '//'
	l.readChar()
	l.readChar()
	for l.ch != '\n' && l.ch != 0 {
		l.readChar()
	}
	if l.ch == '\n' {
		l.line++
		l.col = 0
		l.readChar()
	}
}

func (l *Lexer) readIdentifier() string {
	position := l.position
	for isLetter(l.ch) || isDigit(l.ch) || l.ch == '_' {
		l.readChar()
	}
	return l.input[position:l.position]
}

func (l *Lexer) readNumberOrDurationOrFloat() (string, TokenType) {
	position := l.position
	for isDigit(l.ch) {
		l.readChar()
	}
	// Check if float (dot followed by digit)
	if l.ch == '.' && isDigit(l.peekChar()) {
		l.readChar() // consume '.'
		for isDigit(l.ch) {
			l.readChar()
		}
		return l.input[position:l.position], TOKEN_FLOAT
	}
	// Check if this is a duration literal (e.g., 5s, 10m, 2h, 50ms)
	if l.ch == 'm' && l.peekChar() == 's' {
		l.readChar() // consume 'm'
		l.readChar() // consume 's'
		return l.input[position:l.position], TOKEN_DURATION
	}
	if l.ch == 's' || l.ch == 'm' || l.ch == 'h' {
		l.readChar()
		return l.input[position:l.position], TOKEN_DURATION
	}
	return l.input[position:l.position], TOKEN_INT
}

func (l *Lexer) readString() string {
	l.readChar() // skip leading quote
	var buf []byte
	for l.ch != '"' && l.ch != 0 {
		if l.ch == '\\' {
			next := l.peekChar()
			if next == '"' {
				buf = append(buf, '"')
				l.readChar()
				l.readChar()
				continue
			} else if next == '\\' {
				buf = append(buf, '\\')
				l.readChar()
				l.readChar()
				continue
			}
		}
		if l.ch == '\n' {
			l.line++
			l.col = 0
		}
		buf = append(buf, l.ch)
		l.readChar()
	}
	if l.ch == '"' {
		l.readChar() // skip trailing quote
	}
	return string(buf)
}

func (l *Lexer) readRawString() string {
	l.readChar() // skip leading backtick
	var buf []byte
	for l.ch != '`' && l.ch != 0 {
		if l.ch == '\n' {
			l.line++
			l.col = 0
		}
		buf = append(buf, l.ch)
		l.readChar()
	}
	if l.ch == '`' {
		l.readChar() // skip trailing backtick
	}
	return string(buf)
}

func isLetter(ch byte) bool {
	return 'a' <= ch && ch <= 'z' || 'A' <= ch && ch <= 'Z' || ch == '_'
}

func isDigit(ch byte) bool {
	return '0' <= ch && ch <= '9'
}

var keywords = map[string]TokenType{
	"auth":      TOKEN_AUTH,
	"mail":      TOKEN_MAIL,
	"search":    TOKEN_SEARCH,
	"broker":    TOKEN_BROKER,
	"ai":       TOKEN_AI,
	"server":   TOKEN_SERVER,
	"route":     TOKEN_ROUTE,
	"every":     TOKEN_EVERY,
	"cron":      TOKEN_CRON,
	"subscribe": TOKEN_SUBSCRIBE,
	"publish":   TOKEN_PUBLISH,
	"spawn":     TOKEN_SPAWN,
	"fn":        TOKEN_FN,
	"let":       TOKEN_LET,
	"return":    TOKEN_RETURN,
	"import":    TOKEN_IMPORT,
	"extern":    TOKEN_EXTERN,
	"from":      TOKEN_FROM,
	"try":       TOKEN_TRY,
	"catch":     TOKEN_CATCH,
	"database":  TOKEN_DATABASE,
	"cache":     TOKEN_CACHE,
	"match":     TOKEN_MATCH,
	"test":      TOKEN_TEST,
	"assert":    TOKEN_ASSERT,
	"enum":      TOKEN_ENUM,
	"tool":      TOKEN_TOOL,
	"limit":     TOKEN_LIMIT,
	"migration": TOKEN_MIGRATION,
	"if":        TOKEN_IF,
	"else":      TOKEN_ELSE,
	"for":       TOKEN_FOR,
	"in":        TOKEN_IN,
	"true":      TOKEN_TRUE,
	"false":     TOKEN_FALSE,
	"nil":       TOKEN_NIL,
	"struct":    TOKEN_STRUCT,
	"self":      TOKEN_SELF,
	"export":    TOKEN_EXPORT,
	"interface": TOKEN_INTERFACE,
	"middleware": TOKEN_MIDDLEWARE,
	"use":       TOKEN_USE,
	"await":     TOKEN_AWAIT,
	"declare":   TOKEN_DECLARE,
	"module":    TOKEN_MODULE,
	"as":        TOKEN_AS,
	"ws":        TOKEN_WS,
	"type":      TOKEN_TYPE,
	"validate":  TOKEN_VALIDATE,
	"break":     TOKEN_BREAK,
	"continue":  TOKEN_CONTINUE,
	"beforeEach": TOKEN_BEFORE_EACH,
	"afterEach":  TOKEN_AFTER_EACH,
	"timeout":   TOKEN_TIMEOUT,
	"cors":      TOKEN_CORS,
	"rate_limit": TOKEN_RATE_LIMIT,
	"mock":       TOKEN_MOCK,
	"actor":      TOKEN_ACTOR,
	"workflow":   TOKEN_WORKFLOW,
	"stream":     TOKEN_STREAM,
	"yield":      TOKEN_YIELD,
	"store":      TOKEN_STORE,
	"version":    TOKEN_VERSION,
	"resilient":  TOKEN_RESILIENT,
	"retries":    TOKEN_RETRIES,
	"circuit_breaker": TOKEN_CIRCUIT_BREAKER,
}

func lookupIdent(ident string) TokenType {
	if tok, ok := keywords[ident]; ok {
		return tok
	}
	return TOKEN_IDENT
}

// isKeywordToken returns true if the token type is a language keyword.
// Used to allow keywords as identifiers in contexts like middleware names.
func isKeywordToken(t TokenType) bool {
	for _, kw := range keywords {
		if kw == t {
			return true
		}
	}
	return false
}
