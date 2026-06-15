package dap

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

// ── DAP Protocol Types ────────────────────────────────────────────────────────

// Message is the base DAP protocol envelope.
// https://microsoft.github.io/debug-adapter-protocol/specification#Base_Protocol_ProtocolMessage
type Message struct {
	Seq  int    `json:"seq"`
	Type string `json:"type"` // "request" | "response" | "event"
}

// Request is a DAP request from the client (VS Code).
type Request struct {
	Message
	Command   string          `json:"command"`
	Arguments json.RawMessage `json:"arguments,omitempty"`
}

// Response is a DAP response from the debug adapter (us) or dlv.
type Response struct {
	Message
	RequestSeq int             `json:"request_seq"`
	Success    bool            `json:"success"`
	Command    string          `json:"command"`
	Message_   string          `json:"message,omitempty"`
	Body       json.RawMessage `json:"body,omitempty"`
}

// Event is a DAP event emitted by the debug adapter.
type Event struct {
	Message
	Event string          `json:"event"`
	Body  json.RawMessage `json:"body,omitempty"`
}

// SetBreakpointsArguments mirrors the DAP SetBreakpointsArguments shape.
type SetBreakpointsArguments struct {
	Source      SourceSpec        `json:"source"`
	Breakpoints []BreakpointSpec  `json:"breakpoints,omitempty"`
	Lines       []int             `json:"lines,omitempty"`
}

// SourceSpec identifies a source file in DAP messages.
type SourceSpec struct {
	Name string `json:"name,omitempty"`
	Path string `json:"path,omitempty"`
}

// BreakpointSpec is a single breakpoint location.
type BreakpointSpec struct {
	Line   int `json:"line"`
	Column int `json:"column,omitempty"`
}

// StackTraceArguments mirrors the DAP StackTraceArguments shape.
type StackTraceArguments struct {
	ThreadID int `json:"threadId"`
}

// StackTraceBody is the response body for a stackTrace response.
type StackTraceBody struct {
	StackFrames []StackFrame `json:"stackFrames"`
	TotalFrames int          `json:"totalFrames,omitempty"`
}

// StackFrame is a single call-stack frame.
type StackFrame struct {
	ID     int        `json:"id"`
	Name   string     `json:"name"`
	Source *SourceSpec `json:"source,omitempty"`
	Line   int        `json:"line"`
	Column int        `json:"column"`
}

// ── Proxy ─────────────────────────────────────────────────────────────────────

// Proxy is a bidirectional DAP message pump that sits between the DAP client
// (VS Code, on stdio) and Delve (on a TCP connection). It intercepts
// setBreakpoints requests and stackTrace responses, translating .srv
// coordinates ↔ Go coordinates using the provided SourceMap.
type Proxy struct {
	sm      *SourceMap
	srvFile string // absolute path to the .srv source file
	goFile  string // absolute path to the generated main.go
	dlvAddr string // "localhost:<port>"
}

// NewProxy creates a new DAP proxy.
func NewProxy(sm *SourceMap, dlvAddr string) *Proxy {
	return &Proxy{
		sm:      sm,
		srvFile: sm.SrvFile(),
		goFile:  sm.GoFile(),
		dlvAddr: dlvAddr,
	}
}

// Run starts the DAP proxy, reading from in (stdin) and writing to out
// (stdout) for the client side, while maintaining a connection to dlv.
// It blocks until either side closes.
func (p *Proxy) Run(in io.Reader, out io.Writer) error {
	// Connect to dlv DAP server.
	conn, err := net.Dial("tcp", p.dlvAddr)
	if err != nil {
		return fmt.Errorf("dap proxy: connect to dlv at %s: %w", p.dlvAddr, err)
	}
	defer conn.Close()

	errCh := make(chan error, 2)

	// Client → dlv: translate outgoing requests.
	go func() {
		errCh <- p.pumpClientToDlv(in, conn)
	}()

	// dlv → client: translate incoming responses and events.
	go func() {
		errCh <- p.pumpDlvToClient(conn, out)
	}()

	return <-errCh
}

// pumpClientToDlv reads DAP messages from the client (stdin) and forwards
// them to dlv, intercepting setBreakpoints to translate .srv → .go lines.
func (p *Proxy) pumpClientToDlv(client io.Reader, dlv io.Writer) error {
	scanner := newDAPScanner(client)
	for scanner.Scan() {
		raw := scanner.Bytes()

		translated, err := p.translateClientMessage(raw)
		if err != nil {
			// On translation error log and pass through unmodified.
			fmt.Fprintf(os.Stderr, "[dap-proxy] translate client→dlv: %v\n", err)
			translated = raw
		}

		if err := writeDAPMessage(dlv, translated); err != nil {
			return fmt.Errorf("write to dlv: %w", err)
		}
	}
	return scanner.Err()
}

// pumpDlvToClient reads DAP messages from dlv and forwards them to the client
// (stdout), intercepting stackTrace responses to translate .go → .srv lines.
func (p *Proxy) pumpDlvToClient(dlv io.Reader, client io.Writer) error {
	scanner := newDAPScanner(dlv)
	for scanner.Scan() {
		raw := scanner.Bytes()

		translated, err := p.translateDlvMessage(raw)
		if err != nil {
			fmt.Fprintf(os.Stderr, "[dap-proxy] translate dlv→client: %v\n", err)
			translated = raw
		}

		if err := writeDAPMessage(client, translated); err != nil {
			return fmt.Errorf("write to client: %w", err)
		}
	}
	return scanner.Err()
}

