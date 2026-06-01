// Serv Language Server Protocol (LSP) implementation.
// Provides real-time diagnostics, autocomplete, hover, and go-to-definition for .srv files.
//
// Usage: serv-lsp (communicates via stdin/stdout JSON-RPC)
package main

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"strconv"
	"strings"
	"sync"

	"serv/compiler"
)

// --- LSP Protocol Types ---

type Position struct {
	Line      int `json:"line"`
	Character int `json:"character"`
}

type Range struct {
	Start Position `json:"start"`
	End   Position `json:"end"`
}

type Location struct {
	URI   string `json:"uri"`
	Range Range  `json:"range"`
}

type Diagnostic struct {
	Range    Range  `json:"range"`
	Severity int    `json:"severity"` // 1=Error, 2=Warning, 3=Info, 4=Hint
	Message  string `json:"message"`
	Source   string `json:"source"`
}

type TextDocumentIdentifier struct {
	URI string `json:"uri"`
}

type TextDocumentItem struct {
	URI        string `json:"uri"`
	LanguageID string `json:"languageId"`
	Version    int    `json:"version"`
	Text       string `json:"text"`
}

type VersionedTextDocumentIdentifier struct {
	URI     string `json:"uri"`
	Version int    `json:"version"`
}

type TextDocumentContentChangeEvent struct {
	Text string `json:"text"`
}

type CompletionItem struct {
	Label         string `json:"label"`
	Kind          int    `json:"kind"` // 1=Text, 2=Method, 3=Function, 6=Variable, 7=Class, 8=Interface, 14=Keyword, 22=Struct
	Detail        string `json:"detail,omitempty"`
	Documentation string `json:"documentation,omitempty"`
	InsertText    string `json:"insertText,omitempty"`
}

type CompletionList struct {
	IsIncomplete bool             `json:"isIncomplete"`
	Items        []CompletionItem `json:"items"`
}

type Hover struct {
	Contents MarkupContent `json:"contents"`
	Range    *Range        `json:"range,omitempty"`
}

type MarkupContent struct {
	Kind  string `json:"kind"` // "plaintext" or "markdown"
	Value string `json:"value"`
}

type DocumentSymbol struct {
	Name           string          `json:"name"`
	Kind           int             `json:"kind"` // 5=Class, 6=Method, 12=Function, 13=Variable, 23=Struct
	Range          Range           `json:"range"`
	SelectionRange Range           `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

// --- JSON-RPC Types ---

type JSONRPCMessage struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method,omitempty"`
	Params  json.RawMessage `json:"params,omitempty"`
	Result  interface{}     `json:"result,omitempty"`
	Error   interface{}     `json:"error,omitempty"`
}

// --- Server State ---

type Server struct {
	documents map[string]string // uri -> content
	symbols   map[string][]symbolInfo // uri -> symbols
	mu        sync.RWMutex
}

type symbolInfo struct {
	Name     string
	Kind     string // "struct", "fn", "method", "let", "route", "middleware"
	Line     int
	TypeInfo string // return type or struct fields summary
}

func NewServer() *Server {
	return &Server{
		documents: make(map[string]string),
		symbols:   make(map[string][]symbolInfo),
	}
}

// --- Main Loop ---

func main() {
	server := NewServer()
	reader := bufio.NewReader(os.Stdin)

	for {
		// Read Content-Length header
		header, err := reader.ReadString('\n')
		if err != nil {
			break
		}
		header = strings.TrimSpace(header)
		if !strings.HasPrefix(header, "Content-Length:") {
			continue
		}
		lengthStr := strings.TrimSpace(strings.TrimPrefix(header, "Content-Length:"))
		length, err := strconv.Atoi(lengthStr)
		if err != nil {
			continue
		}

		// Read empty line separator
		reader.ReadString('\n')

		// Read body
		body := make([]byte, length)
		_, err = reader.Read(body)
		if err != nil {
			break
		}

		var msg JSONRPCMessage
		if err := json.Unmarshal(body, &msg); err != nil {
			continue
		}

		server.handleMessage(msg)
	}
}

