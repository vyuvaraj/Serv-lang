package main

import (
	"fmt"
	"os"
	"strings"
)

// formatFile formats a .srv file with consistent indentation and spacing.
func formatFile(srvFile string, checkOnly bool) {
	content, err := os.ReadFile(srvFile)
	if err != nil {
		fmt.Printf("Error reading file: %v\n", err)
		os.Exit(1)
	}

	lines := strings.Split(string(content), "\n")
	var result []string
	indentLevel := 0
	indent := "    " // 4 spaces
	prevEmpty := false
	prevWasBlock := false // track if previous non-empty line ended a block or was a top-level decl

	// Top-level keywords that should have a blank line before them (if not already)
	topLevelKeywords := map[string]bool{
		"server": true, "database": true, "cache": true, "broker": true,
		"route": true, "fn": true, "every": true, "cron": true,
		"subscribe": true, "test": true, "struct": true, "interface": true,
		"middleware": true, "ws": true, "enum": true, "validate": true,
		"type": true, "export": true,
	}

	for i, line := range lines {
		trimmed := strings.TrimSpace(line)

		// Collapse multiple consecutive empty lines into one
		if trimmed == "" {
			if !prevEmpty {
				result = append(result, "")
				prevEmpty = true
			}
			continue
		}
		prevEmpty = false

		// Count braces outside of strings to determine indent changes
		netBraces := countNetBraces(trimmed)

		// Decrease indent BEFORE writing if line starts with closing brace/bracket
		if strings.HasPrefix(trimmed, "}") || strings.HasPrefix(trimmed, "]") {
			indentLevel--
			if indentLevel < 0 {
				indentLevel = 0
			}
		}

		// Insert blank line before top-level keywords (at indent 0) if previous wasn't empty
		if indentLevel == 0 && i > 0 {
			firstWord := strings.Fields(trimmed)[0]
			// Strip trailing punctuation
			firstWord = strings.TrimRight(firstWord, "({[")
			if topLevelKeywords[firstWord] && !prevEmpty && len(result) > 0 && result[len(result)-1] != "" {
				if !prevWasBlock {
					result = append(result, "")
				}
			}
		}

		// Apply indentation
		formatted := strings.Repeat(indent, indentLevel) + trimmed
		result = append(result, formatted)

		// Track if this line is a closing brace (end of block)
		prevWasBlock = strings.HasPrefix(trimmed, "}")

		// Increase indent AFTER writing if line has net opening braces
		if netBraces > 0 {
			indentLevel += netBraces
		} else if netBraces < 0 && !strings.HasPrefix(trimmed, "}") && !strings.HasPrefix(trimmed, "]") {
			// Lines like `} else {` — net is 0, already handled
			indentLevel += netBraces
			if indentLevel < 0 {
				indentLevel = 0
			}
		}
	}

	// Remove trailing empty lines, then ensure single newline at end
	for len(result) > 0 && strings.TrimSpace(result[len(result)-1]) == "" {
		result = result[:len(result)-1]
	}
	output := strings.Join(result, "\n") + "\n"

	if checkOnly {
		// Normalize line endings for cross-platform comparison
		normalizedOutput := strings.ReplaceAll(output, "\r\n", "\n")
		normalizedContent := strings.ReplaceAll(string(content), "\r\n", "\n")
		if normalizedOutput != normalizedContent {
			fmt.Printf("%s: not formatted\n", srvFile)
			os.Exit(1)
		}
		return
	}

	if err := os.WriteFile(srvFile, []byte(output), 0644); err != nil {
		fmt.Printf("Error writing file: %v\n", err)
		os.Exit(1)
	}
	fmt.Printf("Formatted: %s\n", srvFile)
}

// countNetBraces counts opening minus closing braces/brackets in a line,
// ignoring braces inside string literals and comments.
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
		// Check for comment start
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
