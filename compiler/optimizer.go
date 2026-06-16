package compiler

import (
	"fmt"
	"strconv"
)

// Optimize performs ahead-of-time AST optimization passes.
func Optimize(program *Program) *Program {
	if program == nil {
		return nil
	}
	var optimizedStmts []Statement
	for _, stmt := range program.Statements {
		optStmt := optimizeStatement(stmt)
		if optStmt != nil {
			optimizedStmts = append(optimizedStmts, optStmt)
		}
	}
	return &Program{Statements: optimizedStmts}
}

func optimizeStatement(stmt Statement) Statement {
	if stmt == nil {
		return nil
	}

	switch s := stmt.(type) {
	case *BlockStmt:
		var optimizedStmts []Statement
		for _, subStmt := range s.Statements {
			optSub := optimizeStatement(subStmt)
			if optSub != nil {
				optimizedStmts = append(optimizedStmts, optSub)
				// Unreachable Code Elimination: stop processing statements in this block
				// after a return, break, or continue statement.
				if isTerminator(optSub) {
					break
				}
			}
		}
		s.Statements = optimizedStmts
		return s

	case *LetStmt:
		s.Value = optimizeExpression(s.Value)
		return s

	case *ExprStmt:
		s.Value = optimizeExpression(s.Value)
		return s

	case *ReturnStmt:
		if s.Value != nil {
			s.Value = optimizeExpression(s.Value)
		}
		return s

	case *IfStmt:
		cond := optimizeExpression(s.Condition)
		s.Condition = cond

		// Dead Branch Elimination
		if boolLit, ok := cond.(*BooleanLiteral); ok {
			if boolLit.Value {
				// if true -> return body statements wrapped in block or just return the body
				if s.Body != nil {
					return optimizeStatement(s.Body)
				}
				return nil
			} else {
				// if false -> return else body statements
				if s.ElseBody != nil {
					return optimizeStatement(s.ElseBody)
				}
				return nil
			}
		}

		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		if s.ElseBody != nil {
			s.ElseBody = optimizeStatement(s.ElseBody).(*BlockStmt)
		}
		return s

	case *ForStmt:
		if s.Iterable != nil {
			s.Iterable = optimizeExpression(s.Iterable)
		}
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *FnDecl:
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *RouteStmt:
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *ToolStmt:
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *MigrationStmt:
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *EveryStmt:
		if s.Interval != nil {
			s.Interval = optimizeExpression(s.Interval)
		}
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *CronStmt:
		if s.Cron != nil {
			s.Cron = optimizeExpression(s.Cron)
		}
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *SubscribeStmt:
		if s.Topic != nil {
			s.Topic = optimizeExpression(s.Topic)
		}
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *PublishStmt:
		if s.Topic != nil {
			s.Topic = optimizeExpression(s.Topic)
		}
		if s.Value != nil {
			s.Value = optimizeExpression(s.Value)
		}
		return s

	case *SpawnStmt:
		if s.Call != nil {
			s.Call = optimizeExpression(s.Call)
		}
		if s.Limit != nil {
			s.Limit = optimizeExpression(s.Limit)
		}
		return s

	case *MockStmt:
		if s.Target != nil {
			s.Target = optimizeExpression(s.Target)
		}
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *TestStmt:
		if s.Body != nil {
			s.Body = optimizeStatement(s.Body).(*BlockStmt)
		}
		return s

	case *TryCatchStmt:
		if s.TryBody != nil {
			s.TryBody = optimizeStatement(s.TryBody).(*BlockStmt)
		}
		if s.CatchBody != nil {
			s.CatchBody = optimizeStatement(s.CatchBody).(*BlockStmt)
		}
		return s

	default:
		return stmt
	}
}

