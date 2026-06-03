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

func (b *BrokerStmt) statementNode()       {}
func (b *BrokerStmt) TokenLiteral() string { return b.Token.Literal }
func (b *BrokerStmt) String() string       { return "broker " + b.Value.String() + "\n" }

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
	Middlewares []string // middleware names from "use [auth, logging]"
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
	TypeParams []string // generic type parameters: [T, U]
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
	Token    Token
	Pairs    map[string]Expression
	KeyOrder []string // preserves insertion order for deterministic codegen
	Spreads  []SpreadEntry // spread expressions in order
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
type ForStmt struct {
	Token      Token
	Variable   string     // loop variable name (empty for condition-based)
	Iterable   Expression // collection to iterate, or condition expression
	IsRange    bool       // true = for x in collection, false = for condition
	Body       *BlockStmt
}

func (f *ForStmt) statementNode()       {}
func (f *ForStmt) TokenLiteral() string { return f.Token.Literal }
func (f *ForStmt) String() string {
	if f.IsRange {
		return "for " + f.Variable + " in " + f.Iterable.String() + " " + f.Body.String() + "\n"
	}
	return "for " + f.Iterable.String() + " " + f.Body.String() + "\n"
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
	Token  Token
	Name   string
	Fields []StructField
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
