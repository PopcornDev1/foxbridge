package bridge

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func TestHandleTarget_SetDiscoverTargets(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		TargetID:         "t1",
		Type:             "page",
		Title:            "Test",
		URL:              "https://example.com",
		BrowserContextID: "ctx-1",
	})

	msg := &cdp.Message{ID: 1, Method: "Target.setDiscoverTargets", Params: json.RawMessage(`{"discover":true}`)}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// Event is emitted via broadcast — we verify no error and success response.
}

func TestHandleTarget_SetDiscoverTargets_BlankURLForPage(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID: "s1",
		TargetID:  "t1",
		Type:      "page",
		URL:       "",
	})

	msg := &cdp.Message{ID: 1, Method: "Target.setDiscoverTargets", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
}

func TestHandleTarget_SetAutoAttach_BrowserLevel(t *testing.T) {
	b, _ := newTestBridge()

	// Add a pending pair before enabling auto-attach
	pair := &targetPair{
		tabSessionID:  "tab-s1",
		tabTargetID:   "tab-t1",
		pageSessionID: "page-s1",
		pageTargetID:  "page-t1",
		browserCtxID:  "ctx-1",
		url:           "https://example.com",
	}
	b.autoAttach.mu.Lock()
	b.autoAttach.pending = append(b.autoAttach.pending, pair)
	b.autoAttach.mu.Unlock()

	msg := &cdp.Message{
		ID:     1,
		Method: "Target.setAutoAttach",
		Params: json.RawMessage(`{"autoAttach":true,"waitForDebuggerOnStart":true,"flatten":true}`),
	}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	b.autoAttach.mu.Lock()
	enabled := b.autoAttach.enabled
	pendingLen := len(b.autoAttach.pending)
	b.autoAttach.mu.Unlock()

	if !enabled {
		t.Error("autoAttach should be enabled")
	}
	if pendingLen != 0 {
		t.Errorf("pending = %d, want 0 (should be drained)", pendingLen)
	}
}

func TestHandleTarget_SetAutoAttach_SessionLevel_Tab(t *testing.T) {
	b, _ := newTestBridge()

	tabSessionID := "tab-s1"
	b.sessions.Add(&cdp.SessionInfo{
		SessionID: tabSessionID,
		TargetID:  "tab-t1",
		Type:      "tab",
	})

	pair := &targetPair{
		tabSessionID:  tabSessionID,
		tabTargetID:   "tab-t1",
		pageSessionID: "page-s1",
		pageTargetID:  "page-t1",
		browserCtxID:  "ctx-1",
	}
	b.autoAttach.mu.Lock()
	b.autoAttach.pairs["jug-1"] = pair
	b.autoAttach.mu.Unlock()

	msg := &cdp.Message{
		ID:        1,
		Method:    "Target.setAutoAttach",
		SessionID: tabSessionID,
		Params:    json.RawMessage(`{"autoAttach":true,"flatten":true}`),
	}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleTarget_CreateTarget(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Browser.newPage", json.RawMessage(`{"targetId":"new-page-1"}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Target.createTarget",
		Params: json.RawMessage(`{"url":"about:blank","browserContextId":"ctx-1"}`),
	}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res map[string]string
	json.Unmarshal(result, &res)
	if res["targetId"] != "new-page-1" {
		t.Errorf("targetId = %q, want new-page-1", res["targetId"])
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.newPage" {
		t.Errorf("method = %q, want Browser.newPage", last.Method)
	}
}

func TestHandleTarget_CreateTarget_FallbackUUID(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Browser.newPage", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Target.createTarget",
		Params: json.RawMessage(`{"url":"about:blank"}`),
	}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res map[string]string
	json.Unmarshal(result, &res)
	if res["targetId"] == "" {
		t.Error("expected non-empty targetId (UUID fallback)")
	}
}

func TestHandleTarget_CloseTarget(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		JugglerSessionID: "jug-1",
		TargetID:         "t1",
	})
	mb.SetResponse("jug-1", "Page.close", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Target.closeTarget",
		Params: json.RawMessage(`{"targetId":"t1"}`),
	}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res map[string]interface{}
	json.Unmarshal(result, &res)
	if res["success"] != true {
		t.Errorf("success = %v, want true", res["success"])
	}

	// Session removal is async — wait briefly
	time.Sleep(50 * time.Millisecond)
	if _, ok := b.sessions.Get("s1"); ok {
		t.Error("session s1 should have been removed")
	}
}

func TestHandleTarget_CloseTarget_NotFound(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{
		ID:     1,
		Method: "Target.closeTarget",
		Params: json.RawMessage(`{"targetId":"nonexistent"}`),
	}
	_, cdpErr := b.handleTarget(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for nonexistent target")
	}
	if cdpErr.Code != -32000 {
		t.Errorf("error code = %d, want -32000", cdpErr.Code)
	}
}

