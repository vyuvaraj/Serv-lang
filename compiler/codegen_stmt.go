package compiler

import (
	"bytes"
	"fmt"
	"path/filepath"
	"regexp"
	"strings"
)

func (c *Codegen) genImportStmt(_ *ImportStmt) (string, error) {
	return "", nil
}

func (c *Codegen) genGoPackageImport(s *GoPackageImport) (string, error) {
	c.imports[`"`+s.Path+`"`] = true
	pkgName := filepath.Base(s.Path)
	c.goPackageAliases[s.Alias] = pkgName
	return "", nil
}

func (c *Codegen) genDeclareModuleStmt(s *DeclareModuleStmt) (string, error) {
	pkgName := filepath.Base(s.PkgPath)
	for _, fn := range s.Functions {
		key := s.PkgPath + ":" + fn.Name
		c.declaredGoFuncs[key] = pkgName + "." + fn.Name
		if fn.MultiReturn {
			c.goMultiReturnFuncs[pkgName+"."+fn.Name] = true
		}
	}
	return "", nil
}

func (c *Codegen) genExportStmt(s *ExportStmt) (string, error) {
	return c.genStatement(s.Inner)
}

func (c *Codegen) genExternFnStmt(s *ExternFnStmt) (string, error) {
	var out bytes.Buffer
	out.WriteString(fmt.Sprintf("func %s(", s.Name))
	var params []string
	for _, p := range s.Params {
		params = append(params, p+" interface{}")
	}
	out.WriteString(strings.Join(params, ", "))
	out.WriteString(") interface{} {\n")

	if strings.HasPrefix(s.Source, "go:") {
		goCall, ok := c.goExterns[s.Name]
		if !ok {
			return "", fmt.Errorf("missing go call mapping for extern fn: %s", s.Name)
		}
		var callArgs []string
		for _, p := range s.Params {
			callArgs = append(callArgs, p)
		}
		out.WriteString(fmt.Sprintf("\treturn %s(%s)\n", goCall, strings.Join(callArgs, ", ")))
	} else if strings.HasPrefix(s.Source, "python:") {
		parts := strings.Split(strings.TrimPrefix(s.Source, "python:"), ":")
		if len(parts) >= 2 {
			scriptPath := parts[0]
			funcName := parts[1]
			if absPath, err := filepath.Abs(scriptPath); err == nil {
				scriptPath = filepath.ToSlash(absPath)
			}
			var callArgs []string
			for _, p := range s.Params {
				callArgs = append(callArgs, p)
			}
			argsStr := ""
			if len(callArgs) > 0 {
				argsStr = ", " + strings.Join(callArgs, ", ")
			}
			out.WriteString(fmt.Sprintf("\treturn runtime.CallPython(%q, %q%s)\n", scriptPath, funcName, argsStr))
		} else {
			return "", fmt.Errorf("invalid python extern source: %s", s.Source)
		}
	}

	out.WriteString("}\n\n")
	return out.String(), nil
}

func (c *Codegen) genEnumStmt(s *EnumStmt) (string, error) {
	var rOut bytes.Buffer
	rOut.WriteString("const (\n")
	for i, m := range s.Members {
		if valExpr, hasValue := s.Values[m]; hasValue {
			valStr, err := c.genExpression(valExpr)
			if err != nil {
				return "", err
			}
			switch valExpr.(type) {
			case *IntegerLiteral:
				c.varTypes[m] = "int"
				rOut.WriteString(fmt.Sprintf("\t%s = %s\n", m, valStr))
			case *FloatLiteral:
				c.varTypes[m] = "float64"
				rOut.WriteString(fmt.Sprintf("\t%s = %s\n", m, valStr))
			case *StringLiteral:
				c.varTypes[m] = "string"
				rOut.WriteString(fmt.Sprintf("\t%s = %s\n", m, valStr))
			default:
				c.varTypes[m] = "interface{}"
				rOut.WriteString(fmt.Sprintf("\t%s = %s\n", m, valStr))
			}
		} else {
			if i == 0 && len(s.Values) == 0 {
				c.varTypes[m] = "string"
				rOut.WriteString(fmt.Sprintf("\t%s = %q\n", m, m))
			} else {
				c.varTypes[m] = "string"
				rOut.WriteString(fmt.Sprintf("\t%s = %q\n", m, m))
			}
		}
	}
	rOut.WriteString(")\n\n")
	return rOut.String(), nil
}

func (c *Codegen) genTypeAliasStmt(s *TypeAliasStmt) (string, error) {
	goType := toGoType(s.BaseType)
	if goType == "interface{}" && s.BaseType != "any" {
		goType = s.BaseType
	}
	return fmt.Sprintf("type %s = %s\n\n", s.Name, goType), nil
}

func (c *Codegen) genValidateStmt(s *ValidateStmt) (string, error) {
	var keys []string
	for _, k := range s.Required {
		keys = append(keys, fmt.Sprintf("%q", k))
	}
	return fmt.Sprintf("func init() {\n\truntime.ValidateConfig([]string{%s})\n}\n\n", strings.Join(keys, ", ")), nil
}

func (c *Codegen) genIfStmt(s *IfStmt) (string, error) {
	condStr, err := c.genExpression(s.Condition)
	if err != nil {
		return "", err
	}
	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	condType := c.getExpressionType(s.Condition)
	var condCode string
	if condType == "bool" {
		condCode = condStr
	} else {
		condCode = fmt.Sprintf("isTruthy(%s)", condStr)
	}
	if s.ElseBody != nil {
		elseStr, err := c.genBlockStatement(s.ElseBody)
		if err != nil {
			return "", err
		}
		return fmt.Sprintf("if %s %s else %s\n", condCode, bodyStr, elseStr), nil
	}
	return fmt.Sprintf("if %s %s\n", condCode, bodyStr), nil
}

