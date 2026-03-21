package bidi

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/gorilla/websocket"
)

// WSTransport connects to Firefox's BiDi WebSocket endpoint.
// No null-byte framing — standard JSON over WebSocket.
type WSTransport struct {
	conn    *websocket.Conn
	writeMu sync.Mutex
}

// Dial connects to a BiDi WebSocket endpoint (e.g. ws://host:port/session/id).
func Dial(url string) (*WSTransport, error) {
	conn, _, err := websocket.DefaultDialer.Dial(url, nil)
	if err != nil {
		return nil, fmt.Errorf("bidi dial %s: %w", url, err)
	}
	return &WSTransport{conn: conn}, nil
}

// Send marshals a BiDi message and writes it as a WebSocket text frame.
func (t *WSTransport) Send(msg *Message) error {
	data, err := json.Marshal(msg)
	if err != nil {
		return fmt.Errorf("bidi marshal: %w", err)
	}
	t.writeMu.Lock()
	defer t.writeMu.Unlock()
	if err := t.conn.WriteMessage(websocket.TextMessage, data); err != nil {
		return fmt.Errorf("bidi write: %w", err)
	}
	return nil
}

// Receive reads a WebSocket text frame and unmarshals it as a BiDi message.
func (t *WSTransport) Receive() (*Message, error) {
	_, data, err := t.conn.ReadMessage()
	if err != nil {
		return nil, fmt.Errorf("bidi read: %w", err)
	}
	var msg Message
	if err := json.Unmarshal(data, &msg); err != nil {
		return nil, fmt.Errorf("bidi unmarshal: %w", err)
	}
	return &msg, nil
}

// Close closes the WebSocket connection.
func (t *WSTransport) Close() error {
	return t.conn.Close()
}
