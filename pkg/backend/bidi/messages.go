package bidi

import "encoding/json"

// Message represents a WebDriver BiDi protocol message.
// Unlike Juggler, BiDi has no sessionId field — context is embedded in params.
// Requests have ID+Method+Params, responses have ID+Result/Error,
// events have Method+Params (no ID).
type Message struct {
	ID     int             `json:"id,omitempty"`
	Method string          `json:"method,omitempty"`
	Params json.RawMessage `json:"params,omitempty"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *Error          `json:"error,omitempty"`
}

// Error represents a BiDi protocol error.
type Error struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

func (e *Error) Error() string { return e.Message }

// IsEvent returns true if this message is an event (no ID, has method).
func (m *Message) IsEvent() bool { return m.ID == 0 && m.Method != "" }

// IsResponse returns true if this message is a response (has ID, no method).
func (m *Message) IsResponse() bool { return m.ID != 0 && m.Method == "" }
