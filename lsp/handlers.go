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
		"s3":       "S3 / ServStore Client\n\nMethods: `.init(endpoint, accessKey, secretKey)`, `.createBucket(bucket)`, `.deleteBucket(bucket)`, `.setBucketVersioning(bucket, enabled)`, `.put(bucket, key, body)`, `.get(bucket, key)`, `.delete(bucket, key)`, `.list(bucket, prefix)`, `.at(bucket, key, timestamp)`, `.search(bucket, query, maxResults)`",
		"env":      "```serv\nfn env(key: string) -> string\n```\nRead environment variable",
		"config":   "```serv\nfn config(key: string) -> string\n```\nRead from config.yml or env var",
		"validate": "```serv\nfn validate(body, schema) -> []string | nil\n```\nValidate request body against schema",
		"wasm":     "WebAssembly guest execution helpers\n\nMethods: `.readInput()`, `.writeOutput(data)`",
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

// --- Find References & Rename ---

func (s *Server) handleReferences(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	text := s.documents[params.TextDocument.URI]
	syms := s.symbols[params.TextDocument.URI]
	s.mu.RUnlock()

	word := getWordAtPosition(text, params.Position)
	if word == "" {
		sendResponse(msg.ID, []interface{}{})
		return
	}

	// Determine if this is a local variable
	isLocal := false
	var localStartLine, localEndLine int
	enclosingSym := findEnclosingSymbol(syms, params.Position.Line)
	if enclosingSym != nil {
		lines := strings.Split(text, "\n")
		localStartLine = enclosingSym.Line
		localEndLine = len(lines) - 1
		for _, sInfo := range syms {
			if sInfo.Line > localStartLine && sInfo.Line < localEndLine {
				localEndLine = sInfo.Line - 1
			}
		}
		if isLocalVariable(word, text, localStartLine, params.Position.Line) {
			isLocal = true
		}
	}

	var locations []Location

	s.mu.RLock()
	defer s.mu.RUnlock()

	if isLocal {
		docText := s.documents[params.TextDocument.URI]
		lines := strings.Split(docText, "\n")
		for lineNum := localStartLine; lineNum <= localEndLine && lineNum < len(lines); lineNum++ {
			line := lines[lineNum]
			locations = append(locations, findWordOccurrencesInLine(line, word, lineNum, params.TextDocument.URI)...)
		}
	} else {
		for uri, docText := range s.documents {
			lines := strings.Split(docText, "\n")
			for lineNum, line := range lines {
				locations = append(locations, findWordOccurrencesInLine(line, word, lineNum, uri)...)
			}
		}
	}

	sendResponse(msg.ID, locations)
}

func (s *Server) handlePrepareRename(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	text := s.documents[params.TextDocument.URI]
	s.mu.RUnlock()

	word := getWordAtPosition(text, params.Position)
	if word == "" || isBuiltinOrKeyword(word) {
		sendResponse(msg.ID, nil)
		return
	}

	lines := strings.Split(text, "\n")
	if params.Position.Line >= len(lines) {
		sendResponse(msg.ID, nil)
		return
	}
	line := lines[params.Position.Line]
	start := params.Position.Character
	for start > 0 && isWordChar(line[start-1]) {
		start--
	}
	end := params.Position.Character
	for end < len(line) && isWordChar(line[end]) {
		end++
	}

	sendResponse(msg.ID, Range{
		Start: Position{Line: params.Position.Line, Character: start},
		End:   Position{Line: params.Position.Line, Character: end},
	})
}

func (s *Server) handleRename(msg JSONRPCMessage) {
	var params struct {
		TextDocument TextDocumentIdentifier `json:"textDocument"`
		Position     Position               `json:"position"`
		NewName      string                 `json:"newName"`
	}
	json.Unmarshal(msg.Params, &params)

	s.mu.RLock()
	text := s.documents[params.TextDocument.URI]
	syms := s.symbols[params.TextDocument.URI]
	s.mu.RUnlock()

	word := getWordAtPosition(text, params.Position)
	if word == "" || isBuiltinOrKeyword(word) {
		sendResponse(msg.ID, nil)
		return
	}

	isLocal := false
	var localStartLine, localEndLine int
	enclosingSym := findEnclosingSymbol(syms, params.Position.Line)
	if enclosingSym != nil {
		lines := strings.Split(text, "\n")
		localStartLine = enclosingSym.Line
		localEndLine = len(lines) - 1
		for _, sInfo := range syms {
			if sInfo.Line > localStartLine && sInfo.Line < localEndLine {
				localEndLine = sInfo.Line - 1
			}
		}
		if isLocalVariable(word, text, localStartLine, params.Position.Line) {
			isLocal = true
		}
	}

	var locations []Location

	s.mu.RLock()
	if isLocal {
		docText := s.documents[params.TextDocument.URI]
		lines := strings.Split(docText, "\n")
		for lineNum := localStartLine; lineNum <= localEndLine && lineNum < len(lines); lineNum++ {
			line := lines[lineNum]
			locations = append(locations, findWordOccurrencesInLine(line, word, lineNum, params.TextDocument.URI)...)
		}
	} else {
		for uri, docText := range s.documents {
			lines := strings.Split(docText, "\n")
			for lineNum, line := range lines {
				locations = append(locations, findWordOccurrencesInLine(line, word, lineNum, uri)...)
			}
		}
	}
	s.mu.RUnlock()

	changes := make(map[string][]TextEdit)
	for _, loc := range locations {
		changes[loc.URI] = append(changes[loc.URI], TextEdit{
			Range:   loc.Range,
			NewText: params.NewName,
		})
	}

	sendResponse(msg.ID, WorkspaceEdit{Changes: changes})
}

