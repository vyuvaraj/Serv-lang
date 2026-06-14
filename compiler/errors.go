package compiler

import (
	"fmt"
	"strings"
)

// DiagnosticError represents a rich compiler error with source context.
type DiagnosticError struct {
	Line       int
	Col        int
	Message    string
	Source     string // the source line content
	Suggestion string // optional "did you mean X?"
	Hint       string // optional contextual hint
}

// FormatDiagnostics takes raw parser errors and source code, returns formatted diagnostics.
func FormatDiagnostics(errors []string, source string) string {
	if len(errors) == 0 {
		return ""
	}

	lines := strings.Split(source, "\n")
	var out strings.Builder

	for _, errMsg := range errors {
		line, col, msg := parseErrorLocation(errMsg)

		out.WriteString(fmt.Sprintf("  error: %s\n", msg))

		// Show source line with caret
		if line > 0 && line <= len(lines) {
			srcLine := lines[line-1]
			lineNum := fmt.Sprintf(" %d | ", line)
			out.WriteString(fmt.Sprintf("  %s%s\n", lineNum, srcLine))
			// Caret pointer
			padding := strings.Repeat(" ", len(lineNum)+col-1)
			out.WriteString(fmt.Sprintf("  %s^\n", padding))
		}

		// Try to add suggestion
		suggestion := suggestFix(msg, source)
		if suggestion != "" {
			out.WriteString(fmt.Sprintf("  hint: %s\n", suggestion))
		}

		out.WriteString("\n")
	}

	return out.String()
}

// parseErrorLocation extracts line, col, and message from a parser error string.
// Format: "[Line X, Col Y] message"
func parseErrorLocation(err string) (int, int, string) {
	line, col := 0, 0
	msg := err

	if strings.HasPrefix(err, "[Line ") {
		// Parse [Line X, Col Y] prefix
		closeBracket := strings.Index(err, "]")
		if closeBracket > 0 {
			prefix := err[1:closeBracket] // "Line X, Col Y"
			msg = strings.TrimSpace(err[closeBracket+1:])
			fmt.Sscanf(prefix, "Line %d, Col %d", &line, &col)
		}
	}

	return line, col, msg
}

// suggestFix analyzes an error message and suggests a fix.
func suggestFix(errMsg string, _ string) string {
	// "no prefix parse function for X found" — unknown token/keyword
	if strings.Contains(errMsg, "no prefix parse function for") {
		token := extractToken(errMsg, "no prefix parse function for ", " found")
		if token != "" {
			if suggestion := findSimilarKeyword(token); suggestion != "" {
				return fmt.Sprintf("did you mean '%s'?", suggestion)
			}
			if token == "}" || token == ")" || token == "]" {
				return "unexpected closing bracket — check for missing opening bracket or extra closing bracket"
			}
		}
	}

	// "expected next token to be X, got Y instead"
	if strings.Contains(errMsg, "expected next token to be") {
		if strings.Contains(errMsg, "got }") || strings.Contains(errMsg, "got EOF") {
			return "you might be missing a closing bracket or semicolon earlier in the block"
		}
		if strings.Contains(errMsg, "expected next token to be =") {
			return "variable declarations use: let name = value"
		}
		if strings.Contains(errMsg, "expected next token to be {") {
			return "block body requires opening brace '{'"
		}
		if strings.Contains(errMsg, "expected next token to be (") {
			return "function calls require parentheses: fn name(args)"
		}
	}

	// "could not parse X as integer"
	if strings.Contains(errMsg, "could not parse") && strings.Contains(errMsg, "as integer") {
		return "make sure the number is a valid integer (no decimals, no letters)"
	}

	return ""
}

// extractToken extracts a token name from an error message given prefix and suffix.
func extractToken(msg, prefix, suffix string) string {
	start := strings.Index(msg, prefix)
	if start < 0 {
		return ""
	}
	start += len(prefix)
	end := strings.Index(msg[start:], suffix)
	if end < 0 {
		return strings.TrimSpace(msg[start:])
	}
	return strings.TrimSpace(msg[start : start+end])
}

// findSimilarKeyword checks if a token looks like a typo of a known keyword.
func findSimilarKeyword(token string) string {
	keywords := []string{
		"server", "database", "cache", "broker", "route", "fn", "let", "return",
		"import", "export", "from", "try", "catch", "match", "test", "assert",
		"enum", "struct", "interface", "middleware", "if", "else", "for", "in",
		"spawn", "every", "cron", "subscribe", "publish", "true", "false", "nil",
		"self", "await", "ws", "validate", "type", "use", "limit", "tool",
	}

	token = strings.ToLower(token)
	for _, kw := range keywords {
		if levenshtein(token, kw) <= 2 && token != kw {
			return kw
		}
	}
	return ""
}

// levenshtein computes the edit distance between two strings.
func levenshtein(a, b string) int {
	la, lb := len(a), len(b)
	if la == 0 {
		return lb
	}
	if lb == 0 {
		return la
	}

	matrix := make([][]int, la+1)
	for i := range matrix {
		matrix[i] = make([]int, lb+1)
		matrix[i][0] = i
	}
	for j := 0; j <= lb; j++ {
		matrix[0][j] = j
	}

	for i := 1; i <= la; i++ {
		for j := 1; j <= lb; j++ {
			cost := 1
			if a[i-1] == b[j-1] {
				cost = 0
			}
			matrix[i][j] = min3(
				matrix[i-1][j]+1,
				matrix[i][j-1]+1,
				matrix[i-1][j-1]+cost,
			)
		}
	}
	return matrix[la][lb]
}

func min3(a, b, c int) int {
	if a < b {
		if a < c {
			return a
		}
		return c
	}
	if b < c {
		return b
	}
	return c
}