func (c *Codegen) genForStmt(s *ForStmt) (string, error) {
	if s.IsRange {
		iterStr, err := c.genExpression(s.Iterable)
		if err != nil {
			return "", err
		}
		c.declareVar(s.Variable)
		if s.KeyVar != "" {
			c.declareVar(s.KeyVar)
			c.varTypes[s.KeyVar] = "interface{}"
		}

		iterType := c.getExpressionType(s.Iterable)

		// Infer loop variable type from the iterable's element type
		// e.g., for item in []int{} → item is int
		if strings.HasPrefix(iterType, "[]") && iterType != "[]interface{}" {
			elemType := strings.TrimPrefix(iterType, "[]")
			c.varTypes[s.Variable] = elemType
		} else {
			c.varTypes[s.Variable] = "interface{}"
		}

		bodyStr, err := c.genBlockStatement(s.Body)
		if err != nil {
			return "", err
		}

		// Map iteration: for key, value in map
		if s.KeyVar != "" {
			c.varTypes[s.KeyVar] = "string"
			// Inject blank-identifier guards to prevent "declared and not used" errors
			guardedBody := fmt.Sprintf("{\n\t_ = %s\n\t_ = %s\n", s.KeyVar, s.Variable) + bodyStr[2:]
			return fmt.Sprintf("for %s, %s := range runtime.ToMap(%s) %s\n", s.KeyVar, s.Variable, iterStr, guardedBody), nil
		}

		if iterType == "[]interface{}" || strings.HasPrefix(iterType, "[]") {
			return fmt.Sprintf("for _, %s := range %s %s\n", s.Variable, iterStr, bodyStr), nil
		}
		return fmt.Sprintf("for _, %s := range toSlice(%s) %s\n", s.Variable, iterStr, bodyStr), nil
	}
	condStr, err := c.genExpression(s.Iterable)
	if err != nil {
		return "", err
	}
	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	condType := c.getExpressionType(s.Iterable)
	var condCode string
	if condType == "bool" {
		condCode = condStr
	} else {
		condCode = fmt.Sprintf("isTruthy(%s)", condStr)
	}
	return fmt.Sprintf("for %s %s\n", condCode, bodyStr), nil
}

func (c *Codegen) genStructDecl(s *StructDecl) (string, error) {
	var out bytes.Buffer

	typeParamSet := make(map[string]bool)
	for _, tp := range s.TypeParams {
		typeParamSet[tp] = true
	}

	typeParamStr := ""
	if len(s.TypeParams) > 0 {
		var tps []string
		for i, tp := range s.TypeParams {
			constraint := "any"
			if i < len(s.TypeConstraints) && s.TypeConstraints[i] != "" {
				constraint = servConstraintToGo(s.TypeConstraints[i])
			}
			tps = append(tps, tp+" "+constraint)
		}
		typeParamStr = "[" + strings.Join(tps, ", ") + "]"
	}

	out.WriteString(fmt.Sprintf("type %s%s struct {\n", s.Name, typeParamStr))
	for _, f := range s.Fields {
		goType := toGoType(f.Type)
		if typeParamSet[f.Type] {
			goType = f.Type
		} else if strings.HasPrefix(f.Type, "[]") && typeParamSet[strings.TrimPrefix(f.Type, "[]")] {
			goType = "[]" + strings.TrimPrefix(f.Type, "[]")
		} else if goType == "interface{}" && !strings.HasSuffix(f.Type, "?") && !strings.Contains(f.Type, "|") {
			goType = f.Type
		}
		out.WriteString(fmt.Sprintf("\t%s %s\n", capitalizeFirst(f.Name), goType))
	}
	out.WriteString("}\n\n")
	c.varTypes[s.Name] = s.Name
	return out.String(), nil
}

func (c *Codegen) genMethodDecl(s *MethodDecl) (string, error) {
	c.inFunction = true
	oldDeclared := c.declaredVars
	oldVarTypes := c.varTypes
	c.declaredVars = NewScope(nil)
	c.varTypes = make(map[string]string)
	for k, v := range oldVarTypes {
		c.varTypes[k] = v
	}
	c.declareVar("self")
	c.varTypes["self"] = s.TypeName

	var params []string
	for i, p := range s.Params {
		c.declareVar(p)
		pt := "interface{}"
		if i < len(s.ParamTypes) && s.ParamTypes[i] != "" {
			pt = toGoType(s.ParamTypes[i])
			c.varTypes[p] = pt
		}
		params = append(params, p+" "+pt)
	}

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}

	c.declaredVars = oldDeclared
	c.varTypes = oldVarTypes
	c.inFunction = false

	retType := "interface{}"
	if s.ReturnType != "" {
		retType = toGoType(s.ReturnType)
		if retType == "interface{}" && !strings.HasSuffix(s.ReturnType, "?") && !strings.Contains(s.ReturnType, "|") {
			retType = s.ReturnType
		}
	}

	return fmt.Sprintf("func (self *%s) %s(%s) %s %s\n\n", s.TypeName, capitalizeFirst(s.Name), strings.Join(params, ", "), retType, bodyStr), nil
}

func (c *Codegen) genInterfaceDecl(s *InterfaceDecl) (string, error) {
	var out bytes.Buffer
	out.WriteString(fmt.Sprintf("type %s interface {\n", s.Name))
	for _, m := range s.Methods {
		var params []string
		for i, p := range m.Params {
			pt := "interface{}"
			if i < len(m.ParamTypes) && m.ParamTypes[i] != "" {
				pt = toGoType(m.ParamTypes[i])
			}
			params = append(params, p+" "+pt)
		}
		retType := "interface{}"
		if m.ReturnType != "" {
			retType = toGoType(m.ReturnType)
			if retType == "interface{}" {
				retType = m.ReturnType
			}
		}
		out.WriteString(fmt.Sprintf("\t%s(%s) %s\n", capitalizeFirst(m.Name), strings.Join(params, ", "), retType))
	}
	out.WriteString("}\n\n")
	return out.String(), nil
}

func (c *Codegen) genBrokerStmt(s *BrokerStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("func init() {\n\truntime.InitBroker(fmt.Sprint(%s))\n}\n\n", val), nil
}

func (c *Codegen) genAiStmt(s *AiStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("func init() {\n\truntime.InitAI(fmt.Sprint(%s))\n}\n\n", val), nil
}

