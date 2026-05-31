package compiler

import (
	"bytes"
	"strings"
)

type Node interface {
	TokenLiteral() string
	String() string
}

type Statement interface {
	Node
	statementNode()
}

type Expression interface {
	Node
	expressionNode()
}

// Program Node
type Program struct {
	Statements []Statement
}

func (p *Program) TokenLiteral() string {
	if len(p.Statements) > 0 {
		return p.Statements[0].TokenLiteral()
	}
	return ""
}

func (p *Program) String() string {
	var out bytes.Buffer
	for _, s := range p.Statements {
		out.WriteString(s.String())
	}
	return out.String()
}

// Import Statement
type ImportStmt struct {
	Token TokenType
	Path  string
}

func (i *ImportStmt) statementNode()       {}
func (i *ImportStmt) TokenLiteral() string { return string(i.Token) }
func (i *ImportStmt) String() string       { return "import \"" + i.Path + "\"\n" }

// Extern Statement
type ExternFnStmt struct {
	Token  Token
	Name   string
	Params []string
	Source string // e.g. "go:github.com/google/uuid:NewString" or "python:./scripts/analyzer.py:analyze"
}

func (e *ExternFnStmt) statementNode()       {}
func (e *ExternFnStmt) TokenLiteral() string { return e.Token.Literal }
func (e *ExternFnStmt) String() string {
	return "extern fn " + e.Name + "(" + strings.Join(e.Params, ", ") + ") from \"" + e.Source + "\"\n"
}

// Broker Statement
type BrokerStmt struct {
	Token Token
	Value Expression
}

func (b *BrokerStmt) statementNode()       {}
func (b *BrokerStmt) TokenLiteral() string { return b.Token.Literal }
func (b *BrokerStmt) String() string       { return "broker " + b.Value.String() + "\n" }

// Server Statement
type ServerStmt struct {
	Token Token
	Value Expression
}

func (s *ServerStmt) statementNode()       {}
func (s *ServerStmt) TokenLiteral() string { return s.Token.Literal }
func (s *ServerStmt) String() string       { return "server " + s.Value.String() + "\n" }

// Database Statement
type DatabaseStmt struct {
	Token Token
	Value Expression
}

func (d *DatabaseStmt) statementNode()       {}
func (d *DatabaseStmt) TokenLiteral() string { return d.Token.Literal }
func (d *DatabaseStmt) String() string       { return "database " + d.Value.String() + "\n" }

// Cache Statement
type CacheStmt struct {
	Token Token
	Value Expression
}

func (c *CacheStmt) statementNode()       {}
func (c *CacheStmt) TokenLiteral() string { return c.Token.Literal }
func (c *CacheStmt) String() string       { return "cache " + c.Value.String() + "\n" }

// Route Statement
type RouteStmt struct {
	Token       Token
	Method      string
	Path        string
	Param       string // req object name, e.g. "req"
	Body        *BlockStmt
	LimitRate   int
	LimitPeriod string
}

func (r *RouteStmt) statementNode()       {}
func (r *RouteStmt) TokenLiteral() string { return r.Token.Literal }
func (r *RouteStmt) String() string {
	return "route \"" + r.Method + "\" \"" + r.Path + "\" (" + r.Param + ") " + r.Body.String() + "\n"
}

// Tool Statement
type ToolStmt struct {
	Token       Token
	Name        string
	Description string
	Param       string // args object name, e.g. "args"
	Body        *BlockStmt
}

func (t *ToolStmt) statementNode()       {}
func (t *ToolStmt) TokenLiteral() string { return t.Token.Literal }
func (t *ToolStmt) String() string {
	return "tool \"" + t.Name + "\" \"" + t.Description + "\" (" + t.Param + ") " + t.Body.String() + "\n"
}

// Migration Statement
type MigrationStmt struct {
	Token Token
	Name  string
	Body  *BlockStmt
}

func (m *MigrationStmt) statementNode()       {}
func (m *MigrationStmt) TokenLiteral() string { return m.Token.Literal }
func (m *MigrationStmt) String() string {
	return "migration \"" + m.Name + "\" " + m.Body.String() + "\n"
}

// Every Statement
type EveryStmt struct {
	Token    Token
	Interval Expression // duration token or configuration expression
	Body     *BlockStmt
}

func (e *EveryStmt) statementNode()       {}
func (e *EveryStmt) TokenLiteral() string { return e.Token.Literal }
func (e *EveryStmt) String() string {
	return "every " + e.Interval.String() + " " + e.Body.String() + "\n"
}

// Cron Statement
type CronStmt struct {
	Token Token
	Cron  Expression // cron string or config expr
	Body  *BlockStmt
}

func (c *CronStmt) statementNode()       {}
func (c *CronStmt) TokenLiteral() string { return c.Token.Literal }
func (c *CronStmt) String() string {
	return "cron " + c.Cron.String() + " " + c.Body.String() + "\n"
}

// Subscribe Statement
type SubscribeStmt struct {
	Token Token
	Topic Expression // Topic name as expression (literal string or config)
	Param string     // parameter name, e.g. "msg"
	Body  *BlockStmt
}

