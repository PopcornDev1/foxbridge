package bridge

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func TestResolveCDPSession_Empty(t *testing.T) {
	b, _ := newTestBridge()
	if got := b.resolveCDPSession(""); got != "" {
		t.Errorf("resolveCDPSession(\"\") = %q, want \"\"", got)
	}
}

func TestResolveCDPSession_WithPair(t *testing.T) {
	b, _ := newTestBridge()
	pair := &targetPair{
		tabSessionID:  "tab-s1",
		pageSessionID: "page-s1",
		pageTargetID:  "t1",
	}
	b.autoAttach.mu.Lock()
	b.autoAttach.pairs["jug-1"] = pair
	b.autoAttach.mu.Unlock()

	got := b.resolveCDPSession("jug-1")
	if got != "page-s1" {
		t.Errorf("resolveCDPSession(\"jug-1\") = %q, want \"page-s1\"", got)
	}
}

func TestResolveCDPSession_FallbackToSessionManager(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "cdp-s1",
		JugglerSessionID: "jug-s1",
		TargetID:         "t1",
	})

	got := b.resolveCDPSession("jug-s1")
	if got != "cdp-s1" {
		t.Errorf("resolveCDPSession(\"jug-s1\") = %q, want \"cdp-s1\"", got)
	}
}

func TestResolveCDPSession_NotFound(t *testing.T) {
	b, _ := newTestBridge()
	got := b.resolveCDPSession("nonexistent")
	if got != "" {
		t.Errorf("resolveCDPSession(\"nonexistent\") = %q, want \"\"", got)
	}
}

func TestSetupEventSubscriptions_AttachedToTarget(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	// Simulate Browser.attachedToTarget event
	mb.mu.Lock()
	handlers := mb.handlers["Browser.attachedToTarget"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handlers registered for Browser.attachedToTarget")
	}

	params := json.RawMessage(`{
		"sessionId": "jug-s1",
		"targetInfo": {
			"targetId": "t1",
			"browserContextId": "ctx-1",
			"type": "page",
			"url": "https://example.com"
		}
	}`)

	handlers[0]("", params)

	// Give async operations a moment
	time.Sleep(10 * time.Millisecond)

	// Page session should be registered
	info, ok := b.sessions.GetByTarget("t1")
	if !ok {
		t.Fatal("expected session for target t1")
	}
	if info.Type != "page" {
		t.Errorf("type = %q, want page", info.Type)
	}
	if info.JugglerSessionID != "jug-s1" {
		t.Errorf("jugglerSessionID = %q, want jug-s1", info.JugglerSessionID)
	}
}

func TestSetupEventSubscriptions_AttachedToTarget_Worker(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	mb.mu.Lock()
	handlers := mb.handlers["Browser.attachedToTarget"]
	mb.mu.Unlock()

	params := json.RawMessage(`{
		"sessionId": "jug-w1",
		"targetInfo": {
			"targetId": "worker-t1",
			"browserContextId": "ctx-1",
			"type": "worker",
			"url": "https://example.com/sw.js"
		}
	}`)

	handlers[0]("", params)

	info, ok := b.sessions.GetByTarget("worker-t1")
	if !ok {
		t.Fatal("expected session for worker target")
	}
	if info.Type != "worker" {
		t.Errorf("type = %q, want worker", info.Type)
	}
}

func TestSetupEventSubscriptions_DetachedFromTarget(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	// Pre-register a session
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "cdp-s1",
		JugglerSessionID: "jug-s1",
		TargetID:         "t1",
		Type:             "page",
	})

	mb.mu.Lock()
	handlers := mb.handlers["Browser.detachedFromTarget"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handlers registered for Browser.detachedFromTarget")
	}

	params := json.RawMessage(`{"sessionId":"jug-s1","targetId":"t1"}`)
	handlers[0]("", params)

	// Session should be removed
	if _, ok := b.sessions.Get("cdp-s1"); ok {
		t.Error("session cdp-s1 should have been removed")
	}
}

