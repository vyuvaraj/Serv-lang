package compiler

// GenerateMainFunc creates the entrypoint function main() which runs runtime.StartServer()
func (c *Codegen) GenerateMainFunc() string {
	return `func main() {
	runtime.StartServer()
}
`
}

// GenerateHelpers returns utility functions needed by generated code (if/for/multi-return support).
func (c *Codegen) GenerateHelpers() string {
	return `
// isTruthy evaluates truthiness of an interface{} value.
func isTruthy(v interface{}) bool {
	if v == nil {
		return false
	}
	switch val := v.(type) {
	case bool:
		return val
	case int:
		return val != 0
	case int64:
		return val != 0
	case float64:
		return val != 0
	case string:
		return val != ""
	default:
		return true
	}
}

// toSlice coerces an interface{} to []interface{} for range iteration.
func toSlice(v interface{}) []interface{} {
	if v == nil {
		return nil
	}
	if s, ok := v.([]interface{}); ok {
		return s
	}
	return nil
}

// multiReturn splits a value into (result, error) for multi-return assignment.
// If the value is a [2]interface{} tuple, it unpacks it.
// If the value is an error, returns (nil, error).
// Otherwise returns (value, nil).
func multiReturn(v interface{}) (interface{}, interface{}) {
	if v == nil {
		return nil, nil
	}
	if tuple, ok := v.([2]interface{}); ok {
		return tuple[0], tuple[1]
	}
	if err, ok := v.(error); ok {
		return nil, err.Error()
	}
	return v, nil
}

// safeCall executes a function and catches any panic, returning (result, error).
// Used by multi-return let statements: let val, err = expr
func safeCall(fn func() interface{}) (interface{}, interface{}) {
	var result interface{}
	var errVal interface{}
	func() {
		defer func() {
			if r := recover(); r != nil {
				errVal = fmt.Sprint(r)
			}
		}()
		result = fn()
	}()
	// If the result is already a tuple (from safe runtime functions), unpack it
	if tuple, ok := result.([2]interface{}); ok {
		return tuple[0], tuple[1]
	}
	return result, errVal
}
`
}

func hasConcurrency(node Node) bool {
	if node == nil {
		return false
	}
	switch s := node.(type) {
	case *SpawnStmt, *PublishStmt:
		return true
	case *BlockStmt:
		for _, sub := range s.Statements {
			if hasConcurrency(sub) {
				return true
			}
		}
	case *TryCatchStmt:
		return hasConcurrency(s.TryBody) || hasConcurrency(s.CatchBody)
	case *MatchStmt:
		for _, cs := range s.Cases {
			if hasConcurrency(cs.Body) {
				return true
			}
		}
	case *IfStmt:
		if hasConcurrency(s.Body) {
			return true
		}
		if s.ElseBody != nil {
			return hasConcurrency(s.ElseBody)
		}
	case *ForStmt:
		return hasConcurrency(s.Body)
	case *LetStmt:
		return hasConcurrency(s.Value)
	case *DestructureLetStmt:
		return hasConcurrency(s.Value)
	case *ExprStmt:
		return hasConcurrency(s.Value)
	case *ReturnStmt:
		return hasConcurrency(s.Value)
	}
	return false
}

// stmtToken extracts the Token from a statement for source line tracking.
func stmtToken(stmt Statement) Token {
	switch s := stmt.(type) {
	case *LetStmt:
		return s.Token
	case *FnDecl:
		return s.Token
	case *RouteStmt:
		return s.Token
	case *EveryStmt:
		return s.Token
	case *CronStmt:
		return s.Token
	case *SubscribeStmt:
		return s.Token
	case *PublishStmt:
		return s.Token
	case *SpawnStmt:
		return s.Token
	case *ExternFnStmt:
		return s.Token
	case *BrokerStmt:
		return s.Token
	case *ServerStmt:
		return s.Token
	case *DatabaseStmt:
		return s.Token
	case *CacheStmt:
		return s.Token
	case *TryCatchStmt:
		return s.Token
	case *MatchStmt:
		return s.Token
	case *TestStmt:
		return s.Token
	case *EnumStmt:
		return s.Token
	case *ToolStmt:
		return s.Token
	case *MigrationStmt:
		return s.Token
	case *IfStmt:
		return s.Token
	case *ForStmt:
		return s.Token
	case *StructDecl:
		return s.Token
	case *MethodDecl:
		return s.Token
	case *InterfaceDecl:
		return s.Token
	case *MiddlewareDecl:
		return s.Token
	case *DeclareModuleStmt:
		return s.Token
	case *GoPackageImport:
		return s.Token
	case *WsStmt:
		return s.Token
	case *ExportStmt:
		return stmtToken(s.Inner)
	case *ExprStmt:
		return s.Token
	case *ReturnStmt:
		return s.Token
	case *DestructureLetStmt:
		return s.Token
	case *TypeAliasStmt:
		return s.Token
	case *ValidateStmt:
		return s.Token
	case *BreakStmt:
		return s.Token
	case *ContinueStmt:
		return s.Token
	case *BeforeEachStmt:
		return s.Token
	case *AfterEachStmt:
		return s.Token
	default:
		return Token{}
	}
}