func (c *Codegen) genServerStmt(s *ServerStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	if s.TLS {
		return fmt.Sprintf("func init() {\n\truntime.InitServerTLS(fmt.Sprint(%s), %q, %q)\n}\n\n", val, s.CertFile, s.KeyFile), nil
	}
	return fmt.Sprintf("func init() {\n\truntime.InitServer(fmt.Sprint(%s))\n}\n\n", val), nil
}

func (c *Codegen) genAuthStmt(s *AuthStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\truntime.InitAuth(fmt.Sprint(%s))\n}\n\n", val), nil
}

func (c *Codegen) genMailStmt(s *MailStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\truntime.InitMail(fmt.Sprint(%s))\n}\n\n", val), nil
}

func (c *Codegen) genStoreStmt(s *StoreStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\truntime.InitStore(fmt.Sprint(%s))\n}\n\n", val), nil
}

func (c *Codegen) genSearchStmt(s *SearchStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\truntime.InitSearch(fmt.Sprint(%s))\n}\n\n", val), nil
}





func (c *Codegen) genCorsStmt(s *CorsStmt) (string, error) {
	var origins []string
	for _, o := range s.Origins {
		origins = append(origins, fmt.Sprintf("%q", o))
	}
	return fmt.Sprintf("func init() {\n\truntime.EnableCORS([]string{%s})\n}\n\n", strings.Join(origins, ", ")), nil
}

func (c *Codegen) genRateLimitStmt(s *RateLimitStmt) (string, error) {
	return fmt.Sprintf("func init() {\n\truntime.SetGlobalIPRateLimit(%d, %q)\n}\n\n", s.LimitRate, s.LimitPeriod), nil
}

func (c *Codegen) genDatabaseStmt(s *DatabaseStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\truntime.InitDB(fmt.Sprint(%s))\n}\n\n", val), nil
}

func (c *Codegen) genCacheStmt(s *CacheStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\truntime.InitCache(fmt.Sprint(%s))\n}\n\n", val), nil
}

func (c *Codegen) genMatchStmt(s *MatchStmt) (string, error) {
	valStr, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	out.WriteString(fmt.Sprintf("switch %s {\n", valStr))
	for _, cs := range s.Cases {
		if cs.Value != nil {
			caseVal, err := c.genExpression(cs.Value)
			if err != nil {
				return "", err
			}
			bodyStr, err := c.genBlockStatement(cs.Body)
			if err != nil {
				return "", err
			}
			out.WriteString(fmt.Sprintf("case %s: %s\n", caseVal, bodyStr))
		} else {
			bodyStr, err := c.genBlockStatement(cs.Body)
			if err != nil {
				return "", err
			}
			out.WriteString(fmt.Sprintf("default: %s\n", bodyStr))
		}
	}
	out.WriteString("}\n")
	return out.String(), nil
}

func (c *Codegen) genRouteStmt(s *RouteStmt) (string, error) {
	oldRoute := c.currentRoute
	c.currentRoute = s
	defer func() { c.currentRoute = oldRoute }()

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}

	if s.Stream {
		bodyStr = fmt.Sprintf("{\n\t_streamChan := make(chan interface{})\n\tgo func() {\n\t\tdefer close(_streamChan)\n\t\tfunc() %s()\n\t}()\n\treturn _streamChan\n}", bodyStr)
	}


	middlewares := s.Middlewares
	hasAuthStmt := false
	for _, stmt := range c.program.Statements {
		if _, ok := stmt.(*AuthStmt); ok {
			hasAuthStmt = true
			break
		}
	}
	if hasAuthStmt {
		alreadyHasAuth := false
		for _, mw := range middlewares {
			if mw == "auth" {
				alreadyHasAuth = true
				break
			}
		}
		if !alreadyHasAuth {
			middlewares = append([]string{"auth"}, middlewares...)
		}
	}

	if len(middlewares) > 0 {
		var middlewareNames []string
		for _, mw := range middlewares {
			middlewareNames = append(middlewareNames, fmt.Sprintf("%q", mw))
		}
		return fmt.Sprintf("func init() {\n\truntime.AddRouteWithMiddleware(%q, %q, %d, %q, []string{%s}, func(%s runtime.Request) interface{} %s)\n}\n\n",
			s.Method, s.Path, s.LimitRate, s.LimitPeriod, strings.Join(middlewareNames, ", "), s.Param, bodyStr), nil
	}
	return fmt.Sprintf("func init() {\n\truntime.AddRoute(%q, %q, %d, %q, func(%s runtime.Request) interface{} %s)\n}\n\n", s.Method, s.Path, s.LimitRate, s.LimitPeriod, s.Param, bodyStr), nil
}

func (c *Codegen) genMiddlewareDecl(s *MiddlewareDecl) (string, error) {
	oldMw := c.currentMiddleware
	c.currentMiddleware = s
	defer func() { c.currentMiddleware = oldMw }()

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	if !strings.Contains(bodyStr, "return") || strings.Contains(bodyStr, "if") {
		bodyStr = bodyStr[:len(bodyStr)-1] + "\treturn nil\n}"
	}
	return fmt.Sprintf("func init() {\n\truntime.RegisterMiddleware(%q, func(%s runtime.Request) interface{} %s)\n}\n\n", s.Name, s.Param, bodyStr), nil
}

func (c *Codegen) genWsStmt(s *WsStmt) (string, error) {
	oldDeclared := c.declaredVars
	oldVarTypes := c.varTypes
	c.declaredVars = NewScope(nil)
	c.varTypes = make(map[string]string)
	for k, v := range oldVarTypes {
		c.varTypes[k] = v
	}
	c.declareVar(s.Param)
	c.varTypes[s.Param] = "*runtime.WSConn"
	c.inFunction = true

	oldWs := c.currentWs
	c.currentWs = s
	defer func() { c.currentWs = oldWs }()

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}

	c.declaredVars = oldDeclared
	c.varTypes = oldVarTypes
	c.inFunction = false

	return fmt.Sprintf("func init() {\n\truntime.AddWebSocket(%q, func(%s *runtime.WSConn) %s)\n}\n\n", s.Path, s.Param, bodyStr), nil
}

func (c *Codegen) genToolStmt(s *ToolStmt) (string, error) {
	oldTool := c.currentTool
	c.currentTool = s
	defer func() { c.currentTool = oldTool }()

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("func init() {\n\truntime.AddMCPTool(%q, %q, func(%s interface{}) interface{} %s)\n}\n\n", s.Name, s.Description, s.Param, bodyStr), nil
}