func TestSetupEventSubscriptions_NavigationCommitted(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "cdp-s1",
		JugglerSessionID: "jug-s1",
		TargetID:         "t1",
		URL:              "about:blank",
	})

	mb.mu.Lock()
	handlers := mb.handlers["Page.navigationCommitted"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handlers registered for Page.navigationCommitted")
	}

	params := json.RawMessage(`{"frameId":"frame-1","url":"https://example.com","navigationId":"nav-1"}`)
	handlers[0]("jug-s1", params)

	// URL should be updated in session
	info, ok := b.sessions.GetByJugglerSession("jug-s1")
	if !ok {
		t.Fatal("session not found")
	}
	if info.URL != "https://example.com" {
		t.Errorf("URL = %q, want https://example.com", info.URL)
	}
}

func TestSetupEventSubscriptions_ExecutionContextCreated(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "cdp-s1",
		JugglerSessionID: "jug-s1",
		TargetID:         "t1",
	})

	mb.mu.Lock()
	handlers := mb.handlers["Runtime.executionContextCreated"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handlers registered for Runtime.executionContextCreated")
	}

	params := json.RawMessage(`{"executionContextId":"jug-ctx-1","auxData":{"frameId":"frame-1","name":""}}`)
	handlers[0]("jug-s1", params)

	// Frame ID should be stored
	info, _ := b.sessions.GetByJugglerSession("jug-s1")
	if info.FrameID != "frame-1" {
		t.Errorf("frameID = %q, want frame-1", info.FrameID)
	}

	// Context mapping should exist
	b.ctxMapMu.RLock()
	found := false
	for _, v := range b.ctxMap {
		if v == "jug-ctx-1" {
			found = true
			break
		}
	}
	b.ctxMapMu.RUnlock()
	if !found {
		t.Error("expected context mapping for jug-ctx-1")
	}
}

func TestSetupEventSubscriptions_ExecutionContextDestroyed(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	// Pre-populate context mapping
	b.ctxMapMu.Lock()
	b.ctxMap[150] = "jug-ctx-1"
	b.ctxMapMu.Unlock()

	mb.mu.Lock()
	handlers := mb.handlers["Runtime.executionContextDestroyed"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handlers registered for Runtime.executionContextDestroyed")
	}

	params := json.RawMessage(`{"executionContextId":"jug-ctx-1"}`)
	handlers[0]("jug-s1", params)

	// Mapping should be cleaned up
	b.ctxMapMu.RLock()
	_, exists := b.ctxMap[150]
	b.ctxMapMu.RUnlock()
	if exists {
		t.Error("context mapping for 150 should have been removed")
	}
}

func TestSetupEventSubscriptions_AllEventsSubscribed(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	expectedEvents := []string{
		"Browser.attachedToTarget",
		"Browser.detachedFromTarget",
		"Page.navigationCommitted",
		"Page.eventFired",
		"Runtime.executionContextsCleared",
		"Runtime.executionContextCreated",
		"Runtime.executionContextDestroyed",
		"Runtime.console",
		"Page.frameAttached",
		"Page.frameDetached",
		"Page.dialogOpened",
		"Page.dialogClosed",
		"Network.requestWillBeSent",
		"Network.responseReceived",
		"Network.requestFinished",
		"Network.requestFailed",
		"Browser.requestIntercepted",
		"Page.webSocketCreated",
		"Page.webSocketOpened",
		"Page.webSocketClosed",
		"Page.webSocketFrameSent",
		"Page.webSocketFrameReceived",
		"Browser.downloadCreated",
		"Browser.downloadFinished",
		"Page.screencastFrame",
		"Page.fileChooserOpened",
	}

	mb.mu.Lock()
	for _, event := range expectedEvents {
		if len(mb.handlers[event]) == 0 {
			t.Errorf("no handler registered for %s", event)
		}
	}
	mb.mu.Unlock()
}

func TestNewAutoAttachState(t *testing.T) {
	s := newAutoAttachState()
	if s.enabled {
		t.Error("autoAttach should be disabled by default")
	}
	if s.pairs == nil {
		t.Error("pairs map should be initialized")
	}
	if s.pendingFrameIDs == nil {
		t.Error("pendingFrameIDs map should be initialized")
	}
}