func (s *Server) handleMessage(msg JSONRPCMessage) {
	switch msg.Method {
	case "initialize":
		s.handleInitialize(msg)
	case "initialized":
		// No-op notification
	case "shutdown":
		sendResponse(msg.ID, nil)
	case "exit":
		os.Exit(0)
	case "textDocument/didOpen":
		s.handleDidOpen(msg)
	case "textDocument/didChange":
		s.handleDidChange(msg)
	case "textDocument/didClose":
		s.handleDidClose(msg)
	case "textDocument/completion":
		s.handleCompletion(msg)
	case "textDocument/hover":
		s.handleHover(msg)
	case "textDocument/definition":
		s.handleDefinition(msg)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(msg)
	}
}

func (s *Server) handleInitialize(msg JSONRPCMessage) {
	result := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"textDocumentSync": 1, // Full sync
			"completionProvider": map[string]interface{}{
				"triggerCharacters": []string{".", "\""},
			},
			"hoverProvider":          true,
			"definitionProvider":     true,
			"documentSymbolProvider": true,
		},
		"serverInfo": map[string]interface{}{
			"name":    "serv-lsp",
			"version": "1.0.0",
		},
	}
	sendResponse(msg.ID, result)
}

func (s *Server) handleDidOpen(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentItem `json:"textDocument"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.Lock()
	s.documents[params.TextDocument.URI] = params.TextDocument.Text
	s.mu.Unlock()

	s.analyzeAndPublishDiagnostics(params.TextDocument.URI, params.TextDocument.Text)
}

func (s *Server) handleDidChange(msg JSONRPCMessage) {
	var params struct {
		TextDocument   VersionedTextDocumentIdentifier  `json:"textDocument"`
		ContentChanges []TextDocumentContentChangeEvent `json:"contentChanges"`
	}
	json.Unmarshal(msg.Params, &params)

	if len(params.ContentChanges) > 0 {
		text := params.ContentChanges[len(params.ContentChanges)-1].Text
		s.mu.Lock()
		s.documents[params.TextDocument.URI] = text
		s.mu.Unlock()

		s.analyzeAndPublishDiagnostics(params.TextDocument.URI, text)
	}
}

func (s *Server) handleDidClose(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.Lock()
	delete(s.documents, params.TextDocument.URI)
	delete(s.symbols, params.TextDocument.URI)
	s.mu.Unlock()

	// Clear diagnostics
	sendNotification("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         params.TextDocument.URI,
		"diagnostics": []Diagnostic{},
	})
}

// --- Diagnostics ---

func (s *Server) analyzeAndPublishDiagnostics(uri, text string) {
	diagnostics := []Diagnostic{}
	symbols := []symbolInfo{}

	// Parse the file
	lexer := compiler.NewLexer(text)
	parser := compiler.NewParser(lexer)
	program := parser.ParseProgram()

	// Collect parse errors as diagnostics
	for _, errMsg := range parser.Errors() {
		line, col := extractLineCol(errMsg)
		diagnostics = append(diagnostics, Diagnostic{
			Range: Range{
				Start: Position{Line: line, Character: col},
				End:   Position{Line: line, Character: col + 10},
			},
			Severity: 1, // Error
			Message:  errMsg,
			Source:   "serv",
		})
	}

	// Collect symbols from the AST
	for _, stmt := range program.Statements {
		sym := extractSymbol(stmt)
		if sym.Name != "" {
			symbols = append(symbols, sym)
		}
	}

	s.mu.Lock()
	s.symbols[uri] = symbols
	s.mu.Unlock()

	// Publish diagnostics
	sendNotification("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         uri,
		"diagnostics": diagnostics,
	})
}

func extractLineCol(errMsg string) (int, int) {
	// Format: [Line N, Col M] message
	line, col := 0, 0
	if strings.HasPrefix(errMsg, "[Line ") {
		parts := strings.SplitN(errMsg, "]", 2)
		if len(parts) > 0 {
			inner := strings.TrimPrefix(parts[0], "[Line ")
			coords := strings.Split(inner, ", Col ")
			if len(coords) == 2 {
				l, _ := strconv.Atoi(coords[0])
				c, _ := strconv.Atoi(coords[1])
				line = l - 1 // LSP is 0-indexed
				col = c - 1
			}
		}
	}
	return line, col
}

func extractSymbol(stmt compiler.Statement) symbolInfo {
	switch s := stmt.(type) {
	case *compiler.FnDecl:
		return symbolInfo{Name: s.Name, Kind: "fn", Line: s.Token.Line - 1, TypeInfo: s.ReturnType}
	case *compiler.StructDecl:
		var fields []string
		for _, f := range s.Fields {
			fields = append(fields, f.Name+": "+f.Type)
		}
		return symbolInfo{Name: s.Name, Kind: "struct", Line: s.Token.Line - 1, TypeInfo: strings.Join(fields, ", ")}
	case *compiler.MethodDecl:
		return symbolInfo{Name: s.TypeName + "." + s.Name, Kind: "method", Line: s.Token.Line - 1, TypeInfo: s.ReturnType}
	case *compiler.LetStmt:
		return symbolInfo{Name: s.Name, Kind: "let", Line: s.Token.Line - 1, TypeInfo: s.Type}
	case *compiler.RouteStmt:
		return symbolInfo{Name: s.Method + " " + s.Path, Kind: "route", Line: s.Token.Line - 1}
	case *compiler.MiddlewareDecl:
		return symbolInfo{Name: s.Name, Kind: "middleware", Line: s.Token.Line - 1}
	case *compiler.InterfaceDecl:
		return symbolInfo{Name: s.Name, Kind: "interface", Line: s.Token.Line - 1}
	case *compiler.ExportStmt:
		return extractSymbol(s.Inner)
	default:
		return symbolInfo{}
	}
}

// --- Autocomplete ---

func (s *Server) handleCompletion(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	items := []CompletionItem{}

	// Keywords
	keywords := []string{
		"fn", "let", "return", "if", "else", "for", "in", "match",
		"struct", "interface", "middleware", "export", "import",
		"route", "every", "cron", "subscribe", "publish", "spawn",
		"server", "database", "broker", "cache", "try", "catch",
		"test", "assert", "enum", "await", "true", "false", "nil",
		"self", "declare", "module", "from", "extern", "migration", "tool",
	}
	for _, kw := range keywords {
		items = append(items, CompletionItem{
			Label: kw,
			Kind:  14, // Keyword
		})
	}

	// Built-in functions/objects
	builtins := []CompletionItem{
		{Label: "log.info", Kind: 3, Detail: "Log info message", InsertText: "log.info(\"$1\")"},
		{Label: "log.warn", Kind: 3, Detail: "Log warning message", InsertText: "log.warn(\"$1\")"},
		{Label: "log.error", Kind: 3, Detail: "Log error message", InsertText: "log.error(\"$1\")"},
		{Label: "db.query", Kind: 3, Detail: "Execute database query", InsertText: "db.query(\"$1\")"},
		{Label: "cache.set", Kind: 3, Detail: "Set cache value", InsertText: "cache.set(\"$1\", $2, \"10m\")"},
		{Label: "cache.get", Kind: 3, Detail: "Get cache value", InsertText: "cache.get(\"$1\")"},
		{Label: "http.get", Kind: 3, Detail: "HTTP GET request", InsertText: "http.get(\"$1\")"},
		{Label: "http.post", Kind: 3, Detail: "HTTP POST request", InsertText: "http.post(\"$1\", $2)"},
		{Label: "json.parse", Kind: 3, Detail: "Parse JSON string", InsertText: "json.parse($1)"},
		{Label: "json.stringify", Kind: 3, Detail: "Stringify to JSON", InsertText: "json.stringify($1)"},
		{Label: "time.now", Kind: 3, Detail: "Current timestamp", InsertText: "time.now()"},
		{Label: "env", Kind: 3, Detail: "Read environment variable", InsertText: "env(\"$1\")"},
		{Label: "config", Kind: 3, Detail: "Read config value", InsertText: "config(\"$1\")"},
	}
	items = append(items, builtins...)

	// Symbols from the current document
	s.mu.RLock()
	if syms, ok := s.symbols[params.TextDocument.URI]; ok {
		for _, sym := range syms {
			kind := 6 // Variable
			switch sym.Kind {
			case "fn":
				kind = 3 // Function
			case "struct":
				kind = 22 // Struct
			case "method":
				kind = 2 // Method
			case "interface":
				kind = 8 // Interface
			}
			items = append(items, CompletionItem{
				Label:  sym.Name,
				Kind:   kind,
				Detail: sym.TypeInfo,
			})
		}
	}
	s.mu.RUnlock()

	sendResponse(msg.ID, CompletionList{IsIncomplete: false, Items: items})
}

// --- Hover ---

func (s *Server) handleHover(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	syms := s.symbols[params.TextDocument.URI]
	s.mu.RUnlock()

	// Find symbol at position
	for _, sym := range syms {
		if sym.Line == params.Position.Line {
			var content string
			switch sym.Kind {
			case "struct":
				content = fmt.Sprintf("```serv\nstruct %s { %s }\n```", sym.Name, sym.TypeInfo)
			case "fn":
				ret := sym.TypeInfo
				if ret == "" {
					ret = "interface{}"
				}
				content = fmt.Sprintf("```serv\nfn %s() -> %s\n```", sym.Name, ret)
			case "method":
				content = fmt.Sprintf("```serv\nfn %s() -> %s\n```", sym.Name, sym.TypeInfo)
			case "let":
				content = fmt.Sprintf("```serv\nlet %s: %s\n```", sym.Name, sym.TypeInfo)
			case "route":
				content = fmt.Sprintf("```serv\nroute %s\n```", sym.Name)
			case "middleware":
				content = fmt.Sprintf("```serv\nmiddleware %s(req)\n```", sym.Name)
			default:
				content = sym.Name
			}

			sendResponse(msg.ID, Hover{
				Contents: MarkupContent{Kind: "markdown", Value: content},
			})
			return
		}
	}

	sendResponse(msg.ID, nil)
}

// --- Go to Definition ---

func (s *Server) handleDefinition(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	// Get the word at position
	s.mu.RLock()
	text := s.documents[params.TextDocument.URI]
	syms := s.symbols[params.TextDocument.URI]
	s.mu.RUnlock()

	word := getWordAtPosition(text, params.Position)
	if word == "" {
		sendResponse(msg.ID, nil)
		return
	}

	// Find matching symbol
	for _, sym := range syms {
		if sym.Name == word || strings.HasSuffix(sym.Name, "."+word) {
			sendResponse(msg.ID, Location{
				URI: params.TextDocument.URI,
				Range: Range{
					Start: Position{Line: sym.Line, Character: 0},
					End:   Position{Line: sym.Line, Character: len(sym.Name)},
				},
			})
			return
		}
	}

	sendResponse(msg.ID, nil)
}

func getWordAtPosition(text string, pos Position) string {
	lines := strings.Split(text, "\n")
	if pos.Line >= len(lines) {
		return ""
	}
	line := lines[pos.Line]
	if pos.Character >= len(line) {
		return ""
	}

	// Find word boundaries
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

// --- Document Symbols ---

func (s *Server) handleDocumentSymbol(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	syms := s.symbols[params.TextDocument.URI]
	s.mu.RUnlock()

	result := []DocumentSymbol{}
	for _, sym := range syms {
		kind := 13 // Variable
		switch sym.Kind {
		case "fn":
			kind = 12 // Function
		case "struct":
			kind = 23 // Struct
		case "method":
			kind = 6 // Method
		case "interface":
			kind = 11 // Interface
		case "route":
			kind = 12 // Function
		case "middleware":
			kind = 12 // Function
		}

		r := Range{
			Start: Position{Line: sym.Line, Character: 0},
			End:   Position{Line: sym.Line, Character: len(sym.Name) + 10},
		}
		result = append(result, DocumentSymbol{
			Name:           sym.Name,
			Kind:           kind,
			Range:          r,
			SelectionRange: r,
		})
	}

	sendResponse(msg.ID, result)
}

// --- JSON-RPC Helpers ---

func sendResponse(id interface{}, result interface{}) {
	msg := JSONRPCMessage{
		JSONRPC: "2.0",
		ID:      id,
		Result:  result,
	}
	data, _ := json.Marshal(msg)
	content := string(data)
	fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(content), content)
}

func sendNotification(method string, params interface{}) {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  method,
		"params":  params,
	}
	data, _ := json.Marshal(msg)
	content := string(data)
	fmt.Fprintf(os.Stdout, "Content-Length: %d\r\n\r\n%s", len(content), content)
}