func (c *Codegen) genMigrationStmt(s *MigrationStmt) (string, error) {
	oldMigration := c.currentMigration
	c.currentMigration = s
	defer func() { c.currentMigration = oldMigration }()

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("func init() {\n\truntime.RegisterMigration(%q, func() %s)\n}\n\n", s.Name, bodyStr), nil
}

func (c *Codegen) genEveryStmt(s *EveryStmt) (string, error) {
	interval, err := c.genExpression(s.Interval)
	if err != nil {
		return "", err
	}

	oldEvery := c.currentEvery
	c.currentEvery = s
	defer func() { c.currentEvery = oldEvery }()

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\t// Scheduled interval job\n\tgo func() {\n\t\ttime.Sleep(100 * time.Millisecond) // brief sleep to let server initialize\n\t\truntime.Every(fmt.Sprint(%s), func() %s)\n\t}()\n}\n\n", interval, bodyStr), nil
}

func (c *Codegen) genCronStmt(s *CronStmt) (string, error) {
	cronVal, err := c.genExpression(s.Cron)
	if err != nil {
		return "", err
	}

	oldCron := c.currentCron
	c.currentCron = s
	defer func() { c.currentCron = oldCron }()

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\t// Scheduled cron job\n\tgo func() {\n\t\ttime.Sleep(100 * time.Millisecond)\n\t\truntime.Cron(fmt.Sprint(%s), func() %s)\n\t}()\n}\n\n", cronVal, bodyStr), nil
}

func (c *Codegen) genSubscribeStmt(s *SubscribeStmt) (string, error) {
	topic, err := c.genExpression(s.Topic)
	if err != nil {
		return "", err
	}

	oldSub := c.currentSubscribe
	c.currentSubscribe = s
	defer func() { c.currentSubscribe = oldSub }()

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("func init() {\n\truntime.Subscribe(fmt.Sprint(%s), func(%s string) %s)\n}\n\n", topic, s.Param, bodyStr), nil
}

func (c *Codegen) genPublishStmt(s *PublishStmt) (string, error) {
	topic, err := c.genExpression(s.Topic)
	if err != nil {
		return "", err
	}
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	c.imports[`"fmt"`] = true
	return fmt.Sprintf("runtime.Publish(fmt.Sprint(%s), %s)\n", topic, val), nil
}

func (c *Codegen) genSpawnStmt(s *SpawnStmt) (string, error) {
	call, err := c.genExpression(s.Call)
	if err != nil {
		return "", err
	}

	taskName := "task"
	if callExpr, ok := s.Call.(*CallExpr); ok {
		if ident, ok := callExpr.Function.(*Identifier); ok {
			taskName = ident.Value
		} else if member, ok := callExpr.Function.(*MemberExpr); ok {
			taskName = member.Field
		}
	}

	var spawnCode string
	if s.Limit != nil {
		limStr, err := c.genExpression(s.Limit)
		if err != nil {
			return "", err
		}
		semID := fmt.Sprintf("spawn_%d_%d", s.Token.Line, s.Token.Col)
		c.imports[`"fmt"`] = true
		c.imports[`"strconv"`] = true
		spawnCode = fmt.Sprintf(`_spawnTrace := runtime.GetActiveTrace()
		runtime.AcquireSemaphore(%q, func() int {
			val, _ := strconv.Atoi(fmt.Sprint(%s))
			if val <= 0 { return 1 }
			return val
		}())
go func() {
		defer runtime.ReleaseSemaphore(%q)
		if _spawnTrace != nil {
			runtime.SetActiveTrace(_spawnTrace)
			defer runtime.ClearActiveTrace()
		}
		_endSpan := runtime.TraceSpawn(%q)
		defer _endSpan()
		defer func() {
			if r := recover(); r != nil {
				runtime.LogError("Recovered in spawned task: ", r)
			}
		}()
		%s
	}()
`, semID, limStr, semID, taskName, call)
	} else {
		spawnCode = fmt.Sprintf(`_spawnTrace := runtime.GetActiveTrace()
go func() {
		if _spawnTrace != nil {
			runtime.SetActiveTrace(_spawnTrace)
			defer runtime.ClearActiveTrace()
		}
		_endSpan := runtime.TraceSpawn(%q)
		defer _endSpan()
		defer func() {
			if r := recover(); r != nil {
				runtime.LogError("Recovered in spawned task: ", r)
			}
		}()
		%s
	}()
`, taskName, call)
	}
	if !c.inFunction {
		return fmt.Sprintf("func init() {\n\t%s}\n\n", spawnCode), nil
	}
	return spawnCode, nil
}

