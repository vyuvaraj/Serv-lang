package main

import (
	"encoding/json"
	"fmt"
	"strings"

	"serv/compiler"
)

// --- Hover (works on usage, not just definition) ---

func (s *Server) handleHover(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	text := s.documents[params.TextDocument.URI]
	syms := s.symbols[params.TextDocument.URI]
	s.mu.RUnlock()

	// Get the word under the cursor
	word := getWordAtPosition(text, params.Position)
	if word == "" {
		sendResponse(msg.ID, nil)
		return
	}

	// Search for a symbol matching this word
	for _, sym := range syms {
		if sym.Name == word || strings.HasSuffix(sym.Name, "."+word) || (sym.Kind == "fn" && sym.Name == word) {
			content := formatSymbolHover(sym)
			sendResponse(msg.ID, Hover{
				Contents: MarkupContent{Kind: "markdown", Value: content},
			})
			return
		}
	}

	// Check built-in objects
	if hover := builtinHover(word); hover != "" {
		sendResponse(msg.ID, Hover{
			Contents: MarkupContent{Kind: "markdown", Value: hover},
		})
		return
	}

	sendResponse(msg.ID, nil)
}

func formatSymbolHover(sym symbolInfo) string {
	switch sym.Kind {
	case "struct":
		return fmt.Sprintf("```serv\nstruct %s { %s }\n```", sym.Name, sym.TypeInfo)
	case "fn":
		params := formatParamList(sym.Params, sym.ParamTypes)
		ret := sym.TypeInfo
		if ret == "" {
			ret = "void"
		}
		return fmt.Sprintf("```serv\nfn %s(%s) -> %s\n```", sym.Name, params, ret)
	case "method":
		params := formatParamList(sym.Params, sym.ParamTypes)
		ret := sym.TypeInfo
		if ret == "" {
			ret = "void"
		}
		return fmt.Sprintf("```serv\nfn %s(%s) -> %s\n```", sym.Name, params, ret)
	case "let":
		t := sym.TypeInfo
		if t == "" {
			t = "inferred"
		}
		return fmt.Sprintf("```serv\nlet %s: %s\n```", sym.Name, t)
	case "route":
		return fmt.Sprintf("```serv\nroute %s\n```", sym.Name)
	case "middleware":
		return fmt.Sprintf("```serv\nmiddleware %s(req)\n```", sym.Name)
	case "enum":
		return fmt.Sprintf("```serv\nenum %s { %s }\n```", sym.Name, sym.TypeInfo)
	case "interface":
		return fmt.Sprintf("```serv\ninterface %s\n```", sym.Name)
	default:
		return sym.Name
	}
}

func formatParamList(params, paramTypes []string) string {
	var parts []string
	for i, p := range params {
		if i < len(paramTypes) && paramTypes[i] != "" {
			parts = append(parts, p+": "+paramTypes[i])
		} else {
			parts = append(parts, p)
		}
	}
	return strings.Join(parts, ", ")
}

func builtinHover(word string) string {
	builtins := map[string]string{
		"log":      "Built-in structured logger\n\nMethods: `.info()`, `.warn()`, `.error()`, `.debug()`, `.with()`, `.fields()`",
		"db":       "Database client\n\nMethods: `.query()`, `.queryPage()`, `.findOne()`, `.count()`, `.upsert()`",
		"cache":    "Cache client (Redis or in-memory)\n\nMethods: `.set(key, value, ttl)`, `.get(key)`",
		"http":     "HTTP client\n\nMethods: `.get(url)`, `.post(url, body)`",
		"json":     "JSON utilities\n\nMethods: `.parse(str)`, `.stringify(obj)`",
		"time":     "Time utilities\n\nMethods: `.now()`, `.unix()`, `.sleep(ms)`",
		"metric":   "Metrics (exposed at /metrics)\n\nMethods: `.inc(name)`, `.gauge(name, value)`",
		"atomic":   "Atomic operations\n\nMethods: `.new()`, `.inc()`, `.dec()`, `.get()`, `.set()`, `.cas()`",
		"channel":  "Go channels\n\nMethods: `.new()`, `.send()`, `.receive()`, `.tryReceive()`, `.close()`",
		"registry": "Named function registry\n\nMethods: `.set()`, `.call()`, `.has()`, `.list()`",
		"env":      "```serv\nfn env(key: string) -> string\n```\nRead environment variable",
		"config":   "```serv\nfn config(key: string) -> string\n```\nRead from config.yml or env var",
		"validate": "```serv\nfn validate(body, schema) -> []string | nil\n```\nValidate request body against schema",
	}
	if hover, ok := builtins[word]; ok {
		return hover
	}
	return ""
}