func (s *SubscribeStmt) statementNode()       {}
func (s *SubscribeStmt) TokenLiteral() string { return s.Token.Literal }
func (s *SubscribeStmt) String() string {
	return "subscribe " + s.Topic.String() + " (" + s.Param + ") " + s.Body.String() + "\n"
}

// Publish Statement
type PublishStmt struct {
	Token Token
	Topic Expression
	Value Expression
}

func (p *PublishStmt) statementNode()       {}
func (p *PublishStmt) TokenLiteral() string { return p.Token.Literal }
func (p *PublishStmt) String() string {
	return "publish " + p.Topic.String() + " " + p.Value.String() + "\n"
}

// Spawn Statement
type SpawnStmt struct {
	Token Token
	Call  Expression // should resolve to CallExpr
	Limit Expression // optional limit expression (nil if none)
}

func (s *SpawnStmt) statementNode()       {}
func (s *SpawnStmt) TokenLiteral() string { return s.Token.Literal }
func (s *SpawnStmt) String() string {
	if s.Limit != nil {
		return "spawn(" + s.Limit.String() + ") " + s.Call.String() + "\n"
	}
	return "spawn " + s.Call.String() + "\n"
}

type MatchCase struct {
	Value Expression // nil if default "_"
	Body  *BlockStmt
}

type MatchStmt struct {
	Token Token // "match"
	Value Expression
	Cases []MatchCase
}

// Test Statement
type TestStmt struct {
	Token Token
	Name  string // test name
	Body  *BlockStmt
}

func (t *TestStmt) statementNode() {}
func (t *TestStmt) TokenLiteral() string { return t.Token.Literal }
func (t *TestStmt) String() string {
    var out bytes.Buffer
    out.WriteString("test " + t.Name + " {")
    out.WriteString(t.Body.String())
    out.WriteString("}\n")
    return out.String()
}

// Assert Expression
type AssertExpr struct {
	Token Token
	Cond  Expression
}

func (a *AssertExpr) expressionNode() {}
func (a *AssertExpr) TokenLiteral() string { return a.Token.Literal }
func (a *AssertExpr) String() string {
    return "assert " + a.Cond.String()
}

func (m *MatchStmt) statementNode()       {}
func (m *MatchStmt) TokenLiteral() string { return m.Token.Literal }
func (m *MatchStmt) String() string {
	var out bytes.Buffer
	out.WriteString("match " + m.Value.String() + " {\n")
	for _, c := range m.Cases {
		if c.Value != nil {
			out.WriteString(c.Value.String() + " => " + c.Body.String() + "\n")
		} else {
			out.WriteString("_ => " + c.Body.String() + "\n")
		}
	}
	out.WriteString("}\n")
	return out.String()
}

// Let Statement
type LetStmt struct {
	Token Token
	Name  string
	Type  string // optional type annotation
	Value Expression
}

func (l *LetStmt) statementNode()       {}
func (l *LetStmt) TokenLiteral() string { return l.Token.Literal }
func (l *LetStmt) String() string       { return "let " + l.Name + " = " + l.Value.String() + "\n" }

// Return Statement
type ReturnStmt struct {
	Token Token
	Value Expression
}

func (r *ReturnStmt) statementNode()       {}
func (r *ReturnStmt) TokenLiteral() string { return r.Token.Literal }
func (r *ReturnStmt) String() string       { return "return " + r.Value.String() + "\n" }

// Block Statement
type BlockStmt struct {
	Token      Token
	Statements []Statement
}

func (b *BlockStmt) statementNode()       {}
func (b *BlockStmt) TokenLiteral() string { return b.Token.Literal }
func (b *BlockStmt) String() string {
	var out bytes.Buffer
	out.WriteString("{ ")
	for _, s := range b.Statements {
		out.WriteString(s.String())
	}
	out.WriteString(" }")
	return out.String()
}

// Fn Declaration Statement
type FnDecl struct {
	Token      Token
	Name       string
	Params     []string
	ParamTypes []string // optional types for each param
	ReturnType string   // optional return type
	Body       *BlockStmt
}

func (f *FnDecl) statementNode()       {}
func (f *FnDecl) TokenLiteral() string { return f.Token.Literal }
func (f *FnDecl) String() string {
	return "fn " + f.Name + "(" + strings.Join(f.Params, ", ") + ") " + f.Body.String() + "\n"
}

// Expression Statement
type ExprStmt struct {
	Token Token
	Value Expression
}

func (e *ExprStmt) statementNode()       {}
func (e *ExprStmt) TokenLiteral() string { return e.Token.Literal }
func (e *ExprStmt) String() string       { return e.Value.String() }

// TryCatch Statement
type TryCatchStmt struct {
	Token      Token // the 'try' token
	TryBody    *BlockStmt
	Param      string // variable name for the error, e.g., "err"
	CatchBody  *BlockStmt
}

func (t *TryCatchStmt) statementNode()       {}
func (t *TryCatchStmt) TokenLiteral() string { return t.Token.Literal }
func (t *TryCatchStmt) String() string {
	return "try " + t.TryBody.String() + " catch (" + t.Param + ") " + t.CatchBody.String() + "\n"
}

