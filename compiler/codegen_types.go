package compiler

import "strings"

func toGoType(t string) string {
	// Handle optional types: int? -> interface{} (can be nil)
	if strings.HasSuffix(t, "?") {
		return "interface{}"
	}
	// Handle union types: int|string -> interface{}
	if strings.Contains(t, "|") {
		return "interface{}"
	}
	switch t {
	case "int":
		return "int"
	case "string":
		return "string"
	case "float":
		return "float64"
	case "bool":
		return "bool"
	default:
		// Handle array types: []int, []string, []T
		if strings.HasPrefix(t, "[]") {
			elemType := toGoType(strings.TrimPrefix(t, "[]"))
			if elemType == "interface{}" {
				// Could be a generic type param or struct — pass through
				elemType = strings.TrimPrefix(t, "[]")
			}
			return "[]" + elemType
		}
		return "interface{}"
	}
}

// servConstraintToGo maps Serv constraint names to Go constraint interfaces.
func servConstraintToGo(constraint string) string {
	switch constraint {
	case "Comparable", "comparable":
		return "comparable"
	case "Numeric", "numeric":
		return "~int | ~int64 | ~float64"
	case "Integer", "integer":
		return "~int | ~int64 | ~int32 | ~uint | ~uint64"
	case "Float", "float":
		return "~float32 | ~float64"
	case "Ordered", "ordered":
		return "~int | ~int64 | ~float64 | ~string"
	case "Signed", "signed":
		return "~int | ~int64 | ~int32 | ~float64"
	case "Unsigned", "unsigned":
		return "~uint | ~uint64 | ~uint32"
	case "Stringer", "stringer":
		return "fmt.Stringer"
	default:
		// Custom constraint — pass through as-is (user-defined interface)
		return constraint
	}
}

func zeroValue(goType string) string {
	switch goType {
	case "int":
		return "0"
	case "float64":
		return "0.0"
	case "bool":
		return "false"
	case "string":
		return `""`
	default:
		return "nil"
	}
}