func (c *Codegen) genTestStmt(s *TestStmt) (string, error) {
	c.imports[`"serv/runtime"`] = true
	c.inFunction = true
	oldConcurrent := c.inConcurrentContext
	c.inConcurrentContext = hasConcurrency(s.Body)

	funcName := "Test_" + sanitizeTestName(s.Name)
	// Additional cleaning to prevent invalid Go identifier characters (e.g. commas)
	funcName = regexp.MustCompile(`[^a-zA-Z0-9_]`).ReplaceAllString(funcName, "")
	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	c.inFunction = false
	c.inConcurrentContext = oldConcurrent

	// Build the test function body: beforeEach + body + afterEach
	// Strip outer braces from bodyStr to get the inner content
	inner := ""
	if len(bodyStr) > 2 {
		inner = bodyStr[2 : len(bodyStr)-1] // strip "{\n" and "}"
	}

	var finalBody strings.Builder
	finalBody.WriteString("{\n")
	finalBody.WriteString("\truntime.ResetTestState()\n")
	finalBody.WriteString("\tdefer runtime.ResetTestState()\n")

	// BeforeEach blocks
	for _, before := range c.beforeEachBlocks {
		finalBody.WriteString(before)
	}

	// Test body
	finalBody.WriteString(inner)

	// AfterEach blocks
	for _, after := range c.afterEachBlocks {
		finalBody.WriteString(after)
	}

	finalBody.WriteString("}")

	if s.Timeout != "" {
		// Wrap in timeout
		c.imports[`"time"`] = true
		var wrapped strings.Builder
		wrapped.WriteString("{\n")
		wrapped.WriteString(fmt.Sprintf("\t_timeout, _ := time.ParseDuration(%q)\n", s.Timeout))
		wrapped.WriteString("\t_done := make(chan struct{})\n")
		wrapped.WriteString("\tgo func() {\n")
		wrapped.WriteString("\t\tdefer close(_done)\n")
		// Indent the inner body content
		for _, line := range strings.Split(finalBody.String()[2:len(finalBody.String())-1], "\n") {
			if strings.TrimSpace(line) != "" {
				wrapped.WriteString("\t\t")
				wrapped.WriteString(strings.TrimPrefix(line, "\t"))
				wrapped.WriteString("\n")
			}
		}
		wrapped.WriteString("\t}()\n")
		wrapped.WriteString("\tselect {\n")
		wrapped.WriteString("\tcase <-_done:\n")
		wrapped.WriteString("\tcase <-time.After(_timeout):\n")
		wrapped.WriteString("\t\tt.Fatalf(\"test timed out after %s\", _timeout)\n")
		wrapped.WriteString("\t}\n")
		wrapped.WriteString("}")

		c.testFuncs = append(c.testFuncs, fmt.Sprintf("func %s(t *testing.T) %s\n", funcName, wrapped.String()))
	} else {
		c.testFuncs = append(c.testFuncs, fmt.Sprintf("func %s(t *testing.T) %s\n", funcName, finalBody.String()))
	}
	return "", nil
}

func (c *Codegen) genMockStmt(s *MockStmt) (string, error) {
	callExpr, ok := s.Target.(*CallExpr)
	if !ok {
		return "", fmt.Errorf("mock target must be a function call expression")
	}

	funcName, err := c.genExpression(callExpr.Function)
	if err != nil {
		return "", err
	}

	var keyParts []string
	keyParts = append(keyParts, fmt.Sprintf("%q", funcName))
	for _, arg := range callExpr.Arguments {
		argStr, err := c.genExpression(arg)
		if err != nil {
			return "", err
		}
		keyParts = append(keyParts, fmt.Sprintf("fmt.Sprint(%s)", argStr))
	}

	keyExpr := strings.Join(keyParts, " + \":\" + ")

	c.imports[`"serv/runtime"`] = true
	c.imports[`"fmt"`] = true
	bodyCode, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}

	mockCode := fmt.Sprintf("runtime.RegisterMock(%s, func(args ...interface{}) interface{} %s)\n", keyExpr, bodyCode)
	return mockCode, nil
}

func (c *Codegen) genBeforeEachStmt(s *BeforeEachStmt) (string, error) {
	c.inFunction = true
	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	c.inFunction = false
	// Store the inner content (without the outer braces) for injection into tests
	inner := ""
	if len(bodyStr) > 2 {
		inner = bodyStr[2 : len(bodyStr)-1]
	}
	c.beforeEachBlocks = append(c.beforeEachBlocks, inner)
	return "", nil
}

func (c *Codegen) genAfterEachStmt(s *AfterEachStmt) (string, error) {
	c.inFunction = true
	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}
	c.inFunction = false
	inner := ""
	if len(bodyStr) > 2 {
		inner = bodyStr[2 : len(bodyStr)-1]
	}
	c.afterEachBlocks = append(c.afterEachBlocks, inner)
	return "", nil
}

func (c *Codegen) genDestructureLetStmt(s *DestructureLetStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	var out bytes.Buffer
	tmpVar := fmt.Sprintf("_destructure_%d_%d", s.Token.Line, s.Token.Col)
	out.WriteString(fmt.Sprintf("var %s interface{} = %s\n", tmpVar, val))
	c.imports[`"fmt"`] = true
	for _, field := range s.Fields {
		c.declareVar(field)
		out.WriteString(fmt.Sprintf("var %s interface{} = func() interface{} {\n", field))
		out.WriteString(fmt.Sprintf("\tswitch v := %s.(type) {\n", tmpVar))
		out.WriteString(fmt.Sprintf("\tcase *runtime.SafeMap:\n\t\treturn v.Get(%q)\n", field))
		out.WriteString(fmt.Sprintf("\tcase map[string]interface{}:\n\t\treturn v[%q]\n", field))
		out.WriteString("\tdefault:\n")
		out.WriteString(fmt.Sprintf("\t\treturn runtime.GetField(v, %q)\n", field))
		out.WriteString("\t}\n}()\n")
		out.WriteString(fmt.Sprintf("_ = %s\n", field))
	}
	return out.String(), nil
}