// translateClientMessage intercepts setBreakpoints requests and rewrites
// .srv file paths → .go paths and .srv line numbers → .go line numbers.
func (p *Proxy) translateClientMessage(raw []byte) ([]byte, error) {
	var req Request
	if err := json.Unmarshal(raw, &req); err != nil {
		return raw, nil // not JSON or not a request — pass through
	}
	if req.Command != "setBreakpoints" {
		return raw, nil
	}

	var args SetBreakpointsArguments
	if err := json.Unmarshal(req.Arguments, &args); err != nil {
		return raw, nil
	}

	// Only translate if the source is our .srv file.
	if !isSrvFile(args.Source.Path, p.srvFile) {
		return raw, nil
	}

	// Rewrite source path to point to the generated Go file.
	args.Source.Path = p.goFile
	args.Source.Name = filepath.Base(p.goFile)

	// Translate each breakpoint line.
	for i, bp := range args.Breakpoints {
		if goLine, ok := p.sm.SrvToGo(bp.Line); ok {
			args.Breakpoints[i].Line = goLine
		}
	}
	// Also translate the legacy "lines" array if present.
	for i, srvLine := range args.Lines {
		if goLine, ok := p.sm.SrvToGo(srvLine); ok {
			args.Lines[i] = goLine
		}
	}

	newArgs, err := json.Marshal(args)
	if err != nil {
		return raw, err
	}
	req.Arguments = newArgs

	return json.Marshal(req)
}

// translateDlvMessage intercepts stackTrace responses and rewrites
// .go paths → .srv paths and .go line numbers → .srv line numbers.
func (p *Proxy) translateDlvMessage(raw []byte) ([]byte, error) {
	var resp Response
	if err := json.Unmarshal(raw, &resp); err != nil {
		return raw, nil
	}
	if resp.Command != "stackTrace" || !resp.Success {
		return raw, nil
	}

	var body StackTraceBody
	if err := json.Unmarshal(resp.Body, &body); err != nil {
		return raw, nil
	}

	modified := false
	for i, frame := range body.StackFrames {
		if frame.Source == nil {
			continue
		}
		// Only translate frames pointing to our generated Go file.
		if !isGoFile(frame.Source.Path, p.goFile) {
			continue
		}
		if srvLine, ok := p.sm.GoToSrv(frame.Line); ok {
			body.StackFrames[i].Line = srvLine
			body.StackFrames[i].Source = &SourceSpec{
				Path: p.srvFile,
				Name: filepath.Base(p.srvFile),
			}
			modified = true
		}
	}

	if !modified {
		return raw, nil
	}

	newBody, err := json.Marshal(body)
	if err != nil {
		return raw, err
	}
	resp.Body = newBody
	return json.Marshal(resp)
}

// ── DAP framing ───────────────────────────────────────────────────────────────

// DAP messages are framed with HTTP-style headers:
//   Content-Length: <N>\r\n
//   \r\n
//   <N bytes of JSON>

// newDAPScanner returns a bufio.Scanner that splits on DAP Content-Length frames.
func newDAPScanner(r io.Reader) *bufio.Scanner {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4*1024*1024), 4*1024*1024) // 4 MB max message
	scanner.Split(splitDAPFrame)
	return scanner
}

// splitDAPFrame is a bufio.SplitFunc that reads Content-Length framed DAP messages.
func splitDAPFrame(data []byte, atEOF bool) (advance int, token []byte, err error) {
	if atEOF && len(data) == 0 {
		return 0, nil, nil
	}

	// Find the header separator \r\n\r\n
	headerEnd := strings.Index(string(data), "\r\n\r\n")
	if headerEnd < 0 {
		if atEOF {
			return 0, nil, io.ErrUnexpectedEOF
		}
		return 0, nil, nil // need more data
	}

	header := string(data[:headerEnd])
	var contentLength int
	for _, line := range strings.Split(header, "\r\n") {
		if strings.HasPrefix(line, "Content-Length: ") {
			n, e := strconv.Atoi(strings.TrimPrefix(line, "Content-Length: "))
			if e != nil {
				return 0, nil, fmt.Errorf("dap: invalid Content-Length: %w", e)
			}
			contentLength = n
		}
	}

	bodyStart := headerEnd + 4 // skip \r\n\r\n
	if len(data) < bodyStart+contentLength {
		if atEOF {
			return 0, nil, io.ErrUnexpectedEOF
		}
		return 0, nil, nil // need more data
	}

	body := data[bodyStart : bodyStart+contentLength]
	return bodyStart + contentLength, body, nil
}

// writeDAPMessage writes a DAP-framed message to w.
func writeDAPMessage(w io.Writer, body []byte) error {
	header := fmt.Sprintf("Content-Length: %d\r\n\r\n", len(body))
	if _, err := io.WriteString(w, header); err != nil {
		return err
	}
	_, err := w.Write(body)
	return err
}

// ── Path helpers ──────────────────────────────────────────────────────────────

func isSrvFile(path, srvFile string) bool {
	if path == "" {
		return false
	}
	// Compare clean absolute paths, case-insensitively on Windows.
	a := filepath.Clean(strings.ToLower(path))
	b := filepath.Clean(strings.ToLower(srvFile))
	return a == b || strings.HasSuffix(a, filepath.Base(b))
}

func isGoFile(path, goFile string) bool {
	if path == "" {
		return false
	}
	a := filepath.Clean(strings.ToLower(path))
	b := filepath.Clean(strings.ToLower(goFile))
	return a == b || strings.HasSuffix(a, "main.go")
}
