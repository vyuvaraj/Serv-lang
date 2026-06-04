package runtime

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os"
	"sync"
)

type MCPTool struct {
	Name        string
	Description string
	Handler     func(interface{}) interface{}
}

var (
	mcpTools   = make(map[string]MCPTool)
	mcpToolsMu sync.RWMutex
)

func AddMCPTool(name, description string, handler func(interface{}) interface{}) {
	mcpToolsMu.Lock()
	defer mcpToolsMu.Unlock()
	mcpTools[name] = MCPTool{
		Name:        name,
		Description: description,
		Handler:     handler,
	}
}

func InvokeMCPToolForTesting(name string, args interface{}) interface{} {
	mcpToolsMu.RLock()
	t, ok := mcpTools[name]
	mcpToolsMu.RUnlock()
	if !ok {
		panic("tool not found: " + name)
	}
	return t.Handler(args)
}

type jsonRPCRequest struct {
	JSONRPC string          `json:"jsonrpc"`
	ID      interface{}     `json:"id,omitempty"`
	Method  string          `json:"method"`
	Params  json.RawMessage `json:"params,omitempty"`
}

type jsonRPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Result  interface{} `json:"result,omitempty"`
	Error   interface{} `json:"error,omitempty"`
}

func startMCPServer() {
	scanner := bufio.NewScanner(os.Stdin)
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			continue
		}
		var req jsonRPCRequest
		if err := json.Unmarshal(line, &req); err != nil {
			sendRPCError(nil, -32700, "Parse error")
			continue
		}
		handleMCPRequest(req)
	}
}

func sendRPCError(id interface{}, code int, message string) {
	resp := jsonRPCResponse{
		JSONRPC: "2.0",
		ID:      id,
		Error: map[string]interface{}{
			"code":    code,
			"message": message,
		},
	}
	b, _ := json.Marshal(resp)
	fmt.Println(string(b))
}

func handleMCPRequest(req jsonRPCRequest) {
	switch req.Method {
	case "initialize":
		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"protocolVersion": "2024-11-05",
				"capabilities": map[string]interface{}{
					"tools": map[string]interface{}{},
				},
				"serverInfo": map[string]interface{}{
					"name":    "Serv-MCP",
					"version": "1.0.0",
				},
			},
		}
		b, _ := json.Marshal(resp)
		fmt.Println(string(b))

	case "notifications/initialized":
		// Notification, no reply

	case "tools/list":
		mcpToolsMu.RLock()
		toolsList := []map[string]interface{}{}
		for _, t := range mcpTools {
			toolsList = append(toolsList, map[string]interface{}{
				"name":        t.Name,
				"description": t.Description,
				"inputSchema": map[string]interface{}{
					"type":       "object",
					"properties": map[string]interface{}{},
				},
			})
		}
		mcpToolsMu.RUnlock()

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"tools": toolsList,
			},
		}
		b, _ := json.Marshal(resp)
		fmt.Println(string(b))

	case "tools/call":
		var params struct {
			Name      string                 `json:"name"`
			Arguments map[string]interface{} `json:"arguments"`
		}
		if err := json.Unmarshal(req.Params, &params); err != nil {
			sendRPCError(req.ID, -32602, "Invalid params")
			return
		}

		mcpToolsMu.RLock()
		tool, exists := mcpTools[params.Name]
		mcpToolsMu.RUnlock()

		if !exists {
			sendRPCError(req.ID, -32601, "Tool not found: "+params.Name)
			return
		}

		// Run tool handler
		res := tool.Handler(params.Arguments)

		// Convert result to standard MCP content
		var text string
		if resStr, ok := res.(string); ok {
			text = resStr
		} else {
			b, _ := json.Marshal(res)
			text = string(b)
		}

		resp := jsonRPCResponse{
			JSONRPC: "2.0",
			ID:      req.ID,
			Result: map[string]interface{}{
				"content": []map[string]interface{}{
					{
						"type": "text",
						"text": text,
					},
				},
			},
		}
		b, _ := json.Marshal(resp)
		fmt.Println(string(b))

	default:
		sendRPCError(req.ID, -32601, "Method not found: "+req.Method)
	}
}
