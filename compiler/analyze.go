package compiler

import (
	"fmt"
	"strings"
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
		return analyzeBlock(s.Body, nil)
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