func optimizeExpression(expr Expression) Expression {
	if expr == nil {
		return nil
	}

	switch e := expr.(type) {
	case *InfixExpr:
		left := optimizeExpression(e.Left)
		right := optimizeExpression(e.Right)
		e.Left = left
		e.Right = right

		// Constant Folding
		// 1. Integer Arithmetic & Comparison
		if lInt, ok1 := left.(*IntegerLiteral); ok1 {
			if rInt, ok2 := right.(*IntegerLiteral); ok2 {
				if folded := foldIntegers(lInt.Value, rInt.Value, e.Operator, e.Token); folded != nil {
					return folded
				}
			}
			if rFloat, ok2 := right.(*FloatLiteral); ok2 {
				if folded := foldFloats(float64(lInt.Value), rFloat.Value, e.Operator, e.Token); folded != nil {
					return folded
				}
			}
		}

		// 2. Float Arithmetic & Comparison
		if lFloat, ok1 := left.(*FloatLiteral); ok1 {
			if rFloat, ok2 := right.(*FloatLiteral); ok2 {
				if folded := foldFloats(lFloat.Value, rFloat.Value, e.Operator, e.Token); folded != nil {
					return folded
				}
			}
			if rInt, ok2 := right.(*IntegerLiteral); ok2 {
				if folded := foldFloats(lFloat.Value, float64(rInt.Value), e.Operator, e.Token); folded != nil {
					return folded
				}
			}
		}

		// 3. String Concatenation & Comparison
		if lStr, ok1 := left.(*StringLiteral); ok1 {
			if rStr, ok2 := right.(*StringLiteral); ok2 {
				if folded := foldStrings(lStr.Value, rStr.Value, e.Operator, e.Token); folded != nil {
					return folded
				}
			}
		}

		// 4. Boolean Operations
		if lBool, ok1 := left.(*BooleanLiteral); ok1 {
			if rBool, ok2 := right.(*BooleanLiteral); ok2 {
				if folded := foldBooleans(lBool.Value, rBool.Value, e.Operator, e.Token); folded != nil {
					return folded
				}
			}
		}

		return e

	case *PrefixExpr:
		right := optimizeExpression(e.Right)
		e.Right = right

		// Constant Folding
		switch e.Operator {
		case "-":
			if rInt, ok := right.(*IntegerLiteral); ok {
				tok := e.Token
				tok.Type = TOKEN_INT
				tok.Literal = strconv.FormatInt(-rInt.Value, 10)
				return &IntegerLiteral{Token: tok, Value: -rInt.Value}
			}
			if rFloat, ok := right.(*FloatLiteral); ok {
				tok := e.Token
				tok.Type = TOKEN_FLOAT
				tok.Literal = fmt.Sprintf("%f", -rFloat.Value)
				return &FloatLiteral{Token: tok, Value: -rFloat.Value}
			}
		case "!":
			if rBool, ok := right.(*BooleanLiteral); ok {
				tok := e.Token
				tok.Type = TOKEN_TRUE
				if rBool.Value {
					tok.Type = TOKEN_FALSE
				}
				tok.Literal = strconv.FormatBool(!rBool.Value)
				return &BooleanLiteral{Token: tok, Value: !rBool.Value}
			}
		}
		return e

	case *CallExpr:
		if e.Function != nil {
			e.Function = optimizeExpression(e.Function)
		}
		for i, arg := range e.Arguments {
			e.Arguments[i] = optimizeExpression(arg)
		}
		return e

	case *ArrayLiteral:
		for i, el := range e.Elements {
			e.Elements[i] = optimizeExpression(el)
		}
		return e

	case *MapLiteral:
		for k, v := range e.Pairs {
			e.Pairs[k] = optimizeExpression(v)
		}
		for i, s := range e.Spreads {
			e.Spreads[i].Value = optimizeExpression(s.Value)
		}
		return e

	case *StructLiteral:
		for k, v := range e.Fields {
			e.Fields[k] = optimizeExpression(v)
		}
		return e

	case *MemberExpr:
		e.Object = optimizeExpression(e.Object)
		return e

	case *IndexExpr:
		e.Left = optimizeExpression(e.Left)
		e.Index = optimizeExpression(e.Index)
		return e

	case *SliceExpr:
		e.Left = optimizeExpression(e.Left)
		if e.Start != nil {
			e.Start = optimizeExpression(e.Start)
		}
		if e.End != nil {
			e.End = optimizeExpression(e.End)
		}
		return e

	case *AwaitExpr:
		e.Value = optimizeExpression(e.Value)
		return e

	case *ErrorPropExpr:
		e.Value = optimizeExpression(e.Value)
		return e

	case *FnLiteral:
		if e.ArrowExpr != nil {
			e.ArrowExpr = optimizeExpression(e.ArrowExpr)
		}
		if e.Body != nil {
			e.Body = optimizeStatement(e.Body).(*BlockStmt)
		}
		return e

	case *AssertExpr:
		e.Cond = optimizeExpression(e.Cond)
		return e

	default:
		return expr
	}
}

