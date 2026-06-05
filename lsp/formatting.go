package main

import (
	"encoding/json"
	"strings"
)

// --- Formatting ---

func (s *Server) handleFormatting(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	text, ok := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	if !ok || text == "" {
		sendResponse(msg.ID, nil)
		return
	}

	formatted := formatServSource(text)
	if formatted == text {
		sendResponse(msg.ID, []interface{}{}) // no changes
		return
	}

	// Return a single edit replacing the entire document
	lines := strings.Split(text, "\n")
	lastLine := len(lines) - 1
	lastChar := 0
	if lastLine >= 0 {
		lastChar = len(lines[lastLine])
	}

	edit := map[string]interface{}{
		"range": map[string]interface{}{
			"start": map[string]interface{}{"line": 0, "character": 0},
			"end":   map[string]interface{}{"line": lastLine, "character": lastChar},
		},
		"newText": formatted,
	}

	sendResponse(msg.ID, []interface{}{edit})
}

// formatServSource applies the same formatting logic as `serv fmt`.
func formatServSource(source string) string {
	lines := strings.Split(source, "\n")
	var result []string
	indentLevel := 0
	indent := "    " // 4 spaces
	prevEmpty := false
	prevWasBlock := false

	topLevelKeywords := map[string]bool{
		"server": true, "database": true, "cache": true, "broker": true,
		"route": true, "fn": true, "every": true, "cron": true,
		"subscribe": true, "test": true, "struct": true, "interface": true,
		"middleware": true, "ws": true, "enum": true, "validate": true,
		"type": true, "export": true, "beforeEach": true, "afterEach": true,
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

		netBraces := countNetBracesLSP(trimmed)

		if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") {
			indentLevel--
			if indentLevel < 0 {
				indentLevel = 0
			}
		}

		if indentLevel == 0 && i > 0 {
			words := strings.Fields(trimmed)
			if len(words) > 0 {
				firstWord := strings.TrimRight(words[0], "({[")
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
	return strings.Join(result, "\n") + "\n"
}

func countNetBracesLSP(line string) int {
	net := 0
	inString := false
	stringChar := byte(0)
	for i := 0; i < len(line); i++ {
		ch := line[i]
		if inString {
			if ch == '\\' && i+1 < len(line) {
				i++
				continue
			}
			if ch == stringChar {
				inString = false
			}
			continue
		}
		if ch == '/' && i+1 < len(line) && line[i+1] == '/' {
			break
		}
		if ch == '"' || ch == '\'' || ch == '`' {
			inString = true
			stringChar = ch
			continue
		}
		if ch == '{' {
			net++
		} else if ch == '}' {
			net--
		}
	}
	return net
}
