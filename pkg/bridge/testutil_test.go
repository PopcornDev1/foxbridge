package bridge

import (
	"encoding/json"
	"fmt"
	"sync"

	"github.com/PopcornDev1/foxbridge/pkg/backend"
)

// mockCall records a single backend.Call invocation.
type mockCall struct {
	SessionID string
	Method    string
	Params    json.RawMessage
	Result    json.RawMessage
	Err       error
}

// mockBackend implements backend.Backend for testing.
type mockBackend struct {
	mu       sync.Mutex
	calls    []mockCall
	handlers map[string][]backend.EventHandler
	// responses maps "sessionID:method" to a preconfigured response.
	responses map[string]mockCall
}

func newMockBackend() *mockBackend {
	return &mockBackend{
		handlers:  make(map[string][]backend.EventHandler),
		responses: make(map[string]mockCall),
	}
}

// SetResponse preconfigures a response for a given sessionID+method pair.
func (m *mockBackend) SetResponse(sessionID, method string, result json.RawMessage, err error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	key := sessionID + ":" + method
	m.responses[key] = mockCall{Result: result, Err: err}
}

func (m *mockBackend) Call(sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	m.mu.Lock()
	defer m.mu.Unlock()

	call := mockCall{
		SessionID: sessionID,
		Method:    method,
		Params:    params,
	}

	// Check for preconfigured response
	key := sessionID + ":" + method
	if resp, ok := m.responses[key]; ok {
		call.Result = resp.Result
		call.Err = resp.Err
		m.calls = append(m.calls, call)
		return resp.Result, resp.Err
	}

	// Also check wildcard (empty sessionID)
	key = ":" + method
	if resp, ok := m.responses[key]; ok {
		call.Result = resp.Result
		call.Err = resp.Err
		m.calls = append(m.calls, call)
		return resp.Result, resp.Err
	}

	// Default: return empty object
	call.Result = json.RawMessage(`{}`)
	m.calls = append(m.calls, call)
	return call.Result, nil
}

func (m *mockBackend) Subscribe(event string, handler backend.EventHandler) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.handlers[event] = append(m.handlers[event], handler)
}

func (m *mockBackend) Close() error {
	return nil
}

// LastCall returns the most recent call, or an error if none.
func (m *mockBackend) LastCall() (mockCall, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if len(m.calls) == 0 {
		return mockCall{}, fmt.Errorf("no calls recorded")
	}
	return m.calls[len(m.calls)-1], nil
}

// CallCount returns the number of recorded calls.
func (m *mockBackend) CallCount() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.calls)
}

// CallsForMethod returns all calls matching the given method.
func (m *mockBackend) CallsForMethod(method string) []mockCall {
	m.mu.Lock()
	defer m.mu.Unlock()
	var result []mockCall
	for _, c := range m.calls {
		if c.Method == method {
			result = append(result, c)
		}
	}
	return result
}

// mockServer implements just enough of cdp.Server for Bridge tests.
// We capture broadcasts rather than sending over a real WebSocket.
type broadcastCapture struct {
	mu       sync.Mutex
	messages []*mockBroadcast
}

type mockBroadcast struct {
	Method    string
	Params    json.RawMessage
	SessionID string
}

func (bc *broadcastCapture) record(method string, params json.RawMessage, sessionID string) {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	bc.messages = append(bc.messages, &mockBroadcast{
		Method:    method,
		Params:    params,
		SessionID: sessionID,
	})
}

func (bc *broadcastCapture) count() int {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	return len(bc.messages)
}

func (bc *broadcastCapture) last() *mockBroadcast {
	bc.mu.Lock()
	defer bc.mu.Unlock()
	if len(bc.messages) == 0 {
		return nil
	}
	return bc.messages[len(bc.messages)-1]
}
