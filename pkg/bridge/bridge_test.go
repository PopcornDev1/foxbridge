package bridge

import (
	"encoding/json"
	"sync"
	"testing"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

// mockConn is a minimal cdp.Connection that captures sent messages.
type mockConn struct {
	mu       sync.Mutex
	sent     []*cdp.Message
	sendErr  error
}

func (mc *mockConn) lastSent() *cdp.Message {
	mc.mu.Lock()
	defer mc.mu.Unlock()
	if len(mc.sent) == 0 {
		return nil
	}
	return mc.sent[len(mc.sent)-1]
}

// newTestBridge creates a Bridge backed by a mock backend + real SessionManager + real Server.
// Since we can't easily mock cdp.Server.Broadcast (it's a concrete type with WS conns),
// we test Bridge methods that return results directly.
func newTestBridge() (*Bridge, *mockBackend) {
	mb := newMockBackend()
	sessions := cdp.NewSessionManager()
	server := cdp.NewServer(0, nil, sessions)
	b := New(mb, sessions, server)
	return b, mb
}

func TestHandleMessage_DispatchesByDomain(t *testing.T) {
	tests := []struct {
		name       string
		method     string
		params     string
		wantError  bool
		wantResult string // substring to check in result
	}{
		{
			name:       "Page.enable returns success",
			method:     "Page.enable",
			params:     `{}`,
			wantResult: `{}`,
		},
		{
			name:       "Runtime.runIfWaitingForDebugger returns success",
			method:     "Runtime.runIfWaitingForDebugger",
			params:     `{}`,
			wantResult: `{}`,
		},
		{
			name:       "DOM.enable returns success",
			method:     "DOM.enable",
			params:     `{}`,
			wantResult: `{}`,
		},
		{
			name:       "Accessibility.enable returns success",
			method:     "Accessibility.enable",
			params:     `{}`,
			wantResult: `{}`,
		},
		{
			name:       "stub domain Log.enable returns success",
			method:     "Log.enable",
			params:     `{}`,
			wantResult: `{}`,
		},
		{
			name:      "unknown method returns error",
			method:    "FakeDomain.fakeMethod",
			params:    `{}`,
			wantError: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, _ := newTestBridge()

			// We need a real connection to test HandleMessage, but since we can't
			// easily construct one without a websocket, test the individual handlers instead.
			msg := &cdp.Message{
				ID:     1,
				Method: tt.method,
				Params: json.RawMessage(tt.params),
			}

			var result json.RawMessage
			var cdpErr *cdp.Error

			// Call the specific handler based on prefix, mirroring HandleMessage logic
			switch {
			case len(tt.method) > 5 && tt.method[:5] == "Page.":
				result, cdpErr = b.handlePage(nil, msg)
			case len(tt.method) > 8 && tt.method[:8] == "Runtime.":
				result, cdpErr = b.handleRuntime(nil, msg)
			case len(tt.method) > 4 && tt.method[:4] == "DOM.":
				result, cdpErr = b.handleDOM(nil, msg)
			case len(tt.method) > 14 && tt.method[:14] == "Accessibility.":
				result, cdpErr = b.handleAccessibility(nil, msg)
			default:
				result, cdpErr = b.handleStub(nil, msg)
			}

			if tt.wantError {
				if cdpErr == nil {
					t.Errorf("expected error for %s, got nil", tt.method)
				}
				return
			}

			if cdpErr != nil {
				t.Errorf("unexpected error for %s: %v", tt.method, cdpErr.Message)
				return
			}

			if tt.wantResult != "" && string(result) != tt.wantResult {
				t.Errorf("result = %s, want %s", string(result), tt.wantResult)
			}
		})
	}
}

func TestResolveSession_EmptyReturnsEmpty(t *testing.T) {
	b, _ := newTestBridge()
	got := b.resolveSession("")
	if got != "" {
		t.Errorf("resolveSession(\"\") = %q, want \"\"", got)
	}
}

func TestResolveSession_KnownSession(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "cdp-123",
		JugglerSessionID: "juggler-abc",
		TargetID:         "target-1",
	})

	got := b.resolveSession("cdp-123")
	if got != "juggler-abc" {
		t.Errorf("resolveSession(\"cdp-123\") = %q, want \"juggler-abc\"", got)
	}
}

func TestResolveSession_UnknownSessionPassthrough(t *testing.T) {
	b, _ := newTestBridge()
	// When session is not found, it returns the input as-is
	got := b.resolveSession("unknown-session")
	if got != "unknown-session" {
		t.Errorf("resolveSession(\"unknown-session\") = %q, want \"unknown-session\"", got)
	}
}

func TestNextCtxID_MonotonicallyIncreasing(t *testing.T) {
	b, _ := newTestBridge()

	ids := make([]int, 100)
	for i := range ids {
		ids[i] = b.nextCtxID()
	}

	for i := 1; i < len(ids); i++ {
		if ids[i] <= ids[i-1] {
			t.Errorf("nextCtxID not monotonic: ids[%d]=%d <= ids[%d]=%d", i, ids[i], i-1, ids[i-1])
		}
	}

	// First ID should be > 100 (starts at 100, first call returns 101)
	if ids[0] <= 100 {
		t.Errorf("first nextCtxID = %d, want > 100", ids[0])
	}
}

func TestNextCtxID_ConcurrentSafety(t *testing.T) {
	b, _ := newTestBridge()

	const goroutines = 50
	const idsPerRoutine = 20
	results := make(chan int, goroutines*idsPerRoutine)

	var wg sync.WaitGroup
	for i := 0; i < goroutines; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for j := 0; j < idsPerRoutine; j++ {
				results <- b.nextCtxID()
			}
		}()
	}
	wg.Wait()
	close(results)

	seen := make(map[int]bool)
	for id := range results {
		if seen[id] {
			t.Errorf("duplicate context ID: %d", id)
		}
		seen[id] = true
	}

	if len(seen) != goroutines*idsPerRoutine {
		t.Errorf("got %d unique IDs, want %d", len(seen), goroutines*idsPerRoutine)
	}
}

