// Serv Language Server Protocol (LSP) implementation.
// Provides real-time diagnostics, autocomplete, hover, go-to-definition,
// signature help, and document symbols for .srv files.
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
	Name           string           `json:"name"`
	Kind           int              `json:"kind"`
	Range          Range            `json:"range"`
	SelectionRange Range            `json:"selectionRange"`
	Children       []DocumentSymbol `json:"children,omitempty"`
}

type SignatureHelp struct {
	Signatures      []SignatureInformation `json:"signatures"`
	ActiveSignature int                    `json:"activeSignature"`
	ActiveParameter int                    `json:"activeParameter"`
}

type SignatureInformation struct {
	Label         string                 `json:"label"`
	Documentation string                 `json:"documentation,omitempty"`
	Parameters    []ParameterInformation `json:"parameters,omitempty"`
}

type ParameterInformation struct {
	Label string `json:"label"`
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
	documents map[string]string       // uri -> content
	symbols   map[string][]symbolInfo // uri -> symbols
	mu        sync.RWMutex
}

type symbolInfo struct {
	Name       string
	Kind       string // "struct", "fn", "method", "let", "route", "middleware", "enum", "interface"
	Line       int
	Col        int
	TypeInfo   string   // return type or struct fields summary
	Params     []string // function parameter names
	ParamTypes []string // function parameter types
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

		reader.ReadString('\n')

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
		// No-op
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
	case "textDocument/references":
		s.handleReferences(msg)
	case "textDocument/documentSymbol":
		s.handleDocumentSymbol(msg)
	case "textDocument/signatureHelp":
		s.handleSignatureHelp(msg)
	case "textDocument/formatting":
		s.handleFormatting(msg)
	case "textDocument/codeAction":
		s.handleCodeAction(msg)
	}
}

func (s *Server) handleInitialize(msg JSONRPCMessage) {
	result := map[string]interface{}{
		"capabilities": map[string]interface{}{
			"textDocumentSync": 1, // Full sync
			"completionProvider": map[string]interface{}{
				"triggerCharacters": []string{".", "\""},
			},
			"hoverProvider":              true,
			"definitionProvider":         true,
			"referencesProvider":         true,
			"documentSymbolProvider":     true,
			"documentFormattingProvider": true,
			"codeActionProvider":         true,
			"signatureHelpProvider": map[string]interface{}{
				"triggerCharacters":   []string{"(", ","},
				"retriggerCharacters": []string{","},
			},
		},
		"serverInfo": map[string]interface{}{
			"name":    "serv-lsp",
			"version": "3.0.0",
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

	sendNotification("textDocument/publishDiagnostics", map[string]interface{}{
		"uri":         params.TextDocument.URI,
		"diagnostics": []Diagnostic{},
	})
}

// --- JSON-RPC Helpers ---

func sendResponse(id interface{}, result interface{}) {
	msg := map[string]interface{}{
		"jsonrpc": "2.0",
		"id":      id,
		"result":  result,
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
