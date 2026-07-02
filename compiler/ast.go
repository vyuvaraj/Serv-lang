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
// Supports both:
//   import "path.srv"                         (imports all exported symbols)
//   import { Symbol1, Symbol2 } from "path.srv"  (selective named imports)
type ImportStmt struct {
	Token   TokenType
	Path    string
	Names   []string // empty = import all, non-empty = selective
}

func (i *ImportStmt) statementNode()       {}
func (i *ImportStmt) TokenLiteral() string { return string(i.Token) }
func (i *ImportStmt) String() string {
	if len(i.Names) > 0 {
		return "import { " + strings.Join(i.Names, ", ") + " } from \"" + i.Path + "\"\n"
	}
	return "import \"" + i.Path + "\"\n"
}

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

// AI Statement: ai "openai://gpt-4"
type AiStmt struct {
	Token Token
	Value Expression
}

func (b *BrokerStmt) statementNode()       {}
func (b *BrokerStmt) TokenLiteral() string { return b.Token.Literal }
func (b *BrokerStmt) String() string       { return "broker " + b.Value.String() + "\n" }

func (a *AiStmt) statementNode()       {}
func (a *AiStmt) TokenLiteral() string { return a.Token.Literal }
func (a *AiStmt) String() string       { return "ai " + a.Value.String() + "\n" }

// Server Statement
type ServerStmt struct {
	Token    Token
	Value    Expression
	TLS      bool
	CertFile string
	KeyFile  string
}

func (s *ServerStmt) statementNode()       {}
func (s *ServerStmt) TokenLiteral() string { return s.Token.Literal }
func (s *ServerStmt) String() string       { return "server " + s.Value.String() + "\n" }

// Cors Statement
type CorsStmt struct {
	Token   Token
	Origins []string
}

func (c *CorsStmt) statementNode()       {}
func (c *CorsStmt) TokenLiteral() string { return c.Token.Literal }
func (c *CorsStmt) String() string       { return "cors ...\n" }

// RateLimit Statement
type RateLimitStmt struct {
	Token       Token
	LimitRate   int
	LimitPeriod string
}

func (r *RateLimitStmt) statementNode()       {}
func (r *RateLimitStmt) TokenLiteral() string { return r.Token.Literal }
func (r *RateLimitStmt) String() string       { return "rate_limit ...\n" }

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

// Auth Statement
type AuthStmt struct {
	Token Token
	Value Expression
}

func (a *AuthStmt) statementNode()       {}
func (a *AuthStmt) TokenLiteral() string { return a.Token.Literal }
func (a *AuthStmt) String() string       { return "auth " + a.Value.String() + "\n" }

// Mail Statement
type MailStmt struct {
	Token Token
	Value Expression
}

func (m *MailStmt) statementNode()       {}
func (m *MailStmt) TokenLiteral() string { return m.Token.Literal }
func (m *MailStmt) String() string       { return "mail " + m.Value.String() + "\n" }

// Notify Statement
type NotifyStmt struct {
	Token Token
	Value Expression
}

func (n *NotifyStmt) statementNode()       {}
func (n *NotifyStmt) TokenLiteral() string { return n.Token.Literal }
func (n *NotifyStmt) String() string       { return "notify " + n.Value.String() + "\n" }

// Store Statement
type StoreStmt struct {
	Token Token
	Value Expression
}

func (s *StoreStmt) statementNode()       {}
func (s *StoreStmt) TokenLiteral() string { return s.Token.Literal }
func (s *StoreStmt) String() string       { return "store " + s.Value.String() + "\n" }

// Search Statement
type SearchStmt struct {
	Token Token
	Value Expression
}

func (s *SearchStmt) statementNode()       {}
func (s *SearchStmt) TokenLiteral() string { return s.Token.Literal }
func (s *SearchStmt) String() string       { return "search " + s.Value.String() + "\n" }