// --- Helper Functions for Reference Resolution and Scope Tracking ---

func findEnclosingSymbol(symbols []symbolInfo, line int) *symbolInfo {
	var best *symbolInfo
	for i := range symbols {
		sym := &symbols[i]
		if sym.Line <= line {
			if best == nil || sym.Line > best.Line {
				best = sym
			}
		}
	}
	return best
}

func isLocalVariable(word string, text string, startLine, cursorLine int) bool {
	lines := strings.Split(text, "\n")
	for i := startLine; i <= cursorLine && i < len(lines); i++ {
		line := lines[i]
		if strings.Contains(line, "let ") && strings.Contains(line, word) {
			if isVarDeclaredInLet(line, word) {
				return true
			}
		}
		if strings.Contains(line, "for ") && strings.Contains(line, " in ") {
			if isVarDeclaredInFor(line, word) {
				return true
			}
		}
		if (strings.Contains(line, "fn ") || strings.Contains(line, "route ") || strings.Contains(line, "subscribe ") || strings.Contains(line, "ws ")) && (strings.Contains(line, "("+word) || strings.Contains(line, ", "+word)) {
			return true
		}
	}
	return false
}

func isVarDeclaredInLet(line, word string) bool {
	parts := strings.SplitN(line, "=", 2)
	if len(parts) < 1 {
		return false
	}
	left := strings.TrimPrefix(strings.TrimSpace(parts[0]), "let ")
	vars := strings.Split(left, ",")
	for _, v := range vars {
		v = strings.TrimSpace(v)
		if strings.Contains(v, ":") {
			vParts := strings.SplitN(v, ":", 2)
			v = strings.TrimSpace(vParts[0])
		}
		if v == word {
			return true
		}
	}
	return false
}

func isVarDeclaredInFor(line, word string) bool {
	parts := strings.SplitN(line, " in ", 2)
	if len(parts) < 1 {
		return false
	}
	left := strings.TrimPrefix(strings.TrimSpace(parts[0]), "for ")
	vars := strings.Split(left, ",")
	for _, v := range vars {
		if strings.TrimSpace(v) == word {
			return true
		}
	}
	return false
}

func findWordOccurrencesInLine(line, word string, lineNum int, uri string) []Location {
	var locations []Location
	idx := 0
	for {
		pos := strings.Index(line[idx:], word)
		if pos < 0 {
			break
		}
		col := idx + pos
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
	return locations
}

func isBuiltinOrKeyword(word string) bool {
	keywords := map[string]bool{
		"fn": true, "let": true, "return": true, "if": true, "else": true,
		"for": true, "in": true, "match": true, "struct": true, "interface": true,
		"middleware": true, "export": true, "import": true, "route": true,
		"every": true, "cron": true, "subscribe": true, "publish": true,
		"spawn": true, "server": true, "database": true, "broker": true,
		"cache": true, "try": true, "catch": true, "test": true, "assert": true,
		"enum": true, "await": true, "true": true, "false": true, "nil": true,
		"self": true, "declare": true, "module": true, "from": true, "extern": true,
		"migration": true, "tool": true, "ws": true, "use": true, "channel": true,
		"atomic": true, "break": true, "continue": true, "type": true,
	}
	builtins := map[string]bool{
		"log": true, "db": true, "cache": true, "http": true, "json": true,
		"time": true, "metric": true, "atomic": true, "channel": true,
		"registry": true, "env": true, "config": true, "validate": true,
		"s3": true, "wasm": true,
	}
	return keywords[word] || builtins[word]
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
		{Label: "s3.init", Kind: 3, Detail: "Initialize S3 client", InsertText: "s3.init(\"$1\", \"$2\", \"$3\")"},
		{Label: "s3.createBucket", Kind: 3, Detail: "Create S3 Bucket", InsertText: "s3.createBucket(\"$1\")"},
		{Label: "s3.deleteBucket", Kind: 3, Detail: "Delete S3 Bucket", InsertText: "s3.deleteBucket(\"$1\")"},
		{Label: "s3.setBucketVersioning", Kind: 3, Detail: "Set S3 Bucket Versioning", InsertText: "s3.setBucketVersioning(\"$1\", $2)"},
		{Label: "s3.put", Kind: 3, Detail: "Upload object to S3", InsertText: "s3.put(\"$1\", \"$2\", $3)"},
		{Label: "s3.get", Kind: 3, Detail: "Retrieve object from S3", InsertText: "s3.get(\"$1\", \"$2\")"},
		{Label: "s3.delete", Kind: 3, Detail: "Delete S3 object", InsertText: "s3.delete(\"$1\", \"$2\")"},
		{Label: "s3.list", Kind: 3, Detail: "List S3 objects", InsertText: "s3.list(\"$1\", \"$2\")"},
		{Label: "s3.at", Kind: 3, Detail: "Time-travel S3 object version", InsertText: "s3.at(\"$1\", \"$2\", \"$3\")"},
		{Label: "s3.search", Kind: 3, Detail: "Semantic search S3 query", InsertText: "s3.search(\"$1\", \"$2\", $3)"},
		{Label: "wasm.readInput", Kind: 3, Detail: "Read input from stdin (WASM context)", InsertText: "wasm.readInput()"},
		{Label: "wasm.writeOutput", Kind: 3, Detail: "Write output to stdout (WASM context)", InsertText: "wasm.writeOutput($1)"},
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