func TestHandleTarget_CreateBrowserContext(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Browser.createBrowserContext", json.RawMessage(`{"browserContextId":"new-ctx-1"}`), nil)

	msg := &cdp.Message{ID: 1, Method: "Target.createBrowserContext", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res map[string]string
	json.Unmarshal(result, &res)
	if res["browserContextId"] != "new-ctx-1" {
		t.Errorf("browserContextId = %q, want new-ctx-1", res["browserContextId"])
	}
}

func TestHandleTarget_DisposeBrowserContext(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Browser.removeBrowserContext", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Target.disposeBrowserContext",
		Params: json.RawMessage(`{"browserContextId":"ctx-1"}`),
	}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.removeBrowserContext" {
		t.Errorf("method = %q, want Browser.removeBrowserContext", last.Method)
	}
}

func TestHandleTarget_GetTargets(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		TargetID:         "t1",
		Type:             "page",
		Title:            "Test Page",
		URL:              "https://example.com",
		BrowserContextID: "ctx-1",
	})

	msg := &cdp.Message{ID: 1, Method: "Target.getTargets", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res struct {
		TargetInfos []map[string]interface{} `json:"targetInfos"`
	}
	json.Unmarshal(result, &res)
	if len(res.TargetInfos) != 1 {
		t.Fatalf("targetInfos length = %d, want 1", len(res.TargetInfos))
	}
	if res.TargetInfos[0]["targetId"] != "t1" {
		t.Errorf("targetId = %v, want t1", res.TargetInfos[0]["targetId"])
	}
}

func TestHandleTarget_AttachToTarget_Existing(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID: "existing-s1",
		TargetID:  "t1",
		Type:      "page",
	})

	msg := &cdp.Message{
		ID:     1,
		Method: "Target.attachToTarget",
		Params: json.RawMessage(`{"targetId":"t1","flatten":true}`),
	}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res map[string]string
	json.Unmarshal(result, &res)
	if res["sessionId"] != "existing-s1" {
		t.Errorf("sessionId = %q, want existing-s1", res["sessionId"])
	}
}

func TestHandleTarget_AttachToTarget_New(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Target.attachToTarget",
		Params: json.RawMessage(`{"targetId":"t-new","flatten":true}`),
	}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res map[string]string
	json.Unmarshal(result, &res)
	if res["sessionId"] == "" {
		t.Error("expected non-empty sessionId")
	}

	// Should be registered now
	info, ok := b.sessions.GetByTarget("t-new")
	if !ok {
		t.Fatal("session for target t-new not found")
	}
	if info.Type != "page" {
		t.Errorf("type = %q, want page", info.Type)
	}
}

func TestHandleTarget_ActivateTarget(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Target.activateTarget", Params: json.RawMessage(`{"targetId":"t1"}`)}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleTarget_GetBrowserContexts(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{SessionID: "s1", TargetID: "t1", BrowserContextID: "ctx-1"})
	b.sessions.Add(&cdp.SessionInfo{SessionID: "s2", TargetID: "t2", BrowserContextID: "ctx-2"})

	msg := &cdp.Message{ID: 1, Method: "Target.getBrowserContexts", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res struct {
		BrowserContextIDs      []string `json:"browserContextIds"`
		DefaultBrowserContextID string   `json:"defaultBrowserContextId"`
	}
	json.Unmarshal(result, &res)
	if len(res.BrowserContextIDs) < 1 {
		t.Fatal("expected at least one browser context")
	}
	if res.DefaultBrowserContextID == "" {
		t.Error("expected non-empty defaultBrowserContextId")
	}
}

func TestHandleTarget_GetBrowserContexts_Empty(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Target.getBrowserContexts", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res struct {
		BrowserContextIDs []string `json:"browserContextIds"`
	}
	json.Unmarshal(result, &res)
	if res.BrowserContextIDs == nil {
		t.Fatal("expected non-nil browserContextIds array")
	}
	if len(res.BrowserContextIDs) != 0 {
		t.Errorf("browserContextIds length = %d, want 0", len(res.BrowserContextIDs))
	}
}

func TestHandleTarget_GetTargetInfo_Found(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		TargetID:         "t1",
		Type:             "page",
		Title:            "Found Page",
		URL:              "https://example.com",
		BrowserContextID: "ctx-1",
	})

	msg := &cdp.Message{ID: 1, Method: "Target.getTargetInfo", Params: json.RawMessage(`{"targetId":"t1"}`)}
	result, cdpErr := b.handleTarget(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res struct {
		TargetInfo struct {
			TargetID string `json:"targetId"`
			Type     string `json:"type"`
			Title    string `json:"title"`
		} `json:"targetInfo"`
	}
	json.Unmarshal(result, &res)
	if res.TargetInfo.TargetID != "t1" {
		t.Errorf("targetId = %q, want t1", res.TargetInfo.TargetID)
	}
	if res.TargetInfo.Title != "Found Page" {
		t.Errorf("title = %q, want Found Page", res.TargetInfo.Title)
	}
}

func TestHandleTarget_GetTargetInfo_NotFound(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Target.getTargetInfo", Params: json.RawMessage(`{"targetId":"nonexistent"}`)}
	_, cdpErr := b.handleTarget(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for nonexistent target")
	}
	if cdpErr.Code != -32000 {
		t.Errorf("error code = %d, want -32000", cdpErr.Code)
	}
}

func TestHandleTarget_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Target.doesNotExist", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleTarget(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}

func TestHandleTarget_CreateTarget_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Target.createTarget", Params: json.RawMessage(`not-json`)}
	_, cdpErr := b.handleTarget(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}
