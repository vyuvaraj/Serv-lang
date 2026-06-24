package compiler

// scope represents a lexical scope for tracking variables and their map literals.
type escapeScope struct {
	parent         *escapeScope
	vars           map[string]*MapLiteral
	goroutine      int // unique ID of the concurrent context/goroutine
	isFuncBoundary bool
}

func newEscapeScope(parent *escapeScope, goroutine int) *escapeScope {
	return &escapeScope{
		parent:    parent,
		vars:      make(map[string]*MapLiteral),
		goroutine: goroutine,
	}
}

func (s *escapeScope) inFunction() bool {
	curr := s
	for curr != nil {
		if curr.isFuncBoundary {
			return true
		}
		curr = curr.parent
	}
	return false
}

func (s *escapeScope) lookup(name string) (*MapLiteral, int) {
	curr := s
	for curr != nil {
		if ml, ok := curr.vars[name]; ok {
			return ml, curr.goroutine
		}
		curr = curr.parent
	}
	return nil, -1
}

// AnalyzeMapConcurrency walks the AST and determines which map literals escape
// their creating goroutine and thus need SafeMap wrapping.
func AnalyzeMapConcurrency(program *Program) {
	goroutineCounter := 0
	
	// Helper to get a new unique goroutine ID
	nextGoroutine := func() int {
		goroutineCounter++
		return goroutineCounter
	}

	globalScope := newEscapeScope(nil, 0)
	
	// First pass: analyze statements at the global scope
	for _, stmt := range program.Statements {
		analyzeStmtEscape(stmt, globalScope, nextGoroutine)
	}
}

func analyzeStmtEscape(stmt Statement, scope *escapeScope, nextGoroutine func() int) {
	if stmt == nil {
		return
	}

	switch s := stmt.(type) {
	case *ExportStmt:
		analyzeStmtEscape(s.Inner, scope, nextGoroutine)

	case *LetStmt:
		// Analyze the value expression
		ml := findMapLiteralInExpr(s.Value, scope)
		if ml != nil {
			for _, name := range s.Names {
				scope.vars[name] = ml
			}
			if s.Name != "" {
				scope.vars[s.Name] = ml
			}
		}

	case *BlockStmt:
		inner := newEscapeScope(scope, scope.goroutine)
		for _, sub := range s.Statements {
			analyzeStmtEscape(sub, inner, nextGoroutine)
		}

	case *IfStmt:
		analyzeExprEscape(s.Condition, scope, nextGoroutine)
		analyzeStmtEscape(s.Body, scope, nextGoroutine)
		if s.ElseBody != nil {
			analyzeStmtEscape(s.ElseBody, scope, nextGoroutine)
		}

	case *ForStmt:
		analyzeExprEscape(s.Iterable, scope, nextGoroutine)
		inner := newEscapeScope(scope, scope.goroutine)
		if s.Variable != "" {
			inner.vars[s.Variable] = nil
		}
		if s.KeyVar != "" {
			inner.vars[s.KeyVar] = nil
		}
		analyzeStmtEscape(s.Body, inner, nextGoroutine)

	case *FnDecl:
		// Functions start a new scope but inherit global variables.
		inner := newEscapeScope(scope, scope.goroutine)
		inner.isFuncBoundary = true
		for _, p := range s.Params {
			inner.vars[p] = nil
		}
		analyzeStmtEscape(s.Body, inner, nextGoroutine)

	case *MethodDecl:
		inner := newEscapeScope(scope, scope.goroutine)
		inner.isFuncBoundary = true
		for _, p := range s.Params {
			inner.vars[p] = nil
		}
		analyzeStmtEscape(s.Body, inner, nextGoroutine)

	case *ActorDecl:
		actorScope := newEscapeScope(scope, scope.goroutine)
		for _, p := range s.Params {
			actorScope.vars[p] = nil
		}
		analyzeStmtEscape(s.Body, actorScope, nextGoroutine)

	// Concurrent boundaries:
	case *RouteStmt:
		// Route handlers run concurrently per request
		routeScope := newEscapeScope(scope, nextGoroutine())
		routeScope.vars[s.Param] = nil
		analyzeStmtEscape(s.Body, routeScope, nextGoroutine)

	case *EveryStmt:
		everyScope := newEscapeScope(scope, nextGoroutine())
		analyzeStmtEscape(s.Body, everyScope, nextGoroutine)

	case *CronStmt:
		cronScope := newEscapeScope(scope, nextGoroutine())
		analyzeStmtEscape(s.Body, cronScope, nextGoroutine)

	case *SubscribeStmt:
		subScope := newEscapeScope(scope, nextGoroutine())
		subScope.vars[s.Param] = nil
		analyzeStmtEscape(s.Body, subScope, nextGoroutine)

	case *WsStmt:
		wsScope := newEscapeScope(scope, nextGoroutine())
		wsScope.vars[s.Param] = nil
		analyzeStmtEscape(s.Body, wsScope, nextGoroutine)

	case *SpawnStmt:
		// Spawn statement runs its call in a background goroutine
		spawnScope := newEscapeScope(scope, nextGoroutine())
		analyzeExprEscape(s.Call, spawnScope, nextGoroutine)
		if s.Limit != nil {
			analyzeExprEscape(s.Limit, scope, nextGoroutine)
		}

	case *ReturnStmt:
		if s.Value != nil {
			ml := findMapLiteralInExpr(s.Value, scope)
			if ml != nil {
				// Any map returned from a function is marked concurrent to be safe.
				// Maps returned directly from route handlers do not escape to other user code.
				if scope.inFunction() {
					ml.ConcurrentMap = true
				}
			} else {
				analyzeExprEscape(s.Value, scope, nextGoroutine)
			}
		}

	case *ExprStmt:
		analyzeExprEscape(s.Value, scope, nextGoroutine)

	case *TryCatchStmt:
		analyzeStmtEscape(s.TryBody, scope, nextGoroutine)
		catchScope := newEscapeScope(scope, scope.goroutine)
		if s.Param != "" {
			catchScope.vars[s.Param] = nil
		}
		analyzeStmtEscape(s.CatchBody, catchScope, nextGoroutine)

	case *MatchStmt:
		analyzeExprEscape(s.Value, scope, nextGoroutine)
		for _, c := range s.Cases {
			analyzeStmtEscape(c.Body, scope, nextGoroutine)
		}
	}
}