func isTerminator(stmt Statement) bool {
	switch stmt.(type) {
	case *ReturnStmt, *BreakStmt, *ContinueStmt:
		return true
	}
	return false
}

func foldIntegers(l, r int64, op string, tok Token) Expression {
	var val int64
	switch op {
	case "+":
		val = l + r
	case "-":
		val = l - r
	case "*":
		val = l * r
	case "/":
		if r != 0 {
			val = l / r
		} else {
			return nil
		}
	case "%":
		if r != 0 {
			val = l % r
		} else {
			return nil
		}
	default:
		// Comparisons
		res := false
		switch op {
		case "==": res = l == r
		case "!=": res = l != r
		case "<": res = l < r
		case ">": res = l > r
		case "<=": res = l <= r
		case ">=": res = l >= r
		default: return nil
		}
		tok.Type = TOKEN_TRUE
		if !res {
			tok.Type = TOKEN_FALSE
		}
		tok.Literal = strconv.FormatBool(res)
		return &BooleanLiteral{Token: tok, Value: res}
	}
	tok.Type = TOKEN_INT
	tok.Literal = strconv.FormatInt(val, 10)
	return &IntegerLiteral{Token: tok, Value: val}
}

func foldFloats(l, r float64, op string, tok Token) Expression {
	var val float64
	switch op {
	case "+":
		val = l + r
	case "-":
		val = l - r
	case "*":
		val = l * r
	case "/":
		if r != 0 {
			val = l / r
		} else {
			return nil
		}
	default:
		// Comparisons
		res := false
		switch op {
		case "==": res = l == r
		case "!=": res = l != r
		case "<": res = l < r
		case ">": res = l > r
		case "<=": res = l <= r
		case ">=": res = l >= r
		default: return nil
		}
		tok.Type = TOKEN_TRUE
		if !res {
			tok.Type = TOKEN_FALSE
		}
		tok.Literal = strconv.FormatBool(res)
		return &BooleanLiteral{Token: tok, Value: res}
	}
	tok.Type = TOKEN_FLOAT
	tok.Literal = fmt.Sprintf("%f", val)
	return &FloatLiteral{Token: tok, Value: val}
}

func foldStrings(l, r string, op string, tok Token) Expression {
	switch op {
	case "+":
		tok.Type = TOKEN_STRING
		tok.Literal = l + r
		return &StringLiteral{Token: tok, Value: l + r}
	case "==":
		res := l == r
		tok.Type = TOKEN_TRUE
		if !res { tok.Type = TOKEN_FALSE }
		tok.Literal = strconv.FormatBool(res)
		return &BooleanLiteral{Token: tok, Value: res}
	case "!=":
		res := l != r
		tok.Type = TOKEN_TRUE
		if !res { tok.Type = TOKEN_FALSE }
		tok.Literal = strconv.FormatBool(res)
		return &BooleanLiteral{Token: tok, Value: res}
	case "<":
		res := l < r
		tok.Type = TOKEN_TRUE
		if !res { tok.Type = TOKEN_FALSE }
		tok.Literal = strconv.FormatBool(res)
		return &BooleanLiteral{Token: tok, Value: res}
	case ">":
		res := l > r
		tok.Type = TOKEN_TRUE
		if !res { tok.Type = TOKEN_FALSE }
		tok.Literal = strconv.FormatBool(res)
		return &BooleanLiteral{Token: tok, Value: res}
	case "<=":
		res := l <= r
		tok.Type = TOKEN_TRUE
		if !res { tok.Type = TOKEN_FALSE }
		tok.Literal = strconv.FormatBool(res)
		return &BooleanLiteral{Token: tok, Value: res}
	case ">=":
		res := l >= r
		tok.Type = TOKEN_TRUE
		if !res { tok.Type = TOKEN_FALSE }
		tok.Literal = strconv.FormatBool(res)
		return &BooleanLiteral{Token: tok, Value: res}
	}
	return nil
}

func foldBooleans(l, r bool, op string, tok Token) Expression {
	res := false
	switch op {
	case "&&":
		res = l && r
	case "||":
		res = l || r
	case "==":
		res = l == r
	case "!=":
		res = l != r
	default:
		return nil
	}
	tok.Type = TOKEN_TRUE
	if !res { tok.Type = TOKEN_FALSE }
	tok.Literal = strconv.FormatBool(res)
	return &BooleanLiteral{Token: tok, Value: res}
}