func (c *Codegen) genLetStmt(s *LetStmt) (string, error) {
	// Special case: let x = expr? — error propagation
	if errProp, ok := s.Value.(*ErrorPropExpr); ok {
		innerVal, err := c.genExpression(errProp.Value)
		if err != nil {
			return "", err
		}
		c.declareVar(s.Name)
		tmpVal := fmt.Sprintf("_prop_val_%d", errProp.Token.Line)
		tmpErr := fmt.Sprintf("_prop_err_%d", errProp.Token.Line)

		var retStmt string
		if c.currentFn != nil {
			retType := c.currentFn.ReturnType
			if strings.Contains(retType, "|") {
				parts := strings.Split(retType, "|")
				hasError := false
				for _, p := range parts {
					if strings.TrimSpace(p) == "error" {
						hasError = true
						break
					}
				}
				if hasError {
					retStmt = fmt.Sprintf("return [2]interface{}{nil, %s}", tmpErr)
				} else {
					retStmt = "return nil"
				}
			} else if retType == "error" {
				retStmt = fmt.Sprintf("return [2]interface{}{nil, %s}", tmpErr)
			} else if strings.HasSuffix(retType, "?") {
				retStmt = "return nil"
			} else {
				retStmt = "return nil"
			}
		} else if c.currentRoute != nil || c.currentMiddleware != nil || c.currentTool != nil {
			retStmt = fmt.Sprintf("return map[string]interface{}{\"error\": %s}", tmpErr)
		} else {
			retStmt = "return"
		}

		return fmt.Sprintf("%s, %s := runtime.TryCallWithError(func() interface{} { return %s })\nif %s != nil {\n\t%s\n}\nvar %s interface{} = %s\n_ = %s\n",
			tmpVal, tmpErr, innerVal, tmpErr, retStmt, s.Name, tmpVal, s.Name), nil
	}

	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}

	if len(s.Names) > 1 {
		for _, name := range s.Names {
			c.declareVar(name)
		}
		c.imports[`"fmt"`] = true

		isDbQueryCall := false
		if callExpr, ok := s.Value.(*CallExpr); ok {
			if memExpr, ok := callExpr.Function.(*MemberExpr); ok {
				if objMem, ok := memExpr.Object.(*MemberExpr); ok {
					if ident, ok := objMem.Object.(*Identifier); ok && ident.Value == "db" {
						if memExpr.Field == "find" || memExpr.Field == "findOne" {
							isDbQueryCall = true
						}
					}
				}
			}
		}

		if isDbQueryCall {
			return fmt.Sprintf("%s, %s := safeCall(func() interface{} { r, e := %s; return [2]interface{}{r, e} })\n_ = %s\n_ = %s\n",
				s.Names[0], s.Names[1], val, s.Names[0], s.Names[1]), nil
		}

		return fmt.Sprintf("%s, %s := safeCall(func() interface{} { return %s })\n_ = %s\n_ = %s\n",
			s.Names[0], s.Names[1], val, s.Names[0], s.Names[1]), nil
	}

	if c.isDeclared(s.Name) {
		// Re-assignment: update type tracking
		inferred := c.getExpressionType(s.Value)
		targetType, ok := c.varTypes[s.Name]
		if ok && inferred == "interface{}" {
			switch targetType {
			case "int":
				val = fmt.Sprintf("toInt(%s)", val)
			case "float", "float64":
				val = fmt.Sprintf("toFloat64(%s)", val)
			case "bool":
				val = fmt.Sprintf("toBool(%s)", val)
			case "string":
				val = fmt.Sprintf("toString(%s)", val)
			default:
				if strings.HasPrefix(targetType, "*") || c.isStructType(targetType) {
					goType := strings.TrimSuffix(targetType, "?")
					if !strings.HasPrefix(goType, "*") {
						goType = "*" + goType
					}
					if val != "nil" {
						val = fmt.Sprintf("interface{}(%s).(%s)", val, goType)
					}
				}
			}
		} else if inferred != "interface{}" {
			c.varTypes[s.Name] = inferred
		}
		return fmt.Sprintf("%s = %s\n_ = %s\n", s.Name, val, s.Name), nil
	}
	c.declareVar(s.Name)
	goType := "interface{}"
	if s.Type != "" {
		goType = toGoType(s.Type)
		if goType == "interface{}" && !strings.HasSuffix(s.Type, "?") && !strings.Contains(s.Type, "|") {
			// Only use pointer-to-struct for plain struct type names
			goType = "*" + s.Type
		}
		c.varTypes[s.Name] = s.Type
 
		// Apply type coercion if the value is dynamic (interface{})
		inferred := c.getExpressionType(s.Value)
		if inferred == "interface{}" {
			switch s.Type {
			case "int":
				val = fmt.Sprintf("toInt(%s)", val)
			case "float", "float64":
				val = fmt.Sprintf("toFloat64(%s)", val)
			case "bool":
				val = fmt.Sprintf("toBool(%s)", val)
			case "string":
				val = fmt.Sprintf("toString(%s)", val)
			default:
				if c.isStructType(s.Type) {
					if val != "nil" {
						val = fmt.Sprintf("interface{}(%s).(*%s)", val, strings.TrimSuffix(s.Type, "?"))
					}
				}
			}
		}
	} else if structLit, ok := s.Value.(*StructLiteral); ok {
		typeArgStr := ""
		if len(structLit.TypeArgs) > 0 {
			typeArgStr = "[" + strings.Join(structLit.TypeArgs, ", ") + "]"
		}
		goType = "*" + structLit.TypeName + typeArgStr
		c.varTypes[s.Name] = structLit.TypeName + typeArgStr
	} else if callExpr, ok := s.Value.(*CallExpr); ok {
		if ident, ok := callExpr.Function.(*Identifier); ok {
			if retType, exists := c.funcReturnTypes[ident.Value]; exists {
				if c.isStructType(retType) {
					goType = "*" + strings.TrimSuffix(retType, "?")
					c.varTypes[s.Name] = retType
				} else {
					gt := toGoType(retType)
					if gt != "interface{}" {
						goType = gt
						c.varTypes[s.Name] = gt
					}
				}
			}
		}
	} else {
		inferred := c.getExpressionType(s.Value)
		if inferred != "interface{}" {
			// Don't emit typed slices for arrays — they need []interface{} for collection methods
			if strings.HasPrefix(inferred, "[]") {
				goType = "[]interface{}"
				c.varTypes[s.Name] = "[]interface{}"
			} else {
				goType = inferred
				c.varTypes[s.Name] = goType
			}
		}
	}
	if c.inFunction {
		return fmt.Sprintf("var %s %s = %s\n_ = %s\n", s.Name, goType, val, s.Name), nil
	}
	return fmt.Sprintf("var %s %s = %s\n", s.Name, goType, val), nil
}