// --- Go to Definition (searches all symbols by name) ---

func (s *Server) handleDefinition(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	text := s.documents[params.TextDocument.URI]
	syms := s.symbols[params.TextDocument.URI]
	s.mu.RUnlock()

	// Check if cursor is on an import path string (e.g. "handlers/notifier.srv")
	if loc := s.resolveImportAtPosition(params.TextDocument.URI, text, params.Position); loc != nil {
		sendResponse(msg.ID, loc)
		return
	}

	word := getWordAtPosition(text, params.Position)
	if word == "" {
		sendResponse(msg.ID, nil)
		return
	}

	// Search current document symbols
	for _, sym := range syms {
		if sym.Name == word || strings.HasSuffix(sym.Name, "."+word) {
			sendResponse(msg.ID, Location{
				URI: params.TextDocument.URI,
				Range: Range{
					Start: Position{Line: sym.Line, Character: sym.Col},
					End:   Position{Line: sym.Line, Character: sym.Col + len(word)},
				},
			})
			return
		}
	}

	// Search all open documents (cross-file)
	s.mu.RLock()
	for uri, docSyms := range s.symbols {
		if uri == params.TextDocument.URI {
			continue
		}
		for _, sym := range docSyms {
			if sym.Name == word || strings.HasSuffix(sym.Name, "."+word) {
				s.mu.RUnlock()
				sendResponse(msg.ID, Location{
					URI: uri,
					Range: Range{
						Start: Position{Line: sym.Line, Character: sym.Col},
						End:   Position{Line: sym.Line, Character: sym.Col + len(word)},
					},
				})
				return
			}
		}
	}
	s.mu.RUnlock()

	sendResponse(msg.ID, nil)
}

// --- Find References ---