// Assignment Expression (e.g. x = y)
type AssignExpr struct {
	Token Token
	Name  string
	Value Expression
}

func (a *AssignExpr) expressionNode()      {}
func (a *AssignExpr) TokenLiteral() string { return a.Token.Literal }
func (a *AssignExpr) String() string       { return a.Name + " = " + a.Value.String() }

type MemberAssignExpr struct {
	Token  Token
	Object Expression
	Field  string
	Value  Expression
}

func (m *MemberAssignExpr) expressionNode()      {}
func (m *MemberAssignExpr) TokenLiteral() string { return m.Token.Literal }
func (m *MemberAssignExpr) String() string {
	return m.Object.String() + "." + m.Field + " = " + m.Value.String()
}

// Expressions

type Identifier struct {
	Token Token
	Value string
}

func (i *Identifier) expressionNode()      {}
func (i *Identifier) TokenLiteral() string { return i.Token.Literal }
func (i *Identifier) String() string       { return i.Value }

type StringLiteral struct {
	Token Token
	Value string
}

func (s *StringLiteral) expressionNode()      {}
func (s *StringLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *StringLiteral) String() string       { return "\"" + s.Value + "\"" }

type FStringLiteral struct {
	Token Token
	Value string
}

func (f *FStringLiteral) expressionNode()      {}
func (f *FStringLiteral) TokenLiteral() string { return f.Token.Literal }
func (f *FStringLiteral) String() string       { return "f\"" + f.Value + "\"" }

type IntegerLiteral struct {
	Token Token
	Value int64
}

func (i *IntegerLiteral) expressionNode()      {}
func (i *IntegerLiteral) TokenLiteral() string { return i.Token.Literal }
func (i *IntegerLiteral) String() string       { return i.Token.Literal }

type FloatLiteral struct {
	Token Token
	Value float64
}

func (f *FloatLiteral) expressionNode()      {}
func (f *FloatLiteral) TokenLiteral() string { return f.Token.Literal }
func (f *FloatLiteral) String() string       { return f.Token.Literal }

type ArrayLiteral struct {
	Token    Token
	Elements []Expression
}

func (a *ArrayLiteral) expressionNode()      {}
func (a *ArrayLiteral) TokenLiteral() string { return a.Token.Literal }
func (a *ArrayLiteral) String() string {
	var elements []string
	for _, el := range a.Elements {
		elements = append(elements, el.String())
	}
	return "[" + strings.Join(elements, ", ") + "]"
}

type DurationLiteral struct {
	Token Token
	Value string // e.g. "5s", "10m"
}

func (d *DurationLiteral) expressionNode()      {}
func (d *DurationLiteral) TokenLiteral() string { return d.Token.Literal }
func (d *DurationLiteral) String() string       { return d.Value }

type MemberExpr struct {
	Token  Token
	Object Expression
	Field  string
}

func (m *MemberExpr) expressionNode()      {}
func (m *MemberExpr) TokenLiteral() string { return m.Token.Literal }
func (m *MemberExpr) String() string       { return m.Object.String() + "." + m.Field }

type CallExpr struct {
	Token    Token
	Function Expression // Identifier or MemberExpr
	Arguments []Expression
}

func (c *CallExpr) expressionNode()      {}
func (c *CallExpr) TokenLiteral() string { return c.Token.Literal }
func (c *CallExpr) String() string {
	var args []string
	for _, a := range c.Arguments {
		args = append(args, a.String())
	}
	return c.Function.String() + "(" + strings.Join(args, ", ") + ")"
}

type MapLiteral struct {
	Token Token
	Pairs map[string]Expression
}

func (m *MapLiteral) expressionNode()      {}
func (m *MapLiteral) TokenLiteral() string { return m.Token.Literal }
func (m *MapLiteral) String() string {
	var pairs []string
	for k, v := range m.Pairs {
		pairs = append(pairs, "\""+k+"\": "+v.String())
	}
	return "{" + strings.Join(pairs, ", ") + "}"
}

type InfixExpr struct {
	Token    Token
	Left     Expression
	Operator string
	Right    Expression
}

func (i *InfixExpr) expressionNode()      {}
func (i *InfixExpr) TokenLiteral() string { return i.Token.Literal }
func (i *InfixExpr) String() string       { return "(" + i.Left.String() + " " + i.Operator + " " + i.Right.String() + ")" }

type IndexExpr struct {
	Token Token // The '[' token
	Left  Expression
	Index Expression
}

func (i *IndexExpr) expressionNode()      {}
func (i *IndexExpr) TokenLiteral() string { return i.Token.Literal }
func (i *IndexExpr) String() string       { return "(" + i.Left.String() + "[" + i.Index.String() + "])" }

type EnumStmt struct {
	Token   Token
	Name    string
	Members []string
}

func (e *EnumStmt) statementNode()       {}
func (e *EnumStmt) TokenLiteral() string { return e.Token.Literal }
func (e *EnumStmt) String() string {
	return "enum " + e.Name + " { " + strings.Join(e.Members, ", ") + " }\n"
}
