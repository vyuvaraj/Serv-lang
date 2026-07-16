package compiler

import (
	"fmt"
	"net"
	"strings"
	"time"
)

// Diagnostic represents a compiler warning or error from static analysis.
type Diagnostic struct {
	Line     int
	Col      int
	Severity string // "warning" or "error"
	Message  string
}

// Analyze performs static analysis on a parsed program and returns diagnostics.
// This includes:
// - Unused variable detection
// - Missing return detection for typed functions
// - Basic type mismatch checking for function calls
// - Unreachable code detection
// - Dead import detection
func Analyze(program *Program) []Diagnostic {
	var diags []Diagnostic

	// Collect all function/struct/let names defined at top level (for dead import detection)
	definedSymbols := make(map[string]bool)
	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *FnDecl:
			definedSymbols[s.Name] = true
		case *StructDecl:
			definedSymbols[s.Name] = true
		case *LetStmt:
			definedSymbols[s.Name] = true
		case *EnumStmt:
			definedSymbols[s.Name] = true
		case *ExportStmt:
			name := statementNameForAnalysis(s.Inner)
			if name != "" {
				definedSymbols[name] = true
			}
		}
	}

	// Check for dead imports
	diags = append(diags, analyzeDeadImports(program)...)

	// Check for SQL injection risks
	diags = append(diags, analyzeSQLInjection(program)...)

	// ARCH.8: Bounded context domain boundaries checks
	diags = append(diags, analyzeDomainBoundaries(program)...)

	// LANG.8: Interface satisfaction checking
	diags = append(diags, analyzeInterfaceSatisfaction(program)...)

	// LANG.3: Compiler plugin system
	diags = append(diags, LoadAndRunPlugins(program, ".")...)

	// Topic schema validation
	diags = append(diags, analyzeTopicSchemas(program)...)

	// CD.47/77 & CD.46/78: Infrastructure reachability & cross-service dead code detection
	diags = append(diags, analyzeInfrastructureReachability(program)...)
	diags = append(diags, analyzeDeadRoutes(program)...)

	for _, stmt := range program.Statements {
		diags = append(diags, analyzeStatement(stmt, program)...)
	}

	return diags
}

// statementNameForAnalysis extracts the declared name from a statement.
func statementNameForAnalysis(stmt Statement) string {
	switch s := stmt.(type) {
	case *FnDecl:
		return s.Name
	case *StructDecl:
		return s.Name
	case *LetStmt:
		return s.Name
	case *EnumStmt:
		return s.Name
	default:
		return ""
	}
}

func analyzeStatement(stmt Statement, program *Program) []Diagnostic {
	switch s := stmt.(type) {
	case *FnDecl:
		return analyzeFnDecl(s, program)
	case *ExportStmt:
		return analyzeStatement(s.Inner, program)
	case *RouteStmt:
		var diags []Diagnostic
		diags = append(diags, analyzeBlock(s.Body, nil)...)
		diags = append(diags, analyzeRouteContract(s, program)...)
		return diags
	case *EveryStmt:
		return analyzeBlock(s.Body, nil)
	case *CronStmt:
		return analyzeBlock(s.Body, nil)
	case *SubscribeStmt:
		return analyzeBlock(s.Body, nil)
	case *MiddlewareDecl:
		return analyzeBlock(s.Body, nil)
	case *TestStmt:
		return analyzeBlock(s.Body, nil)
	case *AppStmt:
		var diags []Diagnostic
		diags = append(diags, analyzeBlock(s.Body, nil)...)
		for _, subStmt := range s.Body.Statements {
			diags = append(diags, analyzeStatement(subStmt, program)...)
		}
		return diags
	case *AgentDecl:
		return nil
	case *MeshStmt:
		return analyzeBlock(s.Body, nil)
	case *OnStmt:
		return analyzeBlock(s.Body, nil)
	case *LockStmt:
		return analyzeBlock(s.Body, nil)
	case *BucketStmt:
		return analyzeBlock(s.Body, nil)
	case *GateStmt:
		return analyzeBlock(s.Body, nil)
	case *JobStmt:
		return analyzeBlock(s.Body, nil)
	case *RagStmt:
		return analyzeBlock(s.Body, nil)
	case *EmitStmt:
		return nil
	case *CommandDecl:
		return analyzeBlock(s.Body, nil)
	case *EventStoreStmt:
		var diags []Diagnostic
		for _, c := range s.Commands {
			if d := analyzeBlock(c.Body, nil); len(d) > 0 {
				diags = append(diags, d...)
			}
		}
		for _, h := range s.Handlers {
			if d := analyzeBlock(h.Body, nil); len(d) > 0 {
				diags = append(diags, d...)
			}
		}
		return diags
	case *WorkflowDecl:
		var diags []Diagnostic
		diags = append(diags, analyzeBlock(s.Body, nil)...)
		diags = append(diags, analyzeWorkflowDAG(s)...)
		return diags
	}
	return nil
}

