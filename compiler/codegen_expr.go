package compiler

import (
	"fmt"
	"strings"
)

func (c *Codegen) genExpression(expr Expression) (string, error) {
	switch e := expr.(type) {
	case *Identifier:
		// Map standard builtins like log.info, metric.inc, http.get
		// Wait, they are parsed as MemberExpr if they use dot notation.
		// If it's a simple identifier, output it
		return e.Value, nil

	case *StringLiteral:
		return fmt.Sprintf("%q", e.Value), nil

	case *IntegerLiteral:
		return fmt.Sprintf("%d", e.Value), nil

	case *FloatLiteral:
		return fmt.Sprintf("%f", e.Value), nil

	case *ArrayLiteral:
		var elements []string
		for _, el := range e.Elements {
			elStr, err := c.genExpression(el)
			if err != nil {
				return "", err
			}
			elements = append(elements, elStr)
		}
		return fmt.Sprintf("[]interface{}{%s}", strings.Join(elements, ", ")), nil

	case *DurationLiteral:
		return fmt.Sprintf("%q", e.Value), nil

	case *OptionalMemberExpr:
		objStr, err := c.genExpression(e.Object)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("runtime.MemberAccess(%s, %q)", objStr, e.Field), nil

	case *MemberExpr:
		objStr, err := c.genExpression(e.Object)
		if err != nil {
			return "", err
		}

		// Self field access: self.field -> self.Field (direct struct access)
		if _, isSelf := e.Object.(*SelfExpr); isSelf {
			return fmt.Sprintf("self.%s", capitalizeFirst(e.Field)), nil
		}

		// Struct instance field access: if the object is a known struct type variable
		if objType, ok := c.varTypes[objStr]; ok && objType != "interface{}" && objType != "int" && objType != "string" && objType != "float64" && objType != "bool" && objType != "[]interface{}" {
			return fmt.Sprintf("%s.%s", objStr, capitalizeFirst(e.Field)), nil
		}

		// Go package alias: uuid.New -> uuid.New (direct Go package call)
		if goPkg, ok := c.goPackageAliases[objStr]; ok {
			return fmt.Sprintf("%s.%s", goPkg, e.Field), nil
		}

		// Builtin conversions
		if objStr == "time" {
			switch e.Field {
			case "now":
				return "func() interface{} { return time.Now().Format(time.RFC3339) }", nil
			case "sleep":
				return "runtime.Sleep", nil
			case "unix":
				c.imports[`"time"`] = true
				return "func() int { return int(time.Now().Unix()) }", nil
			}
		}

		if objStr == "log" {
			switch e.Field {
			case "info":
				return "runtime.LogInfo", nil
			case "warn":
				return "runtime.LogWarn", nil
			case "error":
				return "runtime.LogError", nil
			case "debug":
				return "runtime.LogDebug", nil
			case "with":
				return "runtime.LogWith", nil
			case "fields":
				return "runtime.LogFields", nil
			case "setLevel":
				return "runtime.LogSetLevel", nil
			case "getLevel":
				return "runtime.LogGetLevel", nil
			}
		}
		if objStr == "metric" {
			switch e.Field {
			case "inc":
				return "runtime.MetricInc", nil
			case "gauge":
				return "runtime.MetricGauge", nil
			}
		}
		if objStr == "http" {
			switch e.Field {
			case "get":
				return "runtime.HTTPGet", nil
			case "post":
				return "runtime.HTTPPost", nil
			}
		}
		if objStr == "json" {
			switch e.Field {
			case "parse":
				return "runtime.JSONParse", nil
			case "stringify":
				return "runtime.JSONStringify", nil
			}
		}
		if objStr == "db" {
			switch e.Field {
			case "query":
				return "runtime.DBQuery", nil
			case "queryPage":
				return "runtime.DBQueryPage", nil
			case "findOne":
				return "runtime.DBFindOne", nil
			case "count":
				return "runtime.DBCount", nil
			case "upsert":
				return "runtime.DBUpsert", nil
			case "beforeQuery":
				return "runtime.AddBeforeQueryHook", nil
			}
		}
		if objStr == "cache" {
			switch e.Field {
			case "set":
				return "runtime.CacheSet", nil
			case "get":
				return "runtime.CacheGet", nil
			}
		}
		if objStr == "mcp" {
			if e.Field == "call" {
				return "runtime.InvokeMCPToolForTesting", nil
			}
		}

		// Atomic operations
		if objStr == "atomic" {
			switch e.Field {
			case "new":
				return "runtime.AtomicNew", nil
			case "inc":
				return "runtime.AtomicInc", nil
			case "dec":
				return "runtime.AtomicDec", nil
			case "get":
				return "runtime.AtomicGet", nil
			case "set":
				return "runtime.AtomicSet", nil
			case "cas":
				return "runtime.AtomicCAS", nil
			}
		}

		// Registry — generic named function map
		if objStr == "registry" {
			switch e.Field {
			case "set":
				return "runtime.RegistrySet", nil
			case "call":
				return "runtime.RegistryCall", nil
			case "list":
				return "runtime.RegistryList", nil
			case "has":
				return "runtime.RegistryHas", nil
			}
		}

		// Cron utilities
		if objStr == "schedule" {
			switch e.Field {
			case "next":
				return "runtime.CronNext", nil
			case "sleepUntilNext":
				return "runtime.CronSleepUntilNext", nil
			}
		}

		// Channel operations
		if objStr == "channel" {
			switch e.Field {
			case "new":
				return "runtime.ChannelNew", nil
			case "send":
				return "runtime.ChannelSend", nil
			case "receive":
				return "runtime.ChannelReceive", nil
			case "tryReceive":
				return "runtime.ChannelTryReceive", nil
			case "trySend":
				return "runtime.ChannelTrySend", nil
			case "close":
				return "runtime.ChannelClose", nil
			case "len":
				return "runtime.ChannelLen", nil
			}
		}

		// WebSocket connection methods
		if varType, ok := c.varTypes[objStr]; ok && varType == "*runtime.WSConn" {
			switch e.Field {
			case "send":
				return fmt.Sprintf("%s.Send", objStr), nil
			case "receive":
				return fmt.Sprintf("%s.Receive", objStr), nil
			case "close":
				return fmt.Sprintf("%s.Close", objStr), nil
			}
		}

		// Collection property access (no parentheses needed)
		if e.Field == "length" {
			return fmt.Sprintf("runtime.Length(%s)", objStr), nil
		}

		// Direct Go member access e.g. req.body
		// We'll support .Body and .Status fields (or map them casing)
		field := e.Field
		switch field {
		case "body":
			field = "Body"
		case "status":
			field = "Status"
		}
		// Since objects might be interface{}, use runtime helper for dynamic field access
		return fmt.Sprintf("runtime.MemberAccess(%s, %q)", objStr, e.Field), nil

	case *MemberAssignExpr:
		objStr, err := c.genExpression(e.Object)
		if err != nil {
			return "", err
		}
		valStr, err := c.genExpression(e.Value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() interface{} {\n\t\t\t// Safe member assignment\n\t\t\tif sm, ok := interface{}(%s).(*runtime.SafeMap); ok {\n\t\t\t\tsm.Set(%q, %s)\n\t\t\t} else if m, ok := interface{}(%s).(map[string]interface{}); ok {\n\t\t\t\tm[%q] = %s\n\t\t\t}\n\t\t\treturn nil\n\t\t}()", objStr, e.Field, valStr, objStr, e.Field, valStr), nil

	case *CallExpr:
		var funcStr string
		var isRegexpCheck bool
		var isCollectionMethod bool
		var collectionResult string
		var isStructMethodCall bool
		var structMethodResult string

		if memExpr, ok := e.Function.(*MemberExpr); ok {
			objStr, err := c.genExpression(memExpr.Object)
			if err == nil && objStr == "regexp" && memExpr.Field == "check" {
				isRegexpCheck = true
				funcStr = "regexp.check"
			}

			// Struct method calls: if the object is a known struct type, generate direct call
			if err == nil && !isRegexpCheck {
				objType := ""
				if ident, ok := memExpr.Object.(*Identifier); ok {
					if t, exists := c.varTypes[ident.Value]; exists {
						objType = t
					}
				} else if _, ok := memExpr.Object.(*SelfExpr); ok {
					if t, exists := c.varTypes["self"]; exists {
						objType = t
					}
				}
				// Check if it's a struct type (not a primitive)
				if objType != "" && objType != "interface{}" && objType != "int" && objType != "string" && objType != "float64" && objType != "bool" && objType != "[]interface{}" {
					var args []string
					for _, arg := range e.Arguments {
						argStr, err := c.genExpression(arg)
						if err != nil {
							break
						}
						args = append(args, argStr)
					}
					structMethodResult = fmt.Sprintf("%s.%s(%s)", objStr, capitalizeFirst(memExpr.Field), strings.Join(args, ", "))
					isStructMethodCall = true
				}
			}

			// Collection methods: .filter, .map, .find, .reduce, .forEach, .length, .push, .contains
			if err == nil && !isRegexpCheck && !isStructMethodCall {
				switch memExpr.Field {
				// ContextLogger methods: .info, .warn, .error, .debug, .with on user-declared variables
				case "info":
					if !isStructMethodCall {
						if ident, ok := memExpr.Object.(*Identifier); ok {
							if c.declaredVars[ident.Value] {
								var args []string
								for _, arg := range e.Arguments {
									argStr, _ := c.genExpression(arg)
									args = append(args, argStr)
								}
								collectionResult = fmt.Sprintf("runtime.ContextLoggerInfo(%s, %s)", objStr, strings.Join(args, ", "))
								isCollectionMethod = true
							}
						}
					}
				case "warn":
					if !isStructMethodCall {
						if ident, ok := memExpr.Object.(*Identifier); ok {
							if c.declaredVars[ident.Value] {
								var args []string
								for _, arg := range e.Arguments {
									argStr, _ := c.genExpression(arg)
									args = append(args, argStr)
								}
								collectionResult = fmt.Sprintf("runtime.ContextLoggerWarn(%s, %s)", objStr, strings.Join(args, ", "))
								isCollectionMethod = true
							}
						}
					}
				case "error":
					if !isStructMethodCall {
						if ident, ok := memExpr.Object.(*Identifier); ok {
							if c.declaredVars[ident.Value] {
								var args []string
								for _, arg := range e.Arguments {
									argStr, _ := c.genExpression(arg)
									args = append(args, argStr)
								}
								collectionResult = fmt.Sprintf("runtime.ContextLoggerError(%s, %s)", objStr, strings.Join(args, ", "))
								isCollectionMethod = true
							}
						}
					}
				case "debug":
					if !isStructMethodCall {
						if ident, ok := memExpr.Object.(*Identifier); ok {
							if c.declaredVars[ident.Value] {
								var args []string
								for _, arg := range e.Arguments {
									argStr, _ := c.genExpression(arg)
									args = append(args, argStr)
								}
								collectionResult = fmt.Sprintf("runtime.ContextLoggerDebug(%s, %s)", objStr, strings.Join(args, ", "))
								isCollectionMethod = true
							}
						}
					}
				case "filter":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genCollectionCallback(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.Filter(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "map":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genCollectionCallback(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.Map(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "find":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genCollectionCallback(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.Find(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "reduce":
					if len(e.Arguments) == 2 {
						cbStr, _ := c.genReduceCallback(e.Arguments[0])
						initStr, _ := c.genExpression(e.Arguments[1])
						collectionResult = fmt.Sprintf("runtime.Reduce(%s, %s, %s)", objStr, cbStr, initStr)
						isCollectionMethod = true
					}
				case "forEach":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genCollectionCallback(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.ForEach(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "length":
					collectionResult = fmt.Sprintf("runtime.Length(%s)", objStr)
					isCollectionMethod = true
				case "push":
					if len(e.Arguments) == 1 {
						elemStr, _ := c.genExpression(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.Push(%s, %s)", objStr, elemStr)
						isCollectionMethod = true
					}
				case "contains":
					if len(e.Arguments) == 1 {
						elemStr, _ := c.genExpression(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.Contains(%s, %s)", objStr, elemStr)
						isCollectionMethod = true
					}
				// String methods
				case "split":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genExpression(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.StringSplit(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "trim":
					collectionResult = fmt.Sprintf("runtime.StringTrim(%s)", objStr)
					isCollectionMethod = true
				case "replace":
					if len(e.Arguments) == 2 {
						oldStr, _ := c.genExpression(e.Arguments[0])
						newStr, _ := c.genExpression(e.Arguments[1])
						collectionResult = fmt.Sprintf("runtime.StringReplace(%s, %s, %s)", objStr, oldStr, newStr)
						isCollectionMethod = true
					}
				case "startsWith":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genExpression(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.StringStartsWith(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "endsWith":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genExpression(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.StringEndsWith(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "includes":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genExpression(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.StringIncludes(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "toUpper":
					collectionResult = fmt.Sprintf("runtime.StringToUpper(%s)", objStr)
					isCollectionMethod = true
				case "toLower":
					collectionResult = fmt.Sprintf("runtime.StringToLower(%s)", objStr)
					isCollectionMethod = true
				case "substring":
					if len(e.Arguments) >= 1 {
						startStr, _ := c.genExpression(e.Arguments[0])
						if len(e.Arguments) >= 2 {
							endStr, _ := c.genExpression(e.Arguments[1])
							collectionResult = fmt.Sprintf("runtime.StringSubstring(%s, %s, %s)", objStr, startStr, endStr)
						} else {
							collectionResult = fmt.Sprintf("runtime.StringSubstring(%s, %s)", objStr, startStr)
						}
						isCollectionMethod = true
					}
				case "indexOf":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genExpression(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.StringIndexOf(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				case "repeat":
					if len(e.Arguments) == 1 {
						argStr, _ := c.genExpression(e.Arguments[0])
						collectionResult = fmt.Sprintf("runtime.StringRepeat(%s, %s)", objStr, argStr)
						isCollectionMethod = true
					}
				}
			}
		}

		if isCollectionMethod {
			return collectionResult, nil
		}

		if isStructMethodCall {
			return structMethodResult, nil
		}

		if !isRegexpCheck {
			var err error
			funcStr, err = c.genExpression(e.Function)
			if err != nil {
				return "", err
			}
		}

		var args []string
		for _, arg := range e.Arguments {
			argStr, err := c.genExpression(arg)
			if err != nil {
				return "", err
			}
			args = append(args, argStr)
		}

		// Builtin conversions for Env and Config
		if funcStr == "env" {
			return fmt.Sprintf("runtime.Env(%s)", strings.Join(args, ", ")), nil
		}
		if funcStr == "config" {
			return fmt.Sprintf("runtime.Config(%s)", strings.Join(args, ", ")), nil
		}
		if funcStr == "validate" {
			return fmt.Sprintf("runtime.ValidateBody(%s)", strings.Join(args, ", ")), nil
		}

		// Special case: time.now()
		if funcStr == "time.now" {
			return "time.Now().Format(time.RFC3339)", nil
		}

		if funcStr == "regexp.check" && len(e.Arguments) == 2 {
			c.imports[`"regexp"`] = true
			textVal, err := c.genExpression(e.Arguments[1])
			if err != nil {
				return "", err
			}
			c.imports[`"fmt"`] = true
			if strLit, ok := e.Arguments[0].(*StringLiteral); ok {
				varName := fmt.Sprintf("regex_%d_%d", e.Token.Line, e.Token.Col)
				decl := fmt.Sprintf("var %s = regexp.MustCompile(%q)", varName, strLit.Value)
				c.regexDecls = append(c.regexDecls, decl)
				return fmt.Sprintf("%s.MatchString(fmt.Sprint(%s))", varName, textVal), nil
			} else {
				patternVal, err := c.genExpression(e.Arguments[0])
				if err != nil {
					return "", err
				}
				return fmt.Sprintf("(func() bool { r, err := regexp.Compile(fmt.Sprint(%s)); if err != nil { return false }; return r.MatchString(fmt.Sprint(%s)) }())", patternVal, textVal), nil
			}
		}

		// Emit type arguments if present
		typeArgStr := ""
		if len(e.TypeArgs) > 0 {
			typeArgStr = "[" + strings.Join(e.TypeArgs, ", ") + "]"
		}

		// Detect Go package function calls that return (value, error)
		// Wrap them to discard error and return first value
		callStr := fmt.Sprintf("%s%s(%s)", funcStr, typeArgStr, strings.Join(args, ", "))
		if memExpr, ok := e.Function.(*MemberExpr); ok {
			if ident, ok := memExpr.Object.(*Identifier); ok {
				if goPkg, isGoAlias := c.goPackageAliases[ident.Value]; isGoAlias {
					qualifiedName := goPkg + "." + memExpr.Field
					if c.goMultiReturnFuncs[qualifiedName] {
						// Multi-return Go function: wrap to discard error
						return fmt.Sprintf("func() interface{} { v, _ := %s; return v }()", callStr), nil
					}
				}
			}
		}
		return callStr, nil

	case *FStringLiteral:
		return c.genFString(e.Value)

	case *MapLiteral:
		// If there are spread entries, generate a runtime.MergeMaps call
		if len(e.Spreads) > 0 {
			var mergeArgs []string
			// Build a combined sequence: spreads and inline map fragments
			// Strategy: generate inline map for explicit keys, spread exprs merged in order
			if len(e.KeyOrder) > 0 {
				var pairs []string
				for _, k := range e.KeyOrder {
					v := e.Pairs[k]
					vStr, err := c.genExpression(v)
					if err != nil {
						return "", err
					}
					pairs = append(pairs, fmt.Sprintf("%q: %s", k, vStr))
				}
				mergeArgs = append(mergeArgs, fmt.Sprintf("map[string]interface{}{\n\t\t%s,\n\t}", strings.Join(pairs, ",\n\t\t")))
			}
			for _, spread := range e.Spreads {
				spreadStr, err := c.genExpression(spread.Value)
				if err != nil {
					return "", err
				}
				mergeArgs = append(mergeArgs, spreadStr)
			}
			return fmt.Sprintf("runtime.MergeMaps(%s)", strings.Join(mergeArgs, ", ")), nil
		}

		var pairs []string
		for _, k := range e.KeyOrder {
			v := e.Pairs[k]
			vStr, err := c.genExpression(v)
			if err != nil {
				return "", err
			}
			pairs = append(pairs, fmt.Sprintf("%q: %s", k, vStr))
		}
		if !e.ConcurrentMap {
			return fmt.Sprintf("map[string]interface{}{\n\t\t%s,\n\t}", strings.Join(pairs, ",\n\t\t")), nil
		}
		return fmt.Sprintf("runtime.NewSafeMapFromMap(map[string]interface{}{\n\t\t%s,\n\t})", strings.Join(pairs, ",\n\t\t")), nil

	case *IndexExpr:
		leftStr, err := c.genExpression(e.Left)
		if err != nil {
			return "", err
		}
		indexStr, err := c.genExpression(e.Index)
		if err != nil {
			return "", err
		}

		// If the left side has a known typed slice type, use direct indexing
		if ident, ok := e.Left.(*Identifier); ok {
			if varType, exists := c.varTypes[ident.Value]; exists {
				if strings.HasPrefix(varType, "[]") && varType != "[]interface{}" {
					// Typed slice — direct index access
					return fmt.Sprintf("%s[%s]", leftStr, indexStr), nil
				}
			}
		}

		c.imports[`"serv/runtime"`] = true
		return fmt.Sprintf("runtime.IndexAccess(%s, %s)", leftStr, indexStr), nil

	case *InfixExpr:
		leftStr, err := c.genExpression(e.Left)
		if err != nil {
			return "", err
		}
		rightStr, err := c.genExpression(e.Right)
		if err != nil {
			return "", err
		}

		lt := c.getExpressionType(e.Left)
		rt := c.getExpressionType(e.Right)

		if lt == rt && (lt == "int" || lt == "float64" || lt == "string" || lt == "bool") {
			return fmt.Sprintf("(%s %s %s)", leftStr, e.Operator, rightStr), nil
		}

		// If one side is typed and the other is interface{}, but we know the native op is valid
		if e.Operator == "%" && (lt == "int" || rt == "int") && (lt == "int" || lt == "interface{}") && (rt == "int" || rt == "interface{}") {
			if lt == "int" && rt == "int" {
				return fmt.Sprintf("(%s %s %s)", leftStr, e.Operator, rightStr), nil
			}
		}

		// Generic type params: if both sides have the same type param (e.g., T),
		// the Go compiler can handle direct operations since constraints enforce it
		if lt == rt && lt != "interface{}" && lt != "" && len(lt) <= 2 && lt[0] >= 'A' && lt[0] <= 'Z' {
			return fmt.Sprintf("(%s %s %s)", leftStr, e.Operator, rightStr), nil
		}

		// Bitwise and shift operators — use runtime helpers for untyped values
		switch e.Operator {
		case "&", "|", "^", "<<", ">>":
			return fmt.Sprintf("runtime.Bitwise(%s, %s, %q)", leftStr, rightStr, e.Operator), nil
		}

		// Since operands are interface{} in Serv, we add a utility to add/concatenate/compare if needed.
		// Comparison operators return bool; arithmetic operators return numeric/string.
		switch e.Operator {
		case "==":
			// Special case: comparison with nil
			if _, isNil := e.Right.(*NilLiteral); isNil {
				return fmt.Sprintf("(%s == nil)", leftStr), nil
			}
			if _, isNil := e.Left.(*NilLiteral); isNil {
				return fmt.Sprintf("(%s == nil)", rightStr), nil
			}
			return fmt.Sprintf("runtime.Equal(%s, %s)", leftStr, rightStr), nil
		case "!=":
			// Special case: comparison with nil
			if _, isNil := e.Right.(*NilLiteral); isNil {
				return fmt.Sprintf("(%s != nil)", leftStr), nil
			}
			if _, isNil := e.Left.(*NilLiteral); isNil {
				return fmt.Sprintf("(%s != nil)", rightStr), nil
			}
			return fmt.Sprintf("!runtime.Equal(%s, %s)", leftStr, rightStr), nil
		case "<", ">", "<=", ">=":
			return fmt.Sprintf("runtime.Compare(%s, %s, %q)", leftStr, rightStr, e.Operator), nil
		default:
			// Arithmetic operators (including %) — use runtime helper instead of inline closure
			return fmt.Sprintf("runtime.Arith(%s, %s, %q)", leftStr, rightStr, e.Operator), nil
		}



	case *PrefixExpr:
		rightStr, err := c.genExpression(e.Right)
		if err != nil {
			return "", err
		}
		rt := c.getExpressionType(e.Right)
		switch e.Operator {
		case "-":
			if rt == "int" || rt == "float64" {
				return fmt.Sprintf("(-%s)", rightStr), nil
			}
			// For interface{}, negate at runtime
			return fmt.Sprintf("runtime.Negate(%s)", rightStr), nil
		case "!":
			if rt == "bool" {
				return fmt.Sprintf("(!%s)", rightStr), nil
			}
			return fmt.Sprintf("(!isTruthy(%s))", rightStr), nil
		}
		return fmt.Sprintf("(%s%s)", e.Operator, rightStr), nil

	case *AssignExpr:
		valStr, err := c.genExpression(e.Value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("%s = %s", e.Name, valStr), nil

	case *CompoundAssignExpr:
		valStr, err := c.genExpression(e.Value)
		if err != nil {
			return "", err
		}
		// If variable has a known numeric type, emit direct Go compound assignment
		if varType, ok := c.varTypes[e.Name]; ok && (varType == "int" || varType == "float64") {
			return fmt.Sprintf("%s %s %s", e.Name, e.Operator, valStr), nil
		}
		// For interface{} variables, compute and reassign
		op := string(e.Operator[0]) // extract arithmetic op from +=, -=, etc.
		return fmt.Sprintf("%s = runtime.Arith(%s, %s, %q)", e.Name, e.Name, valStr, op), nil

	case *SliceExpr:
		leftStr, err := c.genExpression(e.Left)
		if err != nil {
			return "", err
		}
		startStr := ""
		if e.Start != nil {
			s, err := c.genExpression(e.Start)
			if err != nil {
				return "", err
			}
			startStr = s
		}
		endStr := ""
		if e.End != nil {
			s, err := c.genExpression(e.End)
			if err != nil {
				return "", err
			}
			endStr = s
		}
		// If the left side has a known typed slice, use direct Go slicing
		if ident, ok := e.Left.(*Identifier); ok {
			if varType, exists := c.varTypes[ident.Value]; exists {
				if strings.HasPrefix(varType, "[]") {
					if e.Start == nil && e.End != nil {
						return fmt.Sprintf("%s[:%s]", leftStr, endStr), nil
					} else if e.Start != nil && e.End == nil {
						return fmt.Sprintf("%s[%s:]", leftStr, startStr), nil
					} else if e.Start != nil && e.End != nil {
						return fmt.Sprintf("%s[%s:%s]", leftStr, startStr, endStr), nil
					}
					return fmt.Sprintf("%s[:]", leftStr), nil
				}
			}
		}
		// Dynamic slicing for interface{} values
		return fmt.Sprintf("runtime.Slice(%s, %s, %s)", leftStr,
			func() string { if startStr == "" { return "nil" }; return startStr }(),
			func() string { if endStr == "" { return "nil" }; return endStr }()), nil

	case *BooleanLiteral:
		if e.Value {
			return "true", nil
		}
		return "false", nil

	case *NilLiteral:
		return "nil", nil

	case *AwaitExpr:
		// Check if it's await all([...]) pattern
		if callExpr, ok := e.Value.(*CallExpr); ok {
			if ident, ok := callExpr.Function.(*Identifier); ok && ident.Value == "all" {
				// await all([expr1, expr2, ...]) — parallel execution
				if len(callExpr.Arguments) == 1 {
					if arr, ok := callExpr.Arguments[0].(*ArrayLiteral); ok {
						var taskExprs []string
						for _, elem := range arr.Elements {
							elemStr, err := c.genExpression(elem)
							if err != nil {
								return "", err
							}
							taskExprs = append(taskExprs, fmt.Sprintf("func() interface{} { return %s }", elemStr))
						}
						return fmt.Sprintf("runtime.AwaitAll([]func() interface{}{%s})", strings.Join(taskExprs, ", ")), nil
					}
				}
			}
		}
		// Single await: await expr — run in goroutine, wait for result
		valStr, err := c.genExpression(e.Value)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("runtime.Await(func() interface{} { return %s })", valStr), nil

	case *ErrorPropExpr:
		// expr? — call expression, if it returns an error (via tuple), return the error early
		valStr, err := c.genExpression(e.Value)
		if err != nil {
			return "", err
		}
		// Generate: func() interface{} { _v, _e := tryCall(fn); if _e != nil { return _e }; return _v }()
		// But since we need to return from the enclosing function, we use a pattern that
		// assigns to local vars and checks — the let statement handles the actual early return
		return fmt.Sprintf("runtime.TryCall(func() interface{} { return %s })", valStr), nil

	case *SelfExpr:
		return "self", nil

	case *FnLiteral:
		if e.IsArrow {
			// Arrow function: x => expr -> func(x interface{}) interface{} { return expr }
			var params []string
			for _, p := range e.Params {
				params = append(params, p+" interface{}")
			}
			bodyStr, err := c.genExpression(e.ArrowExpr)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("func(%s) interface{} { return %s }", strings.Join(params, ", "), bodyStr), nil
		}
		// Full function literal: fn(x, y) { body }
		var params []string
		for i, p := range e.Params {
			pt := "interface{}"
			if i < len(e.ParamTypes) && e.ParamTypes[i] != "" {
				pt = toGoType(e.ParamTypes[i])
			}
			params = append(params, p+" "+pt)
		}
		bodyStr, err := c.genBlockStatement(e.Body)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func(%s) interface{} %s", strings.Join(params, ", "), bodyStr), nil

	case *StructLiteral:
		var fields []string
		for _, k := range e.KeyOrder {
			v := e.Fields[k]
			vStr, err := c.genExpression(v)
			if err != nil {
				return "", err
			}
			fields = append(fields, fmt.Sprintf("%s: %s", capitalizeFirst(k), vStr))
		}
		return fmt.Sprintf("&%s{\n\t\t%s,\n\t}", e.TypeName, strings.Join(fields, ",\n\t\t")), nil

	case *AssertExpr:
		// Generate structured assertion messages based on the condition type
		if infix, ok := e.Cond.(*InfixExpr); ok {
			leftStr, err := c.genExpression(infix.Left)
			if err != nil {
				return "", err
			}
			rightStr, err := c.genExpression(infix.Right)
			if err != nil {
				return "", err
			}
			switch infix.Operator {
			case "==":
				return fmt.Sprintf("func() {\n\t\tgot := interface{}(%s)\n\t\twant := interface{}(%s)\n\t\tif !runtime.Equal(got, want) {\n\t\t\tt.Fatalf(\"assertion failed: got %%v, want %%v\", got, want)\n\t\t}\n\t}()", leftStr, rightStr), nil
			case "!=":
				return fmt.Sprintf("func() {\n\t\tgot := interface{}(%s)\n\t\tunwanted := interface{}(%s)\n\t\tif runtime.Equal(got, unwanted) {\n\t\t\tt.Fatalf(\"assertion failed: expected value to not equal %%v\", unwanted)\n\t\t}\n\t}()", leftStr, rightStr), nil
			case "<":
				return fmt.Sprintf("func() {\n\t\tleft := interface{}(%s)\n\t\tright := interface{}(%s)\n\t\tif !runtime.Compare(left, right, \"<\") {\n\t\t\tt.Fatalf(\"assertion failed: %%v is not < %%v\", left, right)\n\t\t}\n\t}()", leftStr, rightStr), nil
			case ">":
				return fmt.Sprintf("func() {\n\t\tleft := interface{}(%s)\n\t\tright := interface{}(%s)\n\t\tif !runtime.Compare(left, right, \">\") {\n\t\t\tt.Fatalf(\"assertion failed: %%v is not > %%v\", left, right)\n\t\t}\n\t}()", leftStr, rightStr), nil
			case "<=":
				return fmt.Sprintf("func() {\n\t\tleft := interface{}(%s)\n\t\tright := interface{}(%s)\n\t\tif !runtime.Compare(left, right, \"<=\") {\n\t\t\tt.Fatalf(\"assertion failed: %%v is not <= %%v\", left, right)\n\t\t}\n\t}()", leftStr, rightStr), nil
			case ">=":
				return fmt.Sprintf("func() {\n\t\tleft := interface{}(%s)\n\t\tright := interface{}(%s)\n\t\tif !runtime.Compare(left, right, \">=\") {\n\t\t\tt.Fatalf(\"assertion failed: %%v is not >= %%v\", left, right)\n\t\t}\n\t}()", leftStr, rightStr), nil
			}
		}
		// Fallback for non-comparison assertions (truthiness check)
		condStr, err := c.genExpression(e.Cond)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func() {\n\t\tvar v interface{} = %s\n\t\tif v == nil || v == false {\n\t\t\tt.Fatalf(\"assertion failed: expected truthy value, got %%v\", v)\n\t\t}\n\t}()", condStr), nil

	default:
		return "", fmt.Errorf("unknown expression type: %T", expr)
	}
}










// genCollectionCallback generates a Go function literal for collection method callbacks.
// Handles identifiers (function references), fn literals, and arrow functions.
func (c *Codegen) genCollectionCallback(expr Expression) (string, error) {
	// If it's a FnLiteral (fn(x) { ... } or x => expr), generate directly
	if fnLit, ok := expr.(*FnLiteral); ok {
		if fnLit.IsArrow {
			bodyStr, err := c.genExpression(fnLit.ArrowExpr)
			if err != nil {
				return "", err
			}
			param := "item"
			if len(fnLit.Params) > 0 {
				param = fnLit.Params[0]
			}
			return fmt.Sprintf("func(%s interface{}) interface{} { return %s }", param, bodyStr), nil
		}
		// Full fn literal
		param := "item"
		if len(fnLit.Params) > 0 {
			param = fnLit.Params[0]
		}
		bodyStr, err := c.genBlockStatement(fnLit.Body)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("func(%s interface{}) interface{} %s", param, bodyStr), nil
	}
	// If it's a simple identifier (function name), wrap it
	if ident, ok := expr.(*Identifier); ok {
		return fmt.Sprintf("func(item interface{}) interface{} { return %s(item) }", ident.Value), nil
	}
	// Fallback: generate the expression and wrap it as a callable
	exprStr, err := c.genExpression(expr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("func(item interface{}) interface{} { return %s(item) }", exprStr), nil
}

// genReduceCallback generates a Go function literal for reduce callbacks (2 params).
func (c *Codegen) genReduceCallback(expr Expression) (string, error) {
	if ident, ok := expr.(*Identifier); ok {
		return fmt.Sprintf("func(acc interface{}, item interface{}) interface{} { return %s(acc, item) }", ident.Value), nil
	}
	exprStr, err := c.genExpression(expr)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("func(acc interface{}, item interface{}) interface{} { return %s(acc, item) }", exprStr), nil
}

func (c *Codegen) genFString(str string) (string, error) {
	var formatParts []string
	var args []string

	runes := []rune(str)
	i := 0
	n := len(runes)
	currentText := ""

	for i < n {
		if runes[i] == '{' {
			if currentText != "" {
				formatParts = append(formatParts, currentText)
				currentText = ""
			}
			i++
			exprText := ""
			for i < n && runes[i] != '}' {
				exprText += string(runes[i])
				i++
			}
			if i >= n {
				return "", fmt.Errorf("unterminated brace in f-string")
			}
			i++ // skip '}'
			formatParts = append(formatParts, "%v")

			// Parse and codegen the interpolated expression
			exprCode, err := c.compileInlineExpr(exprText)
			if err != nil {
				// Fallback to raw text if parsing fails
				args = append(args, exprText)
			} else {
				args = append(args, exprCode)
			}
		} else {
			currentText += string(runes[i])
			i++
		}
	}
	if currentText != "" {
		formatParts = append(formatParts, currentText)
	}

	formatStr := strings.Join(formatParts, "")
	if len(args) == 0 {
		return fmt.Sprintf("%q", formatStr), nil
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("fmt.Sprintf(%q, %s)", formatStr, strings.Join(args, ", ")), nil
}

// compileInlineExpr parses and generates Go code for a single expression string.
// Used by f-string interpolation to properly handle self.field -> self.Field etc.
func (c *Codegen) compileInlineExpr(exprText string) (string, error) {
	lexer := NewLexer(exprText)
	parser := NewParser(lexer)
	// Parse as a single expression
	expr := parser.parseExpression(LOWEST)
	if expr == nil || len(parser.Errors()) > 0 {
		return "", fmt.Errorf("failed to parse inline expression")
	}
	return c.genExpression(expr)
}