func (s *Server) handleReferences(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	text := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	word := getWordAtPosition(text, params.Position)
	if word == "" {
		sendResponse(msg.ID, []interface{}{})
		return
	}

	var locations []Location

	// Search all open documents for references to this symbol
	s.mu.RLock()
	for uri, docText := range s.documents {
		lines := strings.Split(docText, "\n")
		for lineNum, line := range lines {
			// Find all occurrences of the word in this line
			idx := 0
			for {
				pos := strings.Index(line[idx:], word)
				if pos < 0 {
					break
				}
				col := idx + pos
				// Check it's a whole word (not part of a larger identifier)
				isBoundary := true
				if col > 0 && isWordChar(line[col-1]) {
					isBoundary = false
				}
				endCol := col + len(word)
				if endCol < len(line) && isWordChar(line[endCol]) {
					isBoundary = false
				}
				if isBoundary {
					locations = append(locations, Location{
						URI: uri,
						Range: Range{
							Start: Position{Line: lineNum, Character: col},
							End:   Position{Line: lineNum, Character: endCol},
						},
					})
				}
				idx = col + len(word)
			}
		}
	}
	s.mu.RUnlock()

	sendResponse(msg.ID, locations)
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
		kind := 13
		switch sym.Kind {
		case "fn":
			kind = 12
		case "struct":
			kind = 23
		case "method":
			kind = 6
		case "interface":
			kind = 11
		case "route":
			kind = 12
		case "middleware":
			kind = 12
		case "enum":
			kind = 10
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

// --- Signature Help ---

func (s *Server) handleSignatureHelp(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	text := s.documents[params.TextDocument.URI]
	syms := s.symbols[params.TextDocument.URI]
	s.mu.RUnlock()

	// Find the function name before the cursor (look backwards for identifier before '(')
	funcName, activeParam := findFunctionContext(text, params.Position)
	if funcName == "" {
		sendResponse(msg.ID, nil)
		return
	}

	// Find the function symbol
	for _, sym := range syms {
		if sym.Name == funcName && (sym.Kind == "fn" || sym.Kind == "method") && len(sym.Params) > 0 {
			sig := buildSignature(sym)
			sendResponse(msg.ID, SignatureHelp{
				Signatures:      []SignatureInformation{sig},
				ActiveSignature: 0,
				ActiveParameter: activeParam,
			})
			return
		}
	}

	sendResponse(msg.ID, nil)
}

func findFunctionContext(text string, pos Position) (string, int) {
	lines := strings.Split(text, "\n")
	if pos.Line >= len(lines) {
		return "", 0
	}

	line := lines[pos.Line]
	if pos.Character > len(line) {
		return "", 0
	}

	// Look backwards from cursor to find the opening '('
	prefix := line[:pos.Character]
	depth := 0
	activeParam := 0

	for i := len(prefix) - 1; i >= 0; i-- {
		ch := prefix[i]
		if ch == ')' {
			depth++
		} else if ch == '(' {
			if depth == 0 {
				// Found the opening paren — extract function name before it
				nameEnd := i
				nameStart := nameEnd - 1
				for nameStart >= 0 && isWordChar(prefix[nameStart]) {
					nameStart--
				}
				nameStart++
				if nameStart < nameEnd {
					return prefix[nameStart:nameEnd], activeParam
				}
				return "", 0
			}
			depth--
		} else if ch == ',' && depth == 0 {
			activeParam++
		}
	}

	return "", 0
}

func buildSignature(sym symbolInfo) SignatureInformation {
	params := formatParamList(sym.Params, sym.ParamTypes)
	label := fmt.Sprintf("fn %s(%s)", sym.Name, params)
	if sym.TypeInfo != "" {
		label += " -> " + sym.TypeInfo
	}

	var paramInfos []ParameterInformation
	for i, p := range sym.Params {
		paramLabel := p
		if i < len(sym.ParamTypes) && sym.ParamTypes[i] != "" {
			paramLabel = p + ": " + sym.ParamTypes[i]
		}
		paramInfos = append(paramInfos, ParameterInformation{Label: paramLabel})
	}

	return SignatureInformation{
		Label:      label,
		Parameters: paramInfos,
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

	// Keywords (including new ones)
	keywords := []string{
		"fn", "let", "return", "if", "else", "for", "in", "match",
		"struct", "interface", "middleware", "export", "import",
		"route", "every", "cron", "subscribe", "publish", "spawn",
		"server", "database", "broker", "cache", "try", "catch",
		"test", "assert", "enum", "await", "true", "false", "nil",
		"self", "declare", "module", "from", "extern", "migration", "tool",
		"ws", "use", "channel", "atomic", "break", "continue", "type",
	}
	for _, kw := range keywords {
		items = append(items, CompletionItem{Label: kw, Kind: 14})
	}

	// Built-in functions/objects
	builtins := []CompletionItem{
		{Label: "log.info", Kind: 3, Detail: "Log info message", InsertText: "log.info(\"$1\")"},
		{Label: "log.warn", Kind: 3, Detail: "Log warning message", InsertText: "log.warn(\"$1\")"},
		{Label: "log.error", Kind: 3, Detail: "Log error message", InsertText: "log.error(\"$1\")"},
		{Label: "log.debug", Kind: 3, Detail: "Log debug message", InsertText: "log.debug(\"$1\")"},
		{Label: "db.query", Kind: 3, Detail: "Execute database query", InsertText: "db.query(\"$1\")"},
		{Label: "db.findOne", Kind: 3, Detail: "Find single document", InsertText: "db.findOne(\"$1\", \"$2\")"},
		{Label: "cache.set", Kind: 3, Detail: "Set cache value with TTL", InsertText: "cache.set(\"$1\", $2, \"10m\")"},
		{Label: "cache.get", Kind: 3, Detail: "Get cache value", InsertText: "cache.get(\"$1\")"},
		{Label: "http.get", Kind: 3, Detail: "HTTP GET request", InsertText: "http.get(\"$1\")"},
		{Label: "http.post", Kind: 3, Detail: "HTTP POST request", InsertText: "http.post(\"$1\", $2)"},
		{Label: "json.parse", Kind: 3, Detail: "Parse JSON string", InsertText: "json.parse($1)"},
		{Label: "json.stringify", Kind: 3, Detail: "Stringify to JSON", InsertText: "json.stringify($1)"},
		{Label: "time.now", Kind: 3, Detail: "Current RFC3339 timestamp", InsertText: "time.now()"},
		{Label: "time.unix", Kind: 3, Detail: "Unix timestamp (seconds)", InsertText: "time.unix()"},
		{Label: "channel.new", Kind: 3, Detail: "Create buffered channel", InsertText: "channel.new(\"$1\", $2)"},
		{Label: "channel.send", Kind: 3, Detail: "Send to channel", InsertText: "channel.send(\"$1\", $2)"},
		{Label: "channel.receive", Kind: 3, Detail: "Receive from channel", InsertText: "channel.receive(\"$1\")"},
		{Label: "atomic.new", Kind: 3, Detail: "Create atomic value", InsertText: "atomic.new(\"$1\", $2)"},
		{Label: "atomic.inc", Kind: 3, Detail: "Increment counter", InsertText: "atomic.inc(\"$1\")"},
		{Label: "metric.inc", Kind: 3, Detail: "Increment counter metric", InsertText: "metric.inc(\"$1\")"},
		{Label: "metric.gauge", Kind: 3, Detail: "Set gauge value", InsertText: "metric.gauge(\"$1\", $2)"},
		{Label: "env", Kind: 3, Detail: "Read environment variable", InsertText: "env(\"$1\")"},
		{Label: "config", Kind: 3, Detail: "Read config value", InsertText: "config(\"$1\")"},
	}
	items = append(items, builtins...)

	// Symbols from current document
	s.mu.RLock()
	if syms, ok := s.symbols[params.TextDocument.URI]; ok {
		for _, sym := range syms {
			kind := 6
			switch sym.Kind {
			case "fn":
				kind = 3
			case "struct":
				kind = 22
			case "method":
				kind = 2
			case "interface":
				kind = 8
			case "enum":
				kind = 13
			}
			detail := sym.TypeInfo
			if sym.Kind == "fn" && len(sym.Params) > 0 {
				detail = "(" + formatParamList(sym.Params, sym.ParamTypes) + ")"
				if sym.TypeInfo != "" {
					detail += " -> " + sym.TypeInfo
				}
			}
			items = append(items, CompletionItem{
				Label:  sym.Name,
				Kind:   kind,
				Detail: detail,
			})
		}
	}
	s.mu.RUnlock()

	sendResponse(msg.ID, CompletionList{IsIncomplete: false, Items: items})
}

// --- extractSymbol ---

func extractSymbol(stmt compiler.Statement) symbolInfo {
	switch s := stmt.(type) {
	case *compiler.FnDecl:
		return symbolInfo{Name: s.Name, Kind: "fn", Line: s.Token.Line - 1, Col: s.Token.Col - 1, TypeInfo: s.ReturnType, Params: s.Params, ParamTypes: s.ParamTypes}
	case *compiler.StructDecl:
		var fields []string
		for _, f := range s.Fields {
			fields = append(fields, f.Name+": "+f.Type)
		}
		return symbolInfo{Name: s.Name, Kind: "struct", Line: s.Token.Line - 1, Col: s.Token.Col - 1, TypeInfo: strings.Join(fields, ", ")}
	case *compiler.MethodDecl:
		return symbolInfo{Name: s.TypeName + "." + s.Name, Kind: "method", Line: s.Token.Line - 1, Col: s.Token.Col - 1, TypeInfo: s.ReturnType, Params: s.Params, ParamTypes: s.ParamTypes}
	case *compiler.LetStmt:
		return symbolInfo{Name: s.Name, Kind: "let", Line: s.Token.Line - 1, Col: s.Token.Col - 1, TypeInfo: s.Type}
	case *compiler.RouteStmt:
		return symbolInfo{Name: s.Method + " " + s.Path, Kind: "route", Line: s.Token.Line - 1, Col: s.Token.Col - 1}
	case *compiler.MiddlewareDecl:
		return symbolInfo{Name: s.Name, Kind: "middleware", Line: s.Token.Line - 1, Col: s.Token.Col - 1, Params: []string{s.Param}}
	case *compiler.InterfaceDecl:
		return symbolInfo{Name: s.Name, Kind: "interface", Line: s.Token.Line - 1, Col: s.Token.Col - 1}
	case *compiler.WsStmt:
		return symbolInfo{Name: "ws " + s.Path, Kind: "route", Line: s.Token.Line - 1, Col: s.Token.Col - 1}
	case *compiler.EveryStmt:
		return symbolInfo{Name: "every " + s.Interval.String(), Kind: "fn", Line: s.Token.Line - 1, Col: s.Token.Col - 1}
	case *compiler.CronStmt:
		return symbolInfo{Name: "cron " + s.Cron.String(), Kind: "fn", Line: s.Token.Line - 1, Col: s.Token.Col - 1}
	case *compiler.SubscribeStmt:
		return symbolInfo{Name: "subscribe " + s.Topic.String(), Kind: "fn", Line: s.Token.Line - 1, Col: s.Token.Col - 1}
	case *compiler.EnumStmt:
		return symbolInfo{Name: s.Name, Kind: "enum", Line: s.Token.Line - 1, Col: s.Token.Col - 1, TypeInfo: strings.Join(s.Members, ", ")}
	case *compiler.ExportStmt:
		return extractSymbol(s.Inner)
	default:
		return symbolInfo{}
	}
}