func analyzeFnDecl(fn *FnDecl, program *Program) []Diagnostic {
	var diags []Diagnostic

	// Unused variable detection within function body
	diags = append(diags, analyzeUnusedVars(fn)...)

	// Missing return detection
	if fn.ReturnType != "" && fn.ReturnType != "nil" {
		if !blockHasReturn(fn.Body) {
			diags = append(diags, Diagnostic{
				Line:     fn.Token.Line,
				Col:      fn.Token.Col,
				Severity: "warning",
				Message:  fmt.Sprintf("function '%s' declares return type '%s' but may not return a value on all paths", fn.Name, fn.ReturnType),
			})
		}
	}

	// Unreachable code detection
	diags = append(diags, analyzeUnreachableCode(fn.Body)...)

	// Basic type checking for function calls in the body
	diags = append(diags, analyzeTypeMismatches(fn, program)...)

	return diags
}

// FormatAnalysisDiagnostics formats analysis diagnostics for display.
func FormatAnalysisDiagnostics(diags []Diagnostic, source string) string {
	if len(diags) == 0 {
		return ""
	}

	lines := strings.Split(source, "\n")
	var out strings.Builder

	for _, d := range diags {
		prefix := "warning"
		if d.Severity == "error" {
			prefix = "error"
		}
		out.WriteString(fmt.Sprintf("  %s: %s\n", prefix, d.Message))

		if d.Line > 0 && d.Line <= len(lines) {
			srcLine := lines[d.Line-1]
			lineNum := fmt.Sprintf(" %d | ", d.Line)
			out.WriteString(fmt.Sprintf("  %s%s\n", lineNum, srcLine))
			if d.Col > 0 {
				padding := strings.Repeat(" ", len(lineNum)+d.Col-1)
				out.WriteString(fmt.Sprintf("  %s^\n", padding))
			}
		}
		out.WriteString("\n")
	}

	return out.String()
}

func analyzeDomainBoundaries(program *Program) []Diagnostic {
	var diags []Diagnostic

	for _, stmt := range program.Statements {
		switch s := stmt.(type) {
		case *RouteStmt:
			diags = append(diags, checkBlockDomainBoundaries(s.Body, s.Path, s.Token.Line, s.Token.Col)...)
		case *EveryStmt:
			diags = append(diags, checkBlockDomainBoundaries(s.Body, "cron-every", s.Token.Line, s.Token.Col)...)
		case *CronStmt:
			diags = append(diags, checkBlockDomainBoundaries(s.Body, "cron-schedule", s.Token.Line, s.Token.Col)...)
		case *SubscribeStmt:
			diags = append(diags, checkBlockDomainBoundaries(s.Body, "subscribe-"+s.Topic.String(), s.Token.Line, s.Token.Col)...)
		}
	}

	return diags
}

func checkBlockDomainBoundaries(block *BlockStmt, contextName string, line, col int) []Diagnostic {
	var diags []Diagnostic
	if block == nil {
		return diags
	}

	for _, stmt := range block.Statements {
		diags = append(diags, checkStmtDomainBoundaries(stmt, contextName, line, col)...)
	}

	return diags
}

