//go:build js && wasm

package main

import (
	"fmt"
	"strings"
	"syscall/js"

	"serv/compiler"
)

func main() {
	c := make(chan struct{})
	js.Global().Set("compileServ", js.FuncOf(compileServ))
	js.Global().Set("formatServ", js.FuncOf(formatServ))
	fmt.Println("Serv Compiler WASM Initialized")
	<-c
}

func compileServ(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "Missing source code argument"})
	}
	source := args[0].String()

	lexer := compiler.NewLexer(source)
	parser := compiler.NewParser(lexer)
	program := parser.ParseProgram()

	if len(parser.Errors()) > 0 {
		formatted := compiler.FormatDiagnostics(parser.Errors(), source)
		return js.ValueOf(map[string]any{
			"error":  "syntax_error",
			"output": formatted,
		})
	}

	// Run static analysis
	diags := compiler.Analyze(program)
	var diagMsgs []any
	hasErrors := false
	for _, d := range diags {
		if d.Severity == "error" {
			hasErrors = true
		}
		diagMsgs = append(diagMsgs, map[string]any{
			"line":     d.Line,
			"col":      d.Col,
			"severity": d.Severity,
			"message":  d.Message,
		})
	}

	analysisFormatted := ""
	if len(diags) > 0 {
		analysisFormatted = compiler.FormatAnalysisDiagnostics(diags, source)
	}

	if hasErrors {
		return js.ValueOf(map[string]any{
			"error":  "analysis_error",
			"output": analysisFormatted,
		})
	}

	codegen := compiler.NewCodegen(program)
	goCode, err := codegen.Generate()
	if err != nil {
		return js.ValueOf(map[string]any{
			"error":  "codegen_error",
			"output": err.Error(),
		})
	}

	goCode += "\n" + codegen.GenerateHelpers()
	goCode += "\n" + codegen.GenerateMainFunc()

	return js.ValueOf(map[string]any{
		"goCode":            goCode,
		"analysisOutput":    analysisFormatted,
		"diagnostics":       diagMsgs,
	})
}

func formatServ(this js.Value, args []js.Value) any {
	if len(args) < 1 {
		return js.ValueOf(map[string]any{"error": "Missing source code argument"})
	}
	content := args[0].String()

	lines := strings.Split(content, "\n")
	var result []string
	indentLevel := 0
	indent := "    " // 4 spaces
	prevEmpty := false
	prevWasBlock := false // track if previous non-empty line ended a block or was a top-level decl

	topLevelKeywords := map[string]bool{
		"server": true, "database": true, "cache": true, "broker": true,
		"route": true, "fn": true, "every": true, "cron": true,
		"subscribe": true, "test": true, "struct": true, "interface": true,
		"middleware": true, "ws": true, "enum": true, "validate": true,
		"type": true, "export": true,
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		if trimmed == "" {
			if !prevEmpty {
				result = append(result, "")
				prevEmpty = true
			}
			continue
		}
		prevEmpty = false

		netBraces := countNetBraces(trimmed)

		if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") {
			indentLevel--
			if indentLevel < 0 {
				indentLevel = 0
			}
		}

		if indentLevel == 0 && i > 0 {
			fields := strings.Fields(trimmed)
			if len(fields) > 0 {
				firstWord := fields[0]
				firstWord = strings.TrimRight(firstWord, "({[")
				if topLevelKeywords[firstWord] && !prevEmpty && len(result) > 0 && result[len(result)-1] != "" {
					if !prevWasBlock {
						result = append(result, "")
					}
				}
			}
		}

		formatted := strings.Repeat(indent, indentLevel) + trimmed
		result = append(result, formatted)

		prevWasBlock = strings.HasPrefix(trimmed, "}")

		if netBraces > 0 {
			indentLevel += netBraces
		} else if netBraces < 0 && !strings.HasPrefix(trimmed, "}") && !strings.HasPrefix(trimmed, "]") {
			indentLevel += netBraces
			if indentLevel < 0 {
				indentLevel = 0
			}
		}
	}

	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	output := strings.Join(result, "\n") + "\n"

	return js.ValueOf(map[string]any{
		"formatted": output,
	})
}

func countNetBraces(line string) int {
	net := 0
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inString {
			if ch == '\\' && i+1 < len(line) {
				i++ // skip escaped char
				continue
			}
			if ch == stringChar {
				inString = false
			}
			continue
		}
		if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
			break // rest of line is comment
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			inString = true
			stringChar = ch
			continue
		}
		switch ch {
		case '{':
			net++
		case '}':
			net--
		}
	}
	return net
}
