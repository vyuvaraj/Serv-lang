package main

import (
	"os"
	"path/filepath"
	"strings"
)

// --- Helpers ---

func getWordAtPosition(text string, pos Position) string {
	lines := strings.Split(text, "\n")
	if pos.Line >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	if pos.Character >= len(line) {
		return ""
	}

	start := pos.Character
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	end := pos.Character
	for end < len(line) && isWordChar(line[end]) {
		end++
	}

	if start == end {
		return ""
	}
	return line[start:end]
}

func isWordChar(ch byte) bool {
	return (ch >= 'a' && ch <= 'z') || (ch >= 'A' && ch <= 'Z') || (ch >= '0' && ch <= '9') || ch == '_'
}

// --- Import Path Resolution ---

func (s *Server) resolveImportAtPosition(uri, text string, pos Position) *Location {
	lines := strings.Split(text, "\n")
	if pos.Line >= len(lines) {
		return nil
	}
	line := lines[pos.Line]

	// Check if this line is an import statement with a string path
	trimmed := strings.TrimSpace(line)
	if !strings.HasPrefix(trimmed, "import") {
		return nil
	}

	// Find the quoted path on this line
	firstQuote := strings.Index(line, "\"")
	if firstQuote < 0 {
		return nil
	}
	lastQuote := strings.LastIndex(line, "\"")
	if lastQuote <= firstQuote {
		return nil
	}

	// Check if cursor is within or near the quoted string
	if pos.Character < firstQuote || pos.Character > lastQuote {
		return nil
	}

	importPath := line[firstQuote+1 : lastQuote]

	// Resolve the path relative to the current file
	currentFilePath := uriToFilePath(uri)
	if currentFilePath == "" {
		return nil
	}

	// Add .srv extension if missing
	if !strings.HasSuffix(importPath, ".srv") && !strings.HasSuffix(importPath, ".srv.d") {
		importPath = importPath + ".srv"
	}

	// Try resolving relative to current file
	resolved := filepath.Join(filepath.Dir(currentFilePath), importPath)
	if _, err := os.Stat(resolved); err == nil {
		return &Location{
			URI: filePathToURI(resolved),
			Range: Range{
				Start: Position{Line: 0, Character: 0},
				End:   Position{Line: 0, Character: 0},
			},
		}
	}

	// Try stdlib resolution (relative to serv binary)
	if strings.HasPrefix(importPath, "stdlib/") {
		if exePath, err := os.Executable(); err == nil {
			candidate := filepath.Join(filepath.Dir(exePath), importPath)
			if _, err := os.Stat(candidate); err == nil {
				return &Location{
					URI: filePathToURI(candidate),
					Range: Range{
						Start: Position{Line: 0, Character: 0},
						End:   Position{Line: 0, Character: 0},
					},
				}
			}
		}
	}

	return nil
}

func uriToFilePath(uri string) string {
	// file:///C:/path/to/file.srv -> C:/path/to/file.srv
	path := strings.TrimPrefix(uri, "file:///")
	path = strings.TrimPrefix(path, "file://")
	// Handle URL encoding
	path = strings.ReplaceAll(path, "%20", " ")
	path = strings.ReplaceAll(path, "%3A", ":")
	// On Windows, paths start with drive letter
	if len(path) > 2 && path[1] == ':' {
		return path
	}
	// On Unix, add leading /
	if !strings.HasPrefix(path, "/") {
		return "/" + path
	}
	return path
}

func filePathToURI(path string) string {
	abs, _ := filepath.Abs(path)
	abs = filepath.ToSlash(abs)
	// Windows: C:/path -> file:///C:/path
	if len(abs) > 1 && abs[1] == ':' {
		return "file:///" + abs
	}
	return "file://" + abs
}