func checkStmtDomainBoundaries(stmt Statement, contextName string, line, col int) []Diagnostic {
	var diags []Diagnostic
	if isNil(stmt) {
		return diags
	}

	switch s := stmt.(type) {
	case *BlockStmt:
		diags = append(diags, checkBlockDomainBoundaries(s, contextName, line, col)...)
	case *LetStmt:
		diags = append(diags, checkExprDomainBoundaries(s.Value, contextName, line, col)...)
	case *ReturnStmt:
		diags = append(diags, checkExprDomainBoundaries(s.Value, contextName, line, col)...)
	case *ExprStmt:
		diags = append(diags, checkExprDomainBoundaries(s.Value, contextName, line, col)...)
	case *IfStmt:
		diags = append(diags, checkExprDomainBoundaries(s.Condition, contextName, line, col)...)
		diags = append(diags, checkStmtDomainBoundaries(s.Body, contextName, line, col)...)
		diags = append(diags, checkStmtDomainBoundaries(s.ElseBody, contextName, line, col)...)
	case *ForStmt:
		diags = append(diags, checkStmtDomainBoundaries(s.Body, contextName, line, col)...)
	case *TryCatchStmt:
		diags = append(diags, checkStmtDomainBoundaries(s.TryBody, contextName, line, col)...)
		diags = append(diags, checkStmtDomainBoundaries(s.CatchBody, contextName, line, col)...)
	case *MatchStmt:
		for _, c := range s.Cases {
			diags = append(diags, checkStmtDomainBoundaries(c.Body, contextName, line, col)...)
		}
	}

	return diags
}

func checkExprDomainBoundaries(expr Expression, contextName string, line, col int) []Diagnostic {
	var diags []Diagnostic
	if isNil(expr) {
		return diags
	}

	switch e := expr.(type) {
	case *CallExpr:
		if ident, ok := e.Function.(*Identifier); ok {
			fnName := ident.Value
			if strings.HasPrefix(fnName, "auth_private_") && !strings.Contains(contextName, "auth") {
				diags = append(diags, Diagnostic{
					Line:     line,
					Col:      col,
					Severity: "warning",
					Message:  fmt.Sprintf("Domain boundary violation: handler for '%s' directly invokes internal helper '%s' belonging to another domain context.", contextName, fnName),
				})
			}
			if strings.HasPrefix(fnName, "mail_internal_") && !strings.Contains(contextName, "mail") {
				diags = append(diags, Diagnostic{
					Line:     line,
					Col:      col,
					Severity: "warning",
					Message:  fmt.Sprintf("Domain boundary violation: handler for '%s' directly invokes internal helper '%s' belonging to another domain context.", contextName, fnName),
				})
			}
		}

		for _, arg := range e.Arguments {
			diags = append(diags, checkExprDomainBoundaries(arg, contextName, line, col)...)
		}

	case *InfixExpr:
		diags = append(diags, checkExprDomainBoundaries(e.Left, contextName, line, col)...)
		diags = append(diags, checkExprDomainBoundaries(e.Right, contextName, line, col)...)

	case *PrefixExpr:
		diags = append(diags, checkExprDomainBoundaries(e.Right, contextName, line, col)...)

	case *IndexExpr:
		diags = append(diags, checkExprDomainBoundaries(e.Left, contextName, line, col)...)
		diags = append(diags, checkExprDomainBoundaries(e.Index, contextName, line, col)...)

	case *MemberExpr:
		diags = append(diags, checkExprDomainBoundaries(e.Object, contextName, line, col)...)

	case *OptionalMemberExpr:
		diags = append(diags, checkExprDomainBoundaries(e.Object, contextName, line, col)...)

	case *MapLiteral:
		for _, val := range e.Pairs {
			diags = append(diags, checkExprDomainBoundaries(val, contextName, line, col)...)
		}

	case *ArrayLiteral:
		for _, el := range e.Elements {
			diags = append(diags, checkExprDomainBoundaries(el, contextName, line, col)...)
		}
	}

	return diags
}

func analyzeRouteContract(route *RouteStmt, program *Program) []Diagnostic {
	var diags []Diagnostic
	if route.ReturnType == "" {
		return diags
	}

	localVars := make(map[string]string)

	var walk func(stmt Statement)
	walk = func(stmt Statement) {
		if stmt == nil {
			return
		}
		switch s := stmt.(type) {
		case *BlockStmt:
			for _, inner := range s.Statements {
				walk(inner)
			}
		case *LetStmt:
			t := resolveExpressionType(s.Value, program, localVars)
			localVars[s.Name] = t
		case *IfStmt:
			if s.Body != nil {
				walk(s.Body)
			}
			if s.ElseBody != nil {
				walk(s.ElseBody)
			}
		case *ForStmt:
			if s.Body != nil {
				walk(s.Body)
			}
		case *ReturnStmt:
			retType := resolveExpressionType(s.Value, program, localVars)
			if retType != "interface{}" && retType != "nil" && retType != route.ReturnType {
				diags = append(diags, Diagnostic{
					Line:     s.Token.Line,
					Col:      s.Token.Col,
					Severity: "error",
					Message:  fmt.Sprintf("route declares return type '%s' but returns value of type '%s'", route.ReturnType, retType),
				})
			}
		}
	}

	walk(route.Body)
	return diags
}