func (c *Codegen) genReturnStmt(s *ReturnStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}

	if c.currentFn != nil && strings.Contains(c.currentFn.ReturnType, "|") {
		parts := strings.Split(c.currentFn.ReturnType, "|")
		hasError := false
		var successTypes []string
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "error" {
				hasError = true
			} else {
				successTypes = append(successTypes, part)
			}
		}

		if hasError {
			valType := c.getExpressionType(s.Value)
			isSuccess := false
			for _, st := range successTypes {
				if valType == toGoType(st) || valType == st {
					isSuccess = true
					break
				}
			}

			isStringLit := false
			if _, ok := s.Value.(*StringLiteral); ok {
				isStringLit = true
			}
			if _, ok := s.Value.(*FStringLiteral); ok {
				isStringLit = true
			}

			isError := false
			if isStringLit {
				hasStringSuccess := false
				for _, st := range successTypes {
					if strings.TrimSpace(st) == "string" {
						hasStringSuccess = true
						break
					}
				}
				if !hasStringSuccess {
					isError = true
				}
			} else if valType == "error" || (valType != "interface{}" && !isSuccess) {
				isError = true
			}

			if isError {
				return fmt.Sprintf("return [2]interface{}{nil, %s}\n", val), nil
			}
		}
	}
	if c.currentFn != nil && len(c.currentFn.TypeParams) > 0 {
		typeParamSet := make(map[string]bool)
		for _, tp := range c.currentFn.TypeParams {
			typeParamSet[tp] = true
		}
		if typeParamSet[c.currentFn.ReturnType] || (strings.HasPrefix(c.currentFn.ReturnType, "[]") && typeParamSet[strings.TrimPrefix(c.currentFn.ReturnType, "[]")]) {
			if val != "nil" {
				val = fmt.Sprintf("interface{}(%s).(%s)", val, c.currentFn.ReturnType)
			}
		}
	}
	if c.currentFn != nil && c.currentFn.ReturnType != "" {
		retType := toGoType(c.currentFn.ReturnType)
		if retType == "interface{}" && c.isStructType(c.currentFn.ReturnType) {
			if val != "nil" {
				goRetType := "*" + strings.TrimSuffix(c.currentFn.ReturnType, "?")
				val = fmt.Sprintf("interface{}(%s).(%s)", val, goRetType)
			}
		}
	}

	return fmt.Sprintf("return %s\n", val), nil
}

func (c *Codegen) genYieldStmt(s *YieldStmt) (string, error) {
	val, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("_streamChan <- %s\n", val), nil
}


func (c *Codegen) genFnDecl(s *FnDecl) (string, error) {
	c.inFunction = true
	oldFn := c.currentFn
	c.currentFn = s
	defer func() { c.currentFn = oldFn }()

	oldConcurrent := c.inConcurrentContext
	c.inConcurrentContext = hasConcurrency(s.Body)

	oldDeclared := c.declaredVars
	oldVarTypes := c.varTypes
	c.declaredVars = NewScope(nil)
	c.varTypes = make(map[string]string)
	for k, v := range oldVarTypes {
		c.varTypes[k] = v
	}

	typeParamSet := make(map[string]bool)
	for _, tp := range s.TypeParams {
		typeParamSet[tp] = true
	}

	var params []string
	for i, p := range s.Params {
		c.declareVar(p)
		pt := "interface{}"
		if i < len(s.ParamTypes) && s.ParamTypes[i] != "" {
			rawType := s.ParamTypes[i]
			if typeParamSet[rawType] {
				pt = rawType
			} else if strings.HasPrefix(rawType, "[]") && typeParamSet[strings.TrimPrefix(rawType, "[]")] {
				pt = "[]" + strings.TrimPrefix(rawType, "[]")
			} else {
				pt = toGoType(rawType)
			}
			c.varTypes[p] = pt
		}
		params = append(params, p+" "+pt)
	}

	bodyStr, err := c.genBlockStatement(s.Body)
	if err != nil {
		return "", err
	}

	c.declaredVars = oldDeclared
	c.varTypes = oldVarTypes
	c.inFunction = false
	c.inConcurrentContext = oldConcurrent

	retType := "interface{}"
	if s.ReturnType != "" {
		if typeParamSet[s.ReturnType] {
			retType = s.ReturnType
		} else if strings.HasPrefix(s.ReturnType, "[]") && typeParamSet[strings.TrimPrefix(s.ReturnType, "[]")] {
			retType = "[]" + strings.TrimPrefix(s.ReturnType, "[]")
		} else {
			retType = toGoType(s.ReturnType)
			if retType == "interface{}" && c.isStructType(s.ReturnType) {
				retType = "*" + strings.TrimSuffix(s.ReturnType, "?")
			}
		}
	}
	// If any parameter has an optional/union type, the function likely returns interface{} values
	// since those params are interface{} in Go and may be returned directly
	if retType != "interface{}" {
		for _, pt := range s.ParamTypes {
			if strings.HasSuffix(pt, "?") || strings.Contains(pt, "|") {
				retType = "interface{}"
				break
			}
		}
	}
	hasReturn := false
	if len(s.Body.Statements) > 0 {
		lastStmt := s.Body.Statements[len(s.Body.Statements)-1]
		if _, ok := lastStmt.(*ReturnStmt); ok {
			hasReturn = true
		}
	}

	if !hasReturn {
		if strings.HasSuffix(bodyStr, "}") {
			bodyStr = bodyStr[:len(bodyStr)-1] + fmt.Sprintf("\treturn %s\n}", zeroValue(retType))
		}
	}
	if s.IsResilient {
		var castStr string
		switch retType {
		case "interface{}":
			castStr = "_res"
		case "int":
			castStr = "toInt(_res)"
		case "float64":
			castStr = "toFloat64(_res)"
		case "bool":
			castStr = "toBool(_res)"
		case "string":
			castStr = "toString(_res)"
		default:
			castStr = fmt.Sprintf("_res.(%s)", retType)
		}
		bodyStr = fmt.Sprintf("{\n\t_wrapper := func() interface{} %s\n\t_res := runtime.ResilientCall(%q, _wrapper, %d, %q, %t)\n\treturn %s\n}",
			bodyStr, s.Name, s.Retries, s.Timeout, s.HasCircuitBreaker, castStr)
	}

	typeParamStr := ""
	if len(s.TypeParams) > 0 {
		var tps []string
		for i, tp := range s.TypeParams {
			constraint := "any"
			if i < len(s.TypeConstraints) && s.TypeConstraints[i] != "" {
				constraint = servConstraintToGo(s.TypeConstraints[i])
			}
			tps = append(tps, tp+" "+constraint)
		}
		typeParamStr = "[" + strings.Join(tps, ", ") + "]"
	}

	return fmt.Sprintf("func %s%s(%s) %s %s\n\n", s.Name, typeParamStr, strings.Join(params, ", "), retType, bodyStr), nil
}