func TestCallJuggler_MarshalParams(t *testing.T) {
	b, mb := newTestBridge()

	_, err := b.callJuggler("", "Browser.getInfo", map[string]string{"key": "val"})
	if err != nil {
		t.Fatalf("callJuggler returned error: %v", err)
	}

	last, err := mb.LastCall()
	if err != nil {
		t.Fatal(err)
	}
	if last.Method != "Browser.getInfo" {
		t.Errorf("method = %q, want %q", last.Method, "Browser.getInfo")
	}

	var params map[string]string
	if err := json.Unmarshal(last.Params, &params); err != nil {
		t.Fatalf("unmarshal params: %v", err)
	}
	if params["key"] != "val" {
		t.Errorf("params[key] = %q, want %q", params["key"], "val")
	}
}

func TestCallJuggler_NilParams(t *testing.T) {
	b, mb := newTestBridge()

	_, err := b.callJuggler("", "Page.close", nil)
	if err != nil {
		t.Fatalf("callJuggler returned error: %v", err)
	}

	last, err := mb.LastCall()
	if err != nil {
		t.Fatal(err)
	}
	if last.Params != nil {
		t.Errorf("expected nil params, got %s", string(last.Params))
	}
}

func TestCallJuggler_SessionResolution(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "cdp-1",
		JugglerSessionID: "jug-1",
		TargetID:         "t-1",
	})

	_, err := b.callJuggler("cdp-1", "Page.reload", nil)
	if err != nil {
		t.Fatalf("callJuggler returned error: %v", err)
	}

	last, err := mb.LastCall()
	if err != nil {
		t.Fatal(err)
	}
	if last.SessionID != "jug-1" {
		t.Errorf("sessionID = %q, want %q", last.SessionID, "jug-1")
	}
}

func TestHandlePage_Navigate(t *testing.T) {
	b, mb := newTestBridge()

	mb.SetResponse("", "Page.navigate", json.RawMessage(`{"navigationId":"nav-1","frameId":"frame-1"}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.navigate",
		Params: json.RawMessage(`{"url":"https://example.com"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("handlePage error: %s", cdpErr.Message)
	}

	var res map[string]interface{}
	json.Unmarshal(result, &res)

	if res["frameId"] != "frame-1" {
		t.Errorf("frameId = %v, want frame-1", res["frameId"])
	}
	if res["loaderId"] != "nav-1" {
		t.Errorf("loaderId = %v, want nav-1", res["loaderId"])
	}
}

func TestHandlePage_NavigateInvalidParams(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.navigate",
		Params: json.RawMessage(`not-json`),
	}

	_, cdpErr := b.handlePage(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestHandlePage_GetFrameTree(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID: "s1",
		TargetID:  "t1",
		FrameID:   "frame-abc",
		URL:       "https://example.com",
	})

	msg := &cdp.Message{
		ID:        1,
		Method:    "Page.getFrameTree",
		SessionID: "s1",
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("handlePage error: %s", cdpErr.Message)
	}

	var res struct {
		FrameTree struct {
			Frame struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	json.Unmarshal(result, &res)

	if res.FrameTree.Frame.ID != "frame-abc" {
		t.Errorf("frame id = %q, want %q", res.FrameTree.Frame.ID, "frame-abc")
	}
	if res.FrameTree.Frame.URL != "https://example.com" {
		t.Errorf("frame url = %q, want %q", res.FrameTree.Frame.URL, "https://example.com")
	}
}

func TestHandlePage_CaptureScreenshot(t *testing.T) {
	b, mb := newTestBridge()

	mb.SetResponse("", "Page.screenshot", json.RawMessage(`{"data":"iVBORbase64data"}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.captureScreenshot",
		Params: json.RawMessage(`{"format":"png"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("handlePage error: %s", cdpErr.Message)
	}

	var res struct {
		Data string `json:"data"`
	}
	json.Unmarshal(result, &res)

	if res.Data != "iVBORbase64data" {
		t.Errorf("data = %q, want iVBORbase64data", res.Data)
	}
}

func TestHandlePage_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.doesNotExist",
		Params: json.RawMessage(`{}`),
	}

	_, cdpErr := b.handlePage(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown Page method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}

func TestHandleStub_BrowserGetVersion(t *testing.T) {
	b, mb := newTestBridge()

	mb.SetResponse("", "Browser.getInfo",
		json.RawMessage(`{"version":"Firefox/146.0.1","userAgent":"Mozilla/5.0"}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Browser.getVersion",
	}

	result, cdpErr := b.handleStub(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res map[string]string
	json.Unmarshal(result, &res)

	if res["protocolVersion"] != "1.3" {
		t.Errorf("protocolVersion = %q, want 1.3", res["protocolVersion"])
	}
	if res["userAgent"] != "Mozilla/5.0" {
		t.Errorf("userAgent = %q, want Mozilla/5.0", res["userAgent"])
	}
}

func TestHandleStub_KnownDomainNoops(t *testing.T) {
	domains := []string{
		"Log.enable", "Security.disable", "CSS.enable",
		"Debugger.enable", "Profiler.disable", "IndexedDB.enable",
	}

	for _, method := range domains {
		t.Run(method, func(t *testing.T) {
			b, _ := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: method, Params: json.RawMessage(`{}`)}
			result, cdpErr := b.handleStub(nil, msg)
			if cdpErr != nil {
				t.Errorf("error for stub %s: %s", method, cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
		})
	}
}
