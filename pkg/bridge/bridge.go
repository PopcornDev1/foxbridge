package bridge

import (
	"encoding/json"
	"log"
	"strings"

	"github.com/PopcornDev1/foxbridge/pkg/backend"
	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

// Bridge translates CDP messages to Juggler protocol calls.
type Bridge struct {
	backend    backend.Backend
	sessions   *cdp.SessionManager
	server     *cdp.Server
	autoAttach *autoAttachState
}

// New creates a new Bridge.
func New(b backend.Backend, sessions *cdp.SessionManager, server *cdp.Server) *Bridge {
	return &Bridge{
		backend:    b,
		sessions:   sessions,
		server:     server,
		autoAttach: newAutoAttachState(),
	}
}

// HandleMessage dispatches an incoming CDP message to the appropriate domain handler.
func (b *Bridge) HandleMessage(conn *cdp.Connection, msg *cdp.Message) {
	method := msg.Method

	var result json.RawMessage
	var cdpErr *cdp.Error

	switch {
	case strings.HasPrefix(method, "Target."):
		result, cdpErr = b.handleTarget(conn, msg)
	case strings.HasPrefix(method, "Page."):
		result, cdpErr = b.handlePage(conn, msg)
	case strings.HasPrefix(method, "Runtime."):
		result, cdpErr = b.handleRuntime(conn, msg)
	case strings.HasPrefix(method, "Input."):
		result, cdpErr = b.handleInput(conn, msg)
	case strings.HasPrefix(method, "Network."):
		result, cdpErr = b.handleNetwork(conn, msg)
	case strings.HasPrefix(method, "Emulation."):
		result, cdpErr = b.handleEmulation(conn, msg)
	case strings.HasPrefix(method, "DOM."):
		result, cdpErr = b.handleDOM(conn, msg)
	case strings.HasPrefix(method, "Accessibility."):
		result, cdpErr = b.handleAccessibility(conn, msg)
	case strings.HasPrefix(method, "Console."):
		result, cdpErr = b.handleConsole(conn, msg)
	default:
		result, cdpErr = b.handleStub(conn, msg)
	}

	resp := &cdp.Message{
		ID:        msg.ID,
		SessionID: msg.SessionID,
	}
	if cdpErr != nil {
		resp.Error = cdpErr
	} else {
		if result == nil {
			result = json.RawMessage(`{}`)
		}
		resp.Result = result
	}

	if err := conn.Send(resp); err != nil {
		log.Printf("failed to send CDP response for %s: %v", method, err)
	}
}

// resolveSession maps a CDP sessionID to a Juggler sessionID.
func (b *Bridge) resolveSession(cdpSessionID string) string {
	if cdpSessionID == "" {
		return ""
	}
	if info, ok := b.sessions.Get(cdpSessionID); ok {
		return info.JugglerSessionID
	}
	return cdpSessionID
}

// callJuggler is a convenience wrapper for backend.Call with session resolution.
func (b *Bridge) callJuggler(cdpSessionID, method string, params interface{}) (json.RawMessage, error) {
	sessionID := b.resolveSession(cdpSessionID)
	var raw json.RawMessage
	if params != nil {
		var err error
		raw, err = json.Marshal(params)
		if err != nil {
			return nil, err
		}
	}
	return b.backend.Call(sessionID, method, raw)
}

// emitEvent sends a CDP event to all connected clients.
func (b *Bridge) emitEvent(method string, params interface{}, sessionID string) {
	var raw json.RawMessage
	if params != nil {
		raw, _ = json.Marshal(params)
	}
	b.server.Broadcast(&cdp.Message{
		Method:    method,
		Params:    raw,
		SessionID: sessionID,
	})
}