func (c *Codegen) genTryCatchStmt(s *TryCatchStmt) (string, error) {
	oldDeclared := c.declaredVars
	c.declaredVars = NewScope(oldDeclared)
	c.declareVar(s.Param)

	tryBodyStr, err := c.genBlockStatement(s.TryBody)
	if err != nil {
		return "", err
	}

	catchBodyStr, err := c.genBlockStatement(s.CatchBody)
	if err != nil {
		return "", err
	}

	c.declaredVars = oldDeclared

	return fmt.Sprintf("func() {\n\tdefer func() {\n\t\tif r := recover(); r != nil {\n\t\t\tvar %s interface{} = r\n\t\t\t_ = %s\n\t\t\t%s\n\t\t}\n\t}()\n\t%s\n}()\n", s.Param, s.Param, catchBodyStr, tryBodyStr), nil
}

func (c *Codegen) genExprStmt(s *ExprStmt) (string, error) {
	expr, err := c.genExpression(s.Value)
	if err != nil {
		return "", err
	}
	if !c.inFunction {
		return fmt.Sprintf("func init() {\n\t%s\n}\n\n", expr), nil
	}
	return expr + "\n", nil
}

func (c *Codegen) genActorDecl(s *ActorDecl) (string, error) {
	c.currentActor = s
	c.actorFields = make(map[string]bool)

	for _, p := range s.Params {
		c.actorFields[p] = true
	}

	var stateVars []string
	var initStmts []string
	var methods []string

	for _, stmt := range s.Body.Statements {
		switch st := stmt.(type) {
		case *LetStmt:
			c.actorFields[st.Name] = true
			stateVars = append(stateVars, st.Name)
			valStr, err := c.genExpression(st.Value)
			if err != nil {
				return "", err
			}
			initStmts = append(initStmts, fmt.Sprintf("actor.%s = %s", st.Name, valStr))
		case *FnDecl:
			oldInFunction := c.inFunction
			c.inFunction = true
			
			oldDeclared := c.declaredVars
			c.declaredVars = NewScope(nil)
			c.declareVar("self")
			
			var params []string
			for _, pName := range st.Params {
				c.declareVar(pName)
				params = append(params, pName+" interface{}")
			}
			
			bodyStr, err := c.genBlockStatement(st.Body)
			if err != nil {
				return "", err
			}
			
			c.declaredVars = oldDeclared
			c.inFunction = oldInFunction
			
			methodCode := fmt.Sprintf("func (self *%s_Actor) %s(%s) interface{} %s\n\n", s.Name, st.Name, strings.Join(params, ", "), bodyStr)
			methods = append(methods, methodCode)
		}
	}

	var structFields []string
	structFields = append(structFields, "mailbox chan runtime.ActorMessage")
	for _, pName := range s.Params {
		structFields = append(structFields, pName+" interface{}")
	}
	for _, svName := range stateVars {
		structFields = append(structFields, svName+" interface{}")
	}

	structDef := fmt.Sprintf("type %s_Actor struct {\n\t%s\n}\n\n", s.Name, strings.Join(structFields, "\n\t"))

	runMethod := fmt.Sprintf(`func (self *%s_Actor) run() {
	for msg := range self.mailbox {
		res := self.receive(msg.Payload)
		if msg.Reply != nil {
			msg.Reply <- res
		}
	}
}

`, s.Name)

	var spawnParams []string
	var structInits []string
	structInits = append(structInits, "mailbox: mailbox")
	for _, pName := range s.Params {
		spawnParams = append(spawnParams, pName+" interface{}")
		structInits = append(structInits, fmt.Sprintf("%s: %s", pName, pName))
	}

	selfDecl := ""
	if len(initStmts) > 0 {
		selfDecl = "self := actor"
	}

	spawnConstructor := fmt.Sprintf(`func Spawn_%s(%s) *runtime.ActorRef {
	mailbox := make(chan runtime.ActorMessage, 100)
	actor := &%s_Actor{
		%s,
	}
	%s
	%s
	go actor.run()
	return &runtime.ActorRef{Mailbox: mailbox}
}

`, s.Name, strings.Join(spawnParams, ", "), s.Name, strings.Join(structInits, ",\n\t\t"), selfDecl, strings.Join(initStmts, "\n\t"))

	c.currentActor = nil
	c.actorFields = nil

	var out bytes.Buffer
	out.WriteString(structDef)
	out.WriteString(runMethod)
	out.WriteString(spawnConstructor)
	for _, m := range methods {
		out.WriteString(m)
	}

	return out.String(), nil
}

// genWorkflowDecl compiles a workflow block into:
//  1. A Go function func workflow_<Name>(ctx *runtime.WorkflowCtx, param interface{}) interface{}
//  2. An init() registration call: runtime.RegisterWorkflow("Name", workflow_<Name>)
func (c *Codegen) genWorkflowDecl(s *WorkflowDecl) (string, error) {
	oldInFunction := c.inFunction
	c.inFunction = true
	oldDeclared := c.declaredVars
	c.declaredVars = NewScope(oldDeclared)
	// The param name is available as a local variable.
	if s.Param != "" {
		c.declareVar(s.Param)
	}
	// The workflow context is available as _wfCtx.
	c.declareVar("_wfCtx")

	// Mark that we are inside a workflow so await expressions use _wfCtx.Step().
	c.currentWorkflow = s
	bodyStr, err := c.genBlockStatement(s.Body)
	c.currentWorkflow = nil
	if err != nil {
		return "", err
	}

	c.declaredVars = oldDeclared
	c.inFunction = oldInFunction

	paramDecl := "_ interface{}"
	if s.Param != "" {
		paramDecl = s.Param + " interface{}"
	}

	fnName := "workflow_" + s.Name

	var out bytes.Buffer
	out.WriteString(fmt.Sprintf(
		"func %s(_wfCtx *runtime.WorkflowCtx, %s) interface{} %s\n\n",
		fnName, paramDecl, bodyStr,
	))
	// Emit an init() that registers this workflow with the runtime.
	out.WriteString(fmt.Sprintf(
		"func init() { runtime.RegisterWorkflow(%q, func(ctx *runtime.WorkflowCtx, p interface{}) interface{} { return %s(ctx, p) }) }\n\n",
		s.Name, fnName,
	))
	return out.String(), nil
}