func resolveExpressionType(expr Expression, program *Program, localVars map[string]string) string {
	if expr == nil {
		return "nil"
	}
	switch e := expr.(type) {
	case *IntegerLiteral:
		return "int"
	case *FloatLiteral:
		return "float64"
	case *StringLiteral, *FStringLiteral, *DurationLiteral:
		return "string"
	case *BooleanLiteral:
		return "bool"
	case *NilLiteral:
		return "nil"
	case *StructLiteral:
		return e.TypeName
	case *Identifier:
		if t, ok := localVars[e.Value]; ok {
			return t
		}
		return "interface{}"
	case *CallExpr:
		if ident, ok := e.Function.(*Identifier); ok {
			for _, stmt := range program.Statements {
				if f, ok := stmt.(*FnDecl); ok && f.Name == ident.Value {
					return f.ReturnType
				}
				if exp, ok := stmt.(*ExportStmt); ok {
					if f, ok := exp.Inner.(*FnDecl); ok && f.Name == ident.Value {
						return f.ReturnType
					}
				}
			}
		}
	}
	return "interface{}"
}

func analyzeWorkflowDAG(wf *WorkflowDecl) []Diagnostic {
	var diags []Diagnostic

	// Map of varName -> list of dependencies (variables it depends on)
	adj := make(map[string][]string)

	var walk func(stmt Statement)
	walk = func(stmt Statement) {
		if stmt == nil {
			return
		}
		switch s := stmt.(type) {
		case *BlockStmt:
			for _, inner := range s.Statements {
				walk(inner)
			}
		case *LetStmt:
			deps := findReferencedIdents(s.Value)
			adj[s.Name] = deps
		case *IfStmt:
			if s.Body != nil {
				walk(s.Body)
			}
			if s.ElseBody != nil {
				walk(s.ElseBody)
			}
		case *ForStmt:
			if s.Body != nil {
				walk(s.Body)
			}
		}
	}
	walk(wf.Body)

	// Detect cycles using DFS (three-color graph search: 0=unvisited, 1=visiting, 2=visited)
	visited := make(map[string]int)

	var detectCycle func(node string, path []string) []string
	detectCycle = func(node string, path []string) []string {
		visited[node] = 1 // visiting
		path = append(path, node)

		for _, dep := range adj[node] {
			if visited[dep] == 1 {
				for i, p := range path {
					if p == dep {
						return append(path[i:], dep)
					}
				}
				return []string{dep, node, dep}
			}
			if visited[dep] == 0 {
				if cycle := detectCycle(dep, path); len(cycle) > 0 {
					return cycle
				}
			}
		}

		visited[node] = 2 // visited
		return nil
	}

	for node := range adj {
		if visited[node] == 0 {
			if cycle := detectCycle(node, nil); len(cycle) > 0 {
				diags = append(diags, Diagnostic{
					Line:     wf.Token.Line,
					Col:      wf.Token.Col,
					Severity: "error",
					Message:  fmt.Sprintf("Workflow '%s' contains cyclic step dependency: %s", wf.Name, strings.Join(cycle, " -> ")),
				})
				break
			}
		}
	}

	return diags
}

func findReferencedIdents(expr Expression) []string {
	var idents []string
	if expr == nil {
		return idents
	}
	switch e := expr.(type) {
	case *Identifier:
		idents = append(idents, e.Value)
	case *AwaitExpr:
		idents = append(idents, findReferencedIdents(e.Value)...)
	case *CallExpr:
		idents = append(idents, findReferencedIdents(e.Function)...)
		for _, arg := range e.Arguments {
			idents = append(idents, findReferencedIdents(arg)...)
		}
	case *InfixExpr:
		idents = append(idents, findReferencedIdents(e.Left)...)
		idents = append(idents, findReferencedIdents(e.Right)...)
	case *PrefixExpr:
		idents = append(idents, findReferencedIdents(e.Right)...)
	case *IndexExpr:
		idents = append(idents, findReferencedIdents(e.Left)...)
		idents = append(idents, findReferencedIdents(e.Index)...)
	case *MemberExpr:
		idents = append(idents, findReferencedIdents(e.Object)...)
	}
	return idents
}

// Global flag to bypass network ping in tests/environments
var SkipInfraReachability = false