// Route Statement
type RouteStmt struct {
	Token       Token
	Method      string
	Path        string
	Param       string // req object name, e.g. "req"
	Body        *BlockStmt
	LimitRate   int
	LimitPeriod string
	Middlewares []string // middleware names from "use [auth, logging]"
	Stream      bool
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

type DBColumn struct {
	Name string
	Type string
}

type DBTable struct {
	Name    string
	Columns []DBColumn
}

type MigrationStmt struct {
	Token  Token
	Name   string
	Body   *BlockStmt
	Tables []DBTable
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

// Spawn Expression
type SpawnExpr struct {
	Token Token
	Call  Expression // should resolve to CallExpr
	Limit Expression // optional limit expression (nil if none)
}

func (s *SpawnExpr) expressionNode()      {}
func (s *SpawnExpr) TokenLiteral() string { return s.Token.Literal }
func (s *SpawnExpr) String() string {
	if s.Limit != nil {
		return "spawn(" + s.Limit.String() + ") " + s.Call.String()
	}
	return "spawn " + s.Call.String()
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
	Token   Token
	Name    string // test name
	Timeout string // optional timeout duration (e.g. "5s"), empty if none
	Body    *BlockStmt
}

func (t *TestStmt) statementNode() {}
func (t *TestStmt) TokenLiteral() string { return t.Token.Literal }
func (t *TestStmt) String() string {
    var out bytes.Buffer
    out.WriteString("test ")
    out.WriteString(t.Name)
    out.WriteString(" {")
    out.WriteString(t.Body.String())
    out.WriteString("}\n")
    return out.String()
}

// Mock Statement
type MockStmt struct {
	Token  Token // TOKEN_MOCK ("mock")
	Target Expression
	Body   *BlockStmt
}

func (m *MockStmt) statementNode()       {}
func (m *MockStmt) TokenLiteral() string { return m.Token.Literal }
func (m *MockStmt) String() string {
	var out bytes.Buffer
	out.WriteString("mock ")
	out.WriteString(m.Target.String())
	out.WriteString(" ")
	out.WriteString(m.Body.String())
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

// BeforeEachStmt: beforeEach { ... } — runs before each test
type BeforeEachStmt struct {
	Token Token
	Body  *BlockStmt
}

func (b *BeforeEachStmt) statementNode()       {}
func (b *BeforeEachStmt) TokenLiteral() string { return b.Token.Literal }
func (b *BeforeEachStmt) String() string       { return "beforeEach " + b.Body.String() + "\n" }

// AfterEachStmt: afterEach { ... } — runs after each test
type AfterEachStmt struct {
	Token Token
	Body  *BlockStmt
}

func (a *AfterEachStmt) statementNode()       {}
func (a *AfterEachStmt) TokenLiteral() string { return a.Token.Literal }
func (a *AfterEachStmt) String() string       { return "afterEach " + a.Body.String() + "\n" }

func (m *MatchStmt) statementNode()       {}
func (m *MatchStmt) TokenLiteral() string { return m.Token.Literal }
func (m *MatchStmt) String() string {
	var out bytes.Buffer
	out.WriteString("match ")
	out.WriteString(m.Value.String())
	out.WriteString(" {\n")
	for _, c := range m.Cases {
		if c.Value != nil {
			out.WriteString(c.Value.String())
			out.WriteString(" => ")
			out.WriteString(c.Body.String())
			out.WriteString("\n")
		} else {
			out.WriteString("_ => ")
			out.WriteString(c.Body.String())
			out.WriteString("\n")
		}
	}
	out.WriteString("}\n")
	return out.String()
}

// Let Statement
// Supports single: let x = expr
// Supports multi:  let val, err = expr
type LetStmt struct {
	Token Token
	Name  string   // primary variable name
	Names []string // all variable names (for multi-return: ["val", "err"])
	Type  string   // optional type annotation
	Value Expression
}

func (l *LetStmt) statementNode()       {}
func (l *LetStmt) TokenLiteral() string { return l.Token.Literal }
func (l *LetStmt) String() string {
	if len(l.Names) > 1 {
		return "let " + strings.Join(l.Names, ", ") + " = " + l.Value.String() + "\n"
	}
	return "let " + l.Name + " = " + l.Value.String() + "\n"
}

// Return Statement
type ReturnStmt struct {
	Token Token
	Value Expression
}

func (r *ReturnStmt) statementNode()       {}
func (r *ReturnStmt) TokenLiteral() string { return r.Token.Literal }
func (r *ReturnStmt) String() string       { return "return " + r.Value.String() + "\n" }

// Yield Statement
type YieldStmt struct {
	Token Token
	Value Expression
}

func (y *YieldStmt) statementNode()       {}
func (y *YieldStmt) TokenLiteral() string { return y.Token.Literal }
func (y *YieldStmt) String() string       { return "yield " + y.Value.String() + "\n" }


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
	Token           Token
	Name            string
	TypeParams      []string // generic type parameters: [T, U]
	TypeConstraints []string // constraints for each type param: [Comparable, Numeric] (empty = any)
	Params          []string
	ParamTypes      []string // optional types for each param
	ReturnType      string   // optional return type
	Body            *BlockStmt
	IsResilient       bool
	Retries           int
	Timeout           string
	HasCircuitBreaker bool
}

func (f *FnDecl) statementNode()       {}
func (f *FnDecl) TokenLiteral() string { return f.Token.Literal }
func (f *FnDecl) String() string {
	return "fn " + f.Name + "(" + strings.Join(f.Params, ", ") + ") " + f.Body.String() + "\n"
}

type VersionBlockStmt struct {
	Token      Token
	Version    string
	Statements []Statement
}

func (v *VersionBlockStmt) statementNode()       {}
func (v *VersionBlockStmt) TokenLiteral() string { return v.Token.Literal }
func (v *VersionBlockStmt) String() string {
	var out strings.Builder
	out.WriteString("version \"")
	out.WriteString(v.Version)
	out.WriteString("\" {\n")
	for _, s := range v.Statements {
		out.WriteString(s.String())
	}
	out.WriteString("}\n")
	return out.String()
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

type IndexAssignExpr struct {
	Token Token
	Left  *IndexExpr
	Value Expression
}

func (ia *IndexAssignExpr) expressionNode()      {}
func (ia *IndexAssignExpr) TokenLiteral() string { return ia.Token.Literal }
func (ia *IndexAssignExpr) String() string {
	return ia.Left.String() + " = " + ia.Value.String()
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
	Token     Token
	Function  Expression // Identifier or MemberExpr
	TypeArgs  []string   // generic type arguments: name[int, string](...)
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
	Token         Token
	Pairs         map[string]Expression
	KeyOrder      []string // preserves insertion order for deterministic codegen
	Spreads       []SpreadEntry // spread expressions in order
	ConcurrentMap bool // true if this map literal escapes or is accessed concurrently
}

// SpreadEntry represents a ...expr in a map literal.
// Position is the index in KeyOrder where this spread should be merged.
type SpreadEntry struct {
	Position int // index in final output order
	Value    Expression
}

func (m *MapLiteral) expressionNode()      {}
func (m *MapLiteral) TokenLiteral() string { return m.Token.Literal }
func (m *MapLiteral) String() string {
	var pairs []string
	for _, k := range m.KeyOrder {
		v := m.Pairs[k]
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

// PrefixExpr: -x, !x
type PrefixExpr struct {
	Token    Token
	Operator string
	Right    Expression
}

func (p *PrefixExpr) expressionNode()      {}
func (p *PrefixExpr) TokenLiteral() string { return p.Token.Literal }
func (p *PrefixExpr) String() string       { return "(" + p.Operator + p.Right.String() + ")" }

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
	Values  map[string]Expression // member -> explicit value (nil if auto)
}

func (e *EnumStmt) statementNode()       {}
func (e *EnumStmt) TokenLiteral() string { return e.Token.Literal }
func (e *EnumStmt) String() string {
	return "enum " + e.Name + " { " + strings.Join(e.Members, ", ") + " }\n"
}

// If Statement
type IfStmt struct {
	Token       Token
	Condition   Expression
	Body        *BlockStmt
	ElseBody    *BlockStmt // nil if no else
}

func (i *IfStmt) statementNode()       {}
func (i *IfStmt) TokenLiteral() string { return i.Token.Literal }
func (i *IfStmt) String() string {
	out := "if " + i.Condition.String() + " " + i.Body.String()
	if i.ElseBody != nil {
		out += " else " + i.ElseBody.String()
	}
	return out + "\n"
}

// For Statement: for item in collection { ... } or for condition { ... }
// Also supports: for key, value in map { ... }
type ForStmt struct {
	Token      Token
	Variable   string     // loop variable name (empty for condition-based)
	KeyVar     string     // key variable name (for map iteration: for k, v in map)
	Iterable   Expression // collection to iterate, or condition expression
	IsRange    bool       // true = for x in collection, false = for condition
	Body       *BlockStmt
}

func (f *ForStmt) statementNode()       {}
func (f *ForStmt) TokenLiteral() string { return f.Token.Literal }
func (f *ForStmt) String() string {
	if f.IsRange {
		if f.KeyVar != "" {
			return "for " + f.KeyVar + ", " + f.Variable + " in " + f.Iterable.String() + " " + f.Body.String() + "\n"
		}
		return "for " + f.Variable + " in " + f.Iterable.String() + " " + f.Body.String() + "\n"
	}
	return "for " + f.Iterable.String() + " " + f.Body.String() + "\n"
}

// Break Statement
type BreakStmt struct {
	Token Token
}

func (b *BreakStmt) statementNode()       {}
func (b *BreakStmt) TokenLiteral() string { return b.Token.Literal }
func (b *BreakStmt) String() string       { return "break\n" }

// Continue Statement
type ContinueStmt struct {
	Token Token
}

func (co *ContinueStmt) statementNode()       {}
func (co *ContinueStmt) TokenLiteral() string { return co.Token.Literal }
func (co *ContinueStmt) String() string       { return "continue\n" }

// CompoundAssignExpr: x += 1, x -= 2, etc.
type CompoundAssignExpr struct {
	Token    Token
	Name     string
	Operator string // "+=", "-=", "*=", "/=", "%="
	Value    Expression
}

func (c *CompoundAssignExpr) expressionNode()      {}
func (c *CompoundAssignExpr) TokenLiteral() string { return c.Token.Literal }
func (c *CompoundAssignExpr) String() string {
	return c.Name + " " + c.Operator + " " + c.Value.String()
}

// SliceExpr: arr[start:end]
type SliceExpr struct {
	Token Token
	Left  Expression
	Start Expression // nil means from beginning
	End   Expression // nil means to end
}

func (s *SliceExpr) expressionNode()      {}
func (s *SliceExpr) TokenLiteral() string { return s.Token.Literal }
func (s *SliceExpr) String() string {
	startStr := ""
	if s.Start != nil {
		startStr = s.Start.String()
	}
	endStr := ""
	if s.End != nil {
		endStr = s.End.String()
	}
	return s.Left.String() + "[" + startStr + ":" + endStr + "]"
}

// Boolean Literal
type BooleanLiteral struct {
	Token Token
	Value bool
}

func (b *BooleanLiteral) expressionNode()      {}
func (b *BooleanLiteral) TokenLiteral() string { return b.Token.Literal }
func (b *BooleanLiteral) String() string       { return b.Token.Literal }

// Nil Literal
type NilLiteral struct {
	Token Token
}

func (n *NilLiteral) expressionNode()      {}
func (n *NilLiteral) TokenLiteral() string { return n.Token.Literal }
func (n *NilLiteral) String() string       { return "nil" }

// Await Expression: await expr or await all([expr1, expr2])
type AwaitExpr struct {
	Token Token
	Value Expression
}

func (a *AwaitExpr) expressionNode()      {}
func (a *AwaitExpr) TokenLiteral() string { return a.Token.Literal }
func (a *AwaitExpr) String() string       { return "await " + a.Value.String() }

// ErrorPropExpr: expr? — if the expression returns an error, propagate it (return early)
type ErrorPropExpr struct {
	Token Token
	Value Expression
}

func (e *ErrorPropExpr) expressionNode()      {}
func (e *ErrorPropExpr) TokenLiteral() string { return e.Token.Literal }
func (e *ErrorPropExpr) String() string       { return e.Value.String() + "?" }

// Function Literal Expression: fn(x, y) { body } or x => expr
type FnLiteral struct {
	Token      Token
	Params     []string
	ParamTypes []string
	Body       *BlockStmt
	IsArrow    bool       // true for x => expr shorthand
	ArrowExpr  Expression // the expression for arrow functions (body is nil)
}

func (f *FnLiteral) expressionNode()      {}
func (f *FnLiteral) TokenLiteral() string { return f.Token.Literal }
func (f *FnLiteral) String() string {
	if f.IsArrow {
		return strings.Join(f.Params, ", ") + " => " + f.ArrowExpr.String()
	}
	return "fn(" + strings.Join(f.Params, ", ") + ") " + f.Body.String()
}

// Struct Declaration: struct Name { field: type, ... }
type StructField struct {
	Name string
	Type string
}

type StructDecl struct {
	Token           Token
	Name            string
	TypeParams      []string // generic type parameters: [T, U]
	TypeConstraints []string // constraints: [Comparable, Numeric] (empty = any)
	Fields          []StructField
}

func (s *StructDecl) statementNode()       {}
func (s *StructDecl) TokenLiteral() string { return s.Token.Literal }
func (s *StructDecl) String() string {
	var fields []string
	for _, f := range s.Fields {
		fields = append(fields, f.Name+": "+f.Type)
	}
	return "struct " + s.Name + " { " + strings.Join(fields, ", ") + " }\n"
}

// Struct Literal Expression: TypeName { field: value, ... }
type StructLiteral struct {
	Token    Token
	TypeName string
	TypeArgs []string // generic type arguments: Box[int]
	Fields   map[string]Expression
	KeyOrder []string
}

func (s *StructLiteral) expressionNode()      {}
func (s *StructLiteral) TokenLiteral() string { return s.Token.Literal }
func (s *StructLiteral) String() string {
	var pairs []string
	for _, k := range s.KeyOrder {
		v := s.Fields[k]
		pairs = append(pairs, k+": "+v.String())
	}
	return s.TypeName + " { " + strings.Join(pairs, ", ") + " }"
}

// Method Declaration: fn TypeName.methodName(params) -> returnType { body }
type MethodDecl struct {
	Token      Token
	TypeName   string   // receiver type
	Name       string   // method name
	Params     []string
	ParamTypes []string
	ReturnType string
	Body       *BlockStmt
}

func (m *MethodDecl) statementNode()       {}
func (m *MethodDecl) TokenLiteral() string { return m.Token.Literal }
func (m *MethodDecl) String() string {
	return "fn " + m.TypeName + "." + m.Name + "(" + strings.Join(m.Params, ", ") + ") " + m.Body.String() + "\n"
}

// Self Expression: self.field access inside methods
type SelfExpr struct {
	Token Token
}

func (s *SelfExpr) expressionNode()      {}
func (s *SelfExpr) TokenLiteral() string { return s.Token.Literal }
func (s *SelfExpr) String() string       { return "self" }

// ExportStmt wraps a statement to mark it as exported from a module.
type ExportStmt struct {
	Token Token
	Inner Statement // the actual statement being exported
}

func (e *ExportStmt) statementNode()       {}
func (e *ExportStmt) TokenLiteral() string { return e.Token.Literal }
func (e *ExportStmt) String() string       { return "export " + e.Inner.String() }

// Interface Declaration
type InterfaceMethod struct {
	Name       string
	Params     []string
	ParamTypes []string
	ReturnType string
}

type InterfaceDecl struct {
	Token   Token
	Name    string
	Methods []InterfaceMethod
}

func (i *InterfaceDecl) statementNode()       {}
func (i *InterfaceDecl) TokenLiteral() string { return i.Token.Literal }
func (i *InterfaceDecl) String() string {
	var methods []string
	for _, m := range i.Methods {
		methods = append(methods, "fn "+m.Name+"()")
	}
	return "interface " + i.Name + " { " + strings.Join(methods, "; ") + " }\n"
}

// Middleware Declaration: middleware name(req) { body }
type MiddlewareDecl struct {
	Token Token
	Name  string
	Param string // request parameter name
	Body  *BlockStmt
}

func (m *MiddlewareDecl) statementNode()       {}
func (m *MiddlewareDecl) TokenLiteral() string { return m.Token.Literal }
func (m *MiddlewareDecl) String() string {
	return "middleware " + m.Name + "(" + m.Param + ") " + m.Body.String() + "\n"
}

// DeclareModuleStmt: declare module "github.com/pkg" { fn Name() -> type }
type DeclareModuleFunc struct {
	Name        string
	Params      []string
	ParamTypes  []string
	ReturnType  string
	MultiReturn bool // true if Go function returns (value, error)
}

type DeclareModuleStmt struct {
	Token     Token
	PkgPath   string // Go import path
	Functions []DeclareModuleFunc
}

func (d *DeclareModuleStmt) statementNode()       {}
func (d *DeclareModuleStmt) TokenLiteral() string { return d.Token.Literal }
func (d *DeclareModuleStmt) String() string {
	return "declare module \"" + d.PkgPath + "\" { ... }\n"
}

// GoPackageImport: import alias from "github.com/pkg"
type GoPackageImport struct {
	Token Token
	Alias string // local alias name
	Path  string // Go package import path
}

func (g *GoPackageImport) statementNode()       {}
func (g *GoPackageImport) TokenLiteral() string { return g.Token.Literal }
func (g *GoPackageImport) String() string {
	return "import " + g.Alias + " from \"" + g.Path + "\"\n"
}

// WebSocket Statement: ws "/path" (conn) { body }
type WsStmt struct {
	Token Token
	Path  string
	Param string // connection parameter name
	Body  *BlockStmt
}

func (w *WsStmt) statementNode()       {}
func (w *WsStmt) TokenLiteral() string { return w.Token.Literal }
func (w *WsStmt) String() string {
	return "ws \"" + w.Path + "\" (" + w.Param + ") " + w.Body.String() + "\n"
}

// DestructureLetStmt: let { name, email } = expr
// Extracts named fields from a map or struct into local variables.
type DestructureLetStmt struct {
	Token  Token
	Fields []string   // field names to extract
	Value  Expression // the source expression
}

func (d *DestructureLetStmt) statementNode()       {}
func (d *DestructureLetStmt) TokenLiteral() string { return d.Token.Literal }
func (d *DestructureLetStmt) String() string {
	return "let { " + strings.Join(d.Fields, ", ") + " } = " + d.Value.String() + "\n"
}

// OptionalMemberExpr: user?.address?.city — returns nil if any part is nil
type OptionalMemberExpr struct {
	Token  Token
	Object Expression
	Field  string
}

func (o *OptionalMemberExpr) expressionNode()      {}
func (o *OptionalMemberExpr) TokenLiteral() string { return o.Token.Literal }
func (o *OptionalMemberExpr) String() string       { return o.Object.String() + "?." + o.Field }

// TypeAliasStmt: type UserID = int
type TypeAliasStmt struct {
	Token    Token
	Name     string
	BaseType string
}

func (t *TypeAliasStmt) statementNode()       {}
func (t *TypeAliasStmt) TokenLiteral() string { return t.Token.Literal }
func (t *TypeAliasStmt) String() string       { return "type " + t.Name + " = " + t.BaseType + "\n" }

// ValidateStmt: validate { required "db.host", required "db.port", optional "log.level" }
// Defines config keys that must exist at startup — fail fast if missing.
type ValidateStmt struct {
	Token    Token
	Required []string // config keys that must be present
	Optional []string // config keys that are allowed but not required
}

func (v *ValidateStmt) statementNode()       {}
func (v *ValidateStmt) TokenLiteral() string { return v.Token.Literal }
func (v *ValidateStmt) String() string       { return "validate { ... }\n" }

// Actor Declaration: actor Name(params) { body }
type ActorDecl struct {
	Token      Token
	Name       string
	Params     []string
	ParamTypes []string
	Body       *BlockStmt
}

func (a *ActorDecl) statementNode()       {}
func (a *ActorDecl) TokenLiteral() string { return a.Token.Literal }
func (a *ActorDecl) String() string {
	var params []string
	for i, name := range a.Params {
		params = append(params, name+": "+a.ParamTypes[i])
	}
	return "actor " + a.Name + "(" + strings.Join(params, ", ") + ") " + a.Body.String() + "\n"
}

// Workflow Declaration: workflow Name(param) { body }
type WorkflowDecl struct {
	Token Token
	Name  string
	Param string
	Body  *BlockStmt
}

func (w *WorkflowDecl) statementNode()       {}
func (w *WorkflowDecl) TokenLiteral() string { return w.Token.Literal }
func (w *WorkflowDecl) String() string {
	return "workflow " + w.Name + "(" + w.Param + ") " + w.Body.String() + "\n"
}

// Inject Statement: inject name: Interface
type InjectStmt struct {
	Token         Token
	Name          string
	InterfaceName string
}

func (i *InjectStmt) statementNode()       {}
func (i *InjectStmt) TokenLiteral() string { return i.Token.Literal }
func (i *InjectStmt) String() string {
	return "inject " + i.Name + ": " + i.InterfaceName + "\n"
}

// GraphQL Statement: graphql "/path" { body }
type GraphQLStmt struct {
	Token Token
	Path  string
	Body  *BlockStmt
}

func (g *GraphQLStmt) statementNode()       {}
func (g *GraphQLStmt) TokenLiteral() string { return g.Token.Literal }
func (g *GraphQLStmt) String() string {
	return "graphql " + g.Path + " " + g.Body.String() + "\n"
}

// Macro Statement: @name(args)
type MacroStmt struct {
	Token Token
	Name  string
	Args  []string
}

func (m *MacroStmt) statementNode()       {}
func (m *MacroStmt) TokenLiteral() string { return m.Token.Literal }
func (m *MacroStmt) String() string {
	return "@" + m.Name + "(" + strings.Join(m.Args, ", ") + ")\n"
}
