package runtime

import (
	"encoding/json"
	"net/http"
	"sync"

	"github.com/gorilla/websocket"
)

// WebSocket support

type WSConn struct {
	conn *websocket.Conn
	mu   sync.Mutex
}

func (w *WSConn) Send(msg interface{}) {
	w.mu.Lock()
	defer w.mu.Unlock()
	var data []byte
	if str, ok := msg.(string); ok {
		data = []byte(str)
	} else {
		data, _ = json.Marshal(msg)
	}
	w.conn.WriteMessage(websocket.TextMessage, data)
}

func (w *WSConn) Receive() interface{} {
	_, message, err := w.conn.ReadMessage()
	if err != nil {
		return nil
	}
	return string(message)
}

func (w *WSConn) Close() {
	w.conn.Close()
}

var (
	wsHandlers   = make(map[string]func(*WSConn))
	wsHandlersMu sync.RWMutex
	upgrader     = websocket.Upgrader{
		CheckOrigin: func(r *http.Request) bool { return true },
	}
)

func AddWebSocket(path string, handler func(*WSConn)) {
	wsHandlersMu.Lock()
	wsHandlers[path] = handler
	wsHandlersMu.Unlock()
	LogInfo("Registered WebSocket endpoint: ", path)
}