func analyzeExprEscape(expr Expression, scope *escapeScope, nextGoroutine func() int) {
	if expr == nil {
		return
	}

	switch e := expr.(type) {
	case *Identifier:
		// Resolve identifier
		if ml, declG := scope.lookup(e.Value); ml != nil {
			// If accessed in a different goroutine scope, mark as concurrent
			if declG != scope.goroutine {
				ml.ConcurrentMap = true
			}
		}

	case *MapLiteral:
		for _, v := range e.Pairs {
			analyzeExprEscape(v, scope, nextGoroutine)
		}

	case *CallExpr:
		analyzeExprEscape(e.Function, scope, nextGoroutine)
		for _, arg := range e.Arguments {
			// If an argument is an identifier pointing to a MapLiteral,
			// or a direct MapLiteral, since we pass it to a function call,
			// to be safe we mark it concurrent.
			ml := findMapLiteralInExpr(arg, scope)
			if ml != nil {
				ml.ConcurrentMap = true
			} else {
				analyzeExprEscape(arg, scope, nextGoroutine)
			}
		}

	case *MemberExpr:
		analyzeExprEscape(e.Object, scope, nextGoroutine)

	case *IndexExpr:
		analyzeExprEscape(e.Left, scope, nextGoroutine)
		analyzeExprEscape(e.Index, scope, nextGoroutine)

	case *InfixExpr:
		analyzeExprEscape(e.Left, scope, nextGoroutine)
		analyzeExprEscape(e.Right, scope, nextGoroutine)

	case *PrefixExpr:
		analyzeExprEscape(e.Right, scope, nextGoroutine)

	case *SliceExpr:
		analyzeExprEscape(e.Left, scope, nextGoroutine)
		if e.Start != nil {
			analyzeExprEscape(e.Start, scope, nextGoroutine)
		}
		if e.End != nil {
			analyzeExprEscape(e.End, scope, nextGoroutine)
		}

	case *AssignExpr:
		ml := findMapLiteralInExpr(e.Value, scope)
		if ml != nil {
			if existing, _ := scope.lookup(e.Name); existing != nil {
				if existing.ConcurrentMap {
					ml.ConcurrentMap = true
				}
			}
		} else {
			analyzeExprEscape(e.Value, scope, nextGoroutine)
		}

	case *MemberAssignExpr:
		analyzeExprEscape(e.Object, scope, nextGoroutine)
		analyzeExprEscape(e.Value, scope, nextGoroutine)

	case *IndexAssignExpr:
		analyzeExprEscape(e.Left, scope, nextGoroutine)
		analyzeExprEscape(e.Value, scope, nextGoroutine)

	case *FnLiteral:
		// Closures / Arrow Functions are function boundaries
		fnScope := newEscapeScope(scope, scope.goroutine)
		fnScope.isFuncBoundary = true
		for _, p := range e.Params {
			fnScope.vars[p] = nil
		}
		if e.Body != nil {
			for _, s := range e.Body.Statements {
				analyzeStmtEscape(s, fnScope, nextGoroutine)
			}
		} else if e.ArrowExpr != nil {
			analyzeExprEscape(e.ArrowExpr, fnScope, nextGoroutine)
		}

	case *ArrayLiteral:
		for _, el := range e.Elements {
			ml := findMapLiteralInExpr(el, scope)
			if ml != nil {
				ml.ConcurrentMap = true
			} else {
				analyzeExprEscape(el, scope, nextGoroutine)
			}
		}
	case *SpawnExpr:
		spawnScope := newEscapeScope(scope, nextGoroutine())
		analyzeExprEscape(e.Call, spawnScope, nextGoroutine)
		if e.Limit != nil {
			analyzeExprEscape(e.Limit, scope, nextGoroutine)
		}
	}
}

// findMapLiteralInExpr recursively checks if an expression resolves to a MapLiteral.
func findMapLiteralInExpr(expr Expression, scope *escapeScope) *MapLiteral {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *MapLiteral:
		return e
	case *Identifier:
		ml, _ := scope.lookup(e.Value)
		return ml
	}
	return nil
}