func TestSetupEventSubscriptions_PendingFrameID(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	// Simulate executionContextCreated arriving BEFORE session registration
	mb.mu.Lock()
	ctxHandler := mb.handlers["Runtime.executionContextCreated"]
	mb.mu.Unlock()

	params := json.RawMessage(`{"executionContextId":"jug-ctx-1","auxData":{"frameId":"frame-buffered","name":""}}`)
	ctxHandler[0]("jug-unregistered", params)

	// Frame ID should be buffered
	b.autoAttach.mu.Lock()
	buffered, ok := b.autoAttach.pendingFrameIDs["jug-unregistered"]
	b.autoAttach.mu.Unlock()
	if !ok {
		t.Fatal("expected buffered frameID")
	}
	if buffered != "frame-buffered" {
		t.Errorf("buffered frameID = %q, want frame-buffered", buffered)
	}
}

func TestSetupEventSubscriptions_WebSocketCreated(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	// Register a session so resolveCDPSession works
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "cdp-s1",
		JugglerSessionID: "jug-s1",
		TargetID:         "t1",
	})

	mb.mu.Lock()
	handlers := mb.handlers["Page.webSocketCreated"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handler registered for Page.webSocketCreated")
	}

	params := json.RawMessage(`{"requestId":"ws-1","url":"wss://example.com/ws"}`)
	handlers[0]("jug-s1", params)

	// Event should be forwarded (via emitEventRaw which calls server.Broadcast)
	// Since we can't easily capture broadcasts in the test, just verify the handler exists and runs without panic
}

func TestSetupEventSubscriptions_WebSocketFrameEvents(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	wsEvents := []string{
		"Page.webSocketOpened",
		"Page.webSocketClosed",
		"Page.webSocketFrameSent",
		"Page.webSocketFrameReceived",
	}

	mb.mu.Lock()
	for _, event := range wsEvents {
		if len(mb.handlers[event]) == 0 {
			t.Errorf("no handler registered for %s", event)
		}
	}
	mb.mu.Unlock()
}

func TestSetupEventSubscriptions_DownloadCreated(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	mb.mu.Lock()
	handlers := mb.handlers["Browser.downloadCreated"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handler registered for Browser.downloadCreated")
	}

	params := json.RawMessage(`{"uuid":"dl-1","url":"https://example.com/file.zip","suggestedFileName":"file.zip"}`)
	handlers[0]("", params)
}

func TestSetupEventSubscriptions_DownloadFinished(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	mb.mu.Lock()
	handlers := mb.handlers["Browser.downloadFinished"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handler registered for Browser.downloadFinished")
	}

	// Test completed state
	params := json.RawMessage(`{"uuid":"dl-1","canceled":false,"error":""}`)
	handlers[0]("", params)

	// Test canceled state
	params = json.RawMessage(`{"uuid":"dl-2","canceled":true,"error":""}`)
	handlers[0]("", params)

	// Test error state
	params = json.RawMessage(`{"uuid":"dl-3","canceled":false,"error":"disk full"}`)
	handlers[0]("", params)
}

func TestSetupEventSubscriptions_ScreencastFrame(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	mb.mu.Lock()
	handlers := mb.handlers["Page.screencastFrame"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handler registered for Page.screencastFrame")
	}

	params := json.RawMessage(`{"data":"base64data","metadata":{"timestamp":1234}}`)
	handlers[0]("jug-s1", params)
}

func TestSetupEventSubscriptions_FileChooserOpened(t *testing.T) {
	b, mb := newTestBridge()
	b.SetupEventSubscriptions()

	mb.mu.Lock()
	handlers := mb.handlers["Page.fileChooserOpened"]
	mb.mu.Unlock()

	if len(handlers) == 0 {
		t.Fatal("no handler registered for Page.fileChooserOpened")
	}

	params := json.RawMessage(`{"executionContextId":"ctx-1","multiple":false}`)
	handlers[0]("jug-s1", params)
}