func (c *Codegen) getExpressionType(expr Expression) string {
	switch e := expr.(type) {
	case *Identifier:
		if t, ok := c.varTypes[e.Value]; ok {
			return t
		}
		return "interface{}"
	case *IntegerLiteral:
		return "int"
	case *FloatLiteral:
		return "float64"
	case *ArrayLiteral:
		// Infer element type from homogeneous arrays
		if len(e.Elements) > 0 {
			firstType := c.getExpressionType(e.Elements[0])
			if firstType != "interface{}" {
				homogeneous := true
				for _, el := range e.Elements[1:] {
					if c.getExpressionType(el) != firstType {
						homogeneous = false
						break
					}
				}
				if homogeneous {
					return "[]" + firstType
				}
			}
		}
		return "[]interface{}"
	case *StringLiteral:
		return "string"
	case *FStringLiteral:
		return "string"
	case *DurationLiteral:
		return "string"
	case *BooleanLiteral:
		return "bool"
	case *NilLiteral:
		return "interface{}"
	case *StructLiteral:
		return e.TypeName
	case *SelfExpr:
		if t, ok := c.varTypes["self"]; ok {
			return t
		}
		return "interface{}"
	case *CallExpr:
		// Infer return type from known functions
		if ident, ok := e.Function.(*Identifier); ok {
			if retType, exists := c.funcReturnTypes[ident.Value]; exists {
				goType := toGoType(retType)
				if goType != "interface{}" {
					return goType
				}
				// If it's a struct type
				if c.structTypes[retType] {
					return retType
				}
			}
		}
		// Infer return type from collection/string method calls
		if memExpr, ok := e.Function.(*MemberExpr); ok {
			// Struct method calls: check if the receiver type has a known method return type
			if ident, ok := memExpr.Object.(*Identifier); ok {
				if objType, exists := c.varTypes[ident.Value]; exists && c.structTypes[objType] {
					// Look up method return type
					methodKey := objType + "." + memExpr.Field
					if retType, exists := c.funcReturnTypes[methodKey]; exists {
						goType := toGoType(retType)
						if goType != "interface{}" {
							return goType
						}
					}
				}
			}
			switch memExpr.Field {
			case "filter":
				// filter preserves the collection type
				if ident, ok := memExpr.Object.(*Identifier); ok {
					if varType, exists := c.varTypes[ident.Value]; exists && strings.HasPrefix(varType, "[]") {
						return varType
					}
				}
				return "[]interface{}"
			case "map":
				return "[]interface{}"
			case "find":
				return "interface{}"
			case "reduce":
				return "interface{}"
			case "length":
				return "int"
			case "contains", "startsWith", "endsWith", "includes":
				return "bool"
			case "split":
				return "[]interface{}"
			case "trim", "replace", "toUpper", "toLower", "substring", "repeat":
				return "string"
			case "indexOf":
				return "int"
			case "push":
				return "[]interface{}"
			}
		}
		return "interface{}"
	case *MemberExpr:
		// Struct field access: if the object has a known struct type, look up field type
		if ident, ok := e.Object.(*Identifier); ok {
			if objType, exists := c.varTypes[ident.Value]; exists {
				if fields, ok := c.structFields[objType]; ok {
					for _, f := range fields {
						if f.Name == e.Field {
							goType := toGoType(f.Type)
							if goType != "interface{}" {
								return goType
							}
							// Check if field type is a known struct
							if c.structTypes[f.Type] {
								return f.Type
							}
							return "interface{}"
						}
					}
				}
			}
		}
		// Self field access
		if _, isSelf := e.Object.(*SelfExpr); isSelf {
			if selfType, ok := c.varTypes["self"]; ok {
				if fields, ok := c.structFields[selfType]; ok {
					for _, f := range fields {
						if f.Name == e.Field {
							goType := toGoType(f.Type)
							if goType != "interface{}" {
								return goType
							}
							return "interface{}"
						}
					}
				}
			}
		}
		// Built-in property types
		if e.Field == "length" {
			return "int"
		}
		return "interface{}"
	case *InfixExpr:
		switch e.Operator {
		case "==", "!=", "<", ">", "<=", ">=":
			return "bool"
		case "&", "|", "^", "<<", ">>":
			return "int"
		case "%":
			return "int"
		default:
			lt := c.getExpressionType(e.Left)
			rt := c.getExpressionType(e.Right)
			if lt == rt && (lt == "int" || lt == "float64" || lt == "string" || lt == "bool") {
				return lt
			}
			if lt == "interface{}" && (rt == "int" || rt == "float64" || rt == "string") {
				return rt
			}
			if rt == "interface{}" && (lt == "int" || lt == "float64" || lt == "string") {
				return lt
			}
			// Mixed numeric: int + float64 = float64
			if (lt == "int" && rt == "float64") || (lt == "float64" && rt == "int") {
				return "float64"
			}
			return "interface{}"
		}
	case *IndexExpr:
		// If indexing a known typed slice, return the element type
		if ident, ok := e.Left.(*Identifier); ok {
			if varType, exists := c.varTypes[ident.Value]; exists {
				if strings.HasPrefix(varType, "[]") && varType != "[]interface{}" {
					return strings.TrimPrefix(varType, "[]")
				}
			}
		}
		return "interface{}"
	case *SliceExpr:
		// Slicing preserves the type
		if ident, ok := e.Left.(*Identifier); ok {
			if varType, exists := c.varTypes[ident.Value]; exists {
				if strings.HasPrefix(varType, "[]") {
					return varType
				}
				if varType == "string" {
					return "string"
				}
			}
		}
		return "interface{}"
	case *CompoundAssignExpr:
		if varType, ok := c.varTypes[e.Name]; ok {
			return varType
		}
		return "interface{}"
	case *PrefixExpr:
		if e.Operator == "!" {
			return "bool"
		}
		// Negation preserves the type
		return c.getExpressionType(e.Right)
	case *AssertExpr:
		return "interface{}"
	case *AwaitExpr:
		return "interface{}"
	case *ErrorPropExpr:
		return c.getExpressionType(e.Value)
	case *FnLiteral:
		return "interface{}"
	default:
		return "interface{}"
	}
}

// capitalizeFirst uppercases the first rune of s.
func capitalizeFirst(s string) string {
	if s == "" {
		return s
	}
	return strings.ToUpper(s[:1]) + s[1:]
}

// sanitizeTestName converts a human-readable test name into a valid Go identifier.
func sanitizeTestName(name string) string {
	words := strings.FieldsFunc(name, func(r rune) bool {
		return r == ' ' || r == '-' || r == '_' || r == '.' || r == '/'
	})
	var out strings.Builder
	for _, w := range words {
		if w == "" {
			continue
		}
		out.WriteString(strings.ToUpper(w[:1]))
		out.WriteString(w[1:])
	}
	result := out.String()
	if result == "" {
		return "Unnamed"
	}
	return result
}