func analyzeInfrastructureReachability(program *Program) []Diagnostic {
	var diags []Diagnostic
	if SkipInfraReachability {
		return diags
	}

	// Helper to extract string values
	getStr := func(expr Expression) string {
		if s, ok := expr.(*StringLiteral); ok {
			return s.Value
		}
		return ""
	}

	for _, stmt := range program.Statements {
		var name, value string
		var token Token
		switch s := stmt.(type) {
		case *DatabaseStmt:
			name = "database"
			value = getStr(s.Value)
			token = s.Token
		case *BrokerStmt:
			name = "broker"
			value = getStr(s.Value)
			token = s.Token
		case *CacheStmt:
			name = "cache"
			value = getStr(s.Value)
			token = s.Token
		}

		if value != "" && value != "in-memory" {
			// Extract host/port from connection URI schemas
			addr := value
			if strings.Contains(addr, "://") {
				parts := strings.SplitN(addr, "://", 2)
				addr = parts[1]
			}
			if strings.Contains(addr, "/") {
				addr = strings.Split(addr, "/")[0]
			}
			if !strings.Contains(addr, ":") {
				// Apply default standard service port placeholders if missing
				switch name {
				case "database":
					addr += ":5432" // postgres
				case "broker":
					addr += ":61613" // stomp
				case "cache":
					addr += ":6379" // redis
				}
			}

			// Perform network dial reachability check
			conn, err := net.DialTimeout("tcp", addr, 100*time.Millisecond)
			if err != nil {
				diags = append(diags, Diagnostic{
					Line:     token.Line,
					Col:      token.Col,
					Severity: "warning",
					Message:  fmt.Sprintf("Infrastructure reachability warning: %s target '%s' is currently unreachable", name, value),
				})
			} else {
				conn.Close()
			}
		}
	}
	return diags
}

func analyzeDeadRoutes(program *Program) []Diagnostic {
	var diags []Diagnostic

	// 1. Gather all defined service routes
	type routeKey struct {
		service string
		path    string
		token   Token
	}
	declared := make(map[string]routeKey)
	for _, mainStmt := range program.Statements {
		if app, isApp := mainStmt.(*AppStmt); isApp {
			for _, appStmt := range app.Body.Statements {
				if r, isR := appStmt.(*RouteStmt); isR {
					key := app.Name + r.Path
					declared[key] = routeKey{service: app.Name, path: r.Path, token: r.Token}
				}
			}
		}
	}

	// 2. Identify all outbound serv:// endpoint calls in workspace
	called := make(map[string]bool)
	var walk func(stmt Statement)
	walk = func(stmt Statement) {
		if stmt == nil {
			return
		}
		switch s := stmt.(type) {
		case *BlockStmt:
			for _, inner := range s.Statements {
				walk(inner)
			}
		case *IfStmt:
			walk(s.Body)
			walk(s.ElseBody)
		case *ForStmt:
			walk(s.Body)
		case *AppStmt:
			walk(s.Body)
		case *RouteStmt:
			walk(s.Body)
		case *FnDecl:
			walk(s.Body)
		case *ExportStmt:
			walk(s.Inner)
		case *LetStmt:
			if call, isCall := s.Value.(*CallExpr); isCall {
				if ident, isIdent := call.Function.(*MemberExpr); isIdent {
					if obj, isObj := ident.Object.(*Identifier); isObj && obj.Value == "http" && (ident.Field == "get" || ident.Field == "post") {
						if len(call.Arguments) > 0 {
							if strLit, isStr := call.Arguments[0].(*StringLiteral); isStr && strings.HasPrefix(strLit.Value, "serv://") {
								uriVal := strings.TrimPrefix(strLit.Value, "serv://")
								parts := strings.SplitN(uriVal, "/", 2)
								if len(parts) == 2 {
									called[parts[0]+"/"+parts[1]] = true
								}
							}
						}
					}
				}
			}
		}
	}

	for _, mainStmt := range program.Statements {
		walk(mainStmt)
	}

	// 3. Flag unused routes
	for _, r := range declared {
		if !called[r.service+r.path] {
			diags = append(diags, Diagnostic{
				Line:     r.token.Line,
				Col:      r.token.Col,
				Severity: "warning",
				Message:  fmt.Sprintf("Dead route warning: Route '%s' declared in service '%s' is never called by other services", r.path, r.service),
			})
		}
	}

	return diags
}
