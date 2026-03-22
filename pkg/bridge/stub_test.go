package bridge

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func TestHandleStub_BrowserGetVersion_Success(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Browser.getInfo",
		json.RawMessage(`{"version":"Firefox/146.0.1","userAgent":"Mozilla/5.0 Gecko"}`), nil)

	msg := &cdp.Message{ID: 1, Method: "Browser.getVersion"}
	result, cdpErr := b.handleStub(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res map[string]string
	json.Unmarshal(result, &res)
	if res["protocolVersion"] != "1.3" {
		t.Errorf("protocolVersion = %q, want 1.3", res["protocolVersion"])
	}
	if res["userAgent"] != "Mozilla/5.0 Gecko" {
		t.Errorf("userAgent = %q, want Mozilla/5.0 Gecko", res["userAgent"])
	}
	if res["product"] != "foxbridge (Firefox/146.0.1)" {
		t.Errorf("product = %q, want foxbridge (Firefox/146.0.1)", res["product"])
	}
}

func TestHandleStub_BrowserGetVersion_Fallback(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Browser.getInfo", nil, fmt.Errorf("connection refused"))

	msg := &cdp.Message{ID: 1, Method: "Browser.getVersion"}
	result, cdpErr := b.handleStub(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res map[string]string
	json.Unmarshal(result, &res)
	if res["protocolVersion"] != "1.3" {
		t.Errorf("protocolVersion = %q, want 1.3", res["protocolVersion"])
	}
	if res["product"] != "foxbridge (Firefox/Camoufox)" {
		t.Errorf("product = %q, want foxbridge (Firefox/Camoufox)", res["product"])
	}
}

func TestHandleStub_BrowserClose(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Browser.close", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{ID: 1, Method: "Browser.close"}
	result, cdpErr := b.handleStub(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	last, _ := mb.LastCall()
	if last.Method != "Browser.close" {
		t.Errorf("method = %q, want Browser.close", last.Method)
	}
}

func TestHandleStub_BrowserClose_Error(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Browser.close", nil, fmt.Errorf("failed"))

	msg := &cdp.Message{ID: 1, Method: "Browser.close"}
	_, cdpErr := b.handleStub(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error")
	}
	if cdpErr.Code != -32000 {
		t.Errorf("error code = %d, want -32000", cdpErr.Code)
	}
}

func TestHandleStub_GetWindowForTarget(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Browser.getWindowForTarget", Params: json.RawMessage(`{"targetId":"t1"}`)}
	result, cdpErr := b.handleStub(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res struct {
		WindowID int `json:"windowId"`
		Bounds   struct {
			Width  int    `json:"width"`
			Height int    `json:"height"`
			State  string `json:"windowState"`
		} `json:"bounds"`
	}
	json.Unmarshal(result, &res)
	if res.WindowID != 1 {
		t.Errorf("windowId = %d, want 1", res.WindowID)
	}
	if res.Bounds.Width != 1280 {
		t.Errorf("width = %d, want 1280", res.Bounds.Width)
	}
	if res.Bounds.Height != 720 {
		t.Errorf("height = %d, want 720", res.Bounds.Height)
	}
	if res.Bounds.State != "normal" {
		t.Errorf("windowState = %q, want normal", res.Bounds.State)
	}
}

func TestHandleStub_SetWindowBounds(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Browser.setWindowBounds", Params: json.RawMessage(`{"windowId":1,"bounds":{"width":800}}`)}
	result, cdpErr := b.handleStub(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleStub_SystemInfoGetProcessInfo(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "SystemInfo.getProcessInfo"}
	result, cdpErr := b.handleStub(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res struct {
		ProcessInfo []interface{} `json:"processInfo"`
	}
	json.Unmarshal(result, &res)
	if res.ProcessInfo == nil {
		t.Error("expected non-nil processInfo")
	}
}

func TestHandleStub_StubDomains(t *testing.T) {
	domains := []string{
		"Debugger.enable",
		"Debugger.disable",
		"Debugger.setBreakpoint",
		"Profiler.enable",
		"Profiler.start",
		"Performance.enable",
		"HeapProfiler.enable",
		"Memory.getDOMCounters",
		"ServiceWorker.enable",
		"CacheStorage.requestEntries",
		"IndexedDB.enable",
		"Log.enable",
		"Security.enable",
		"Fetch.enable",
		"CSS.enable",
		"Overlay.enable",
		"DOMStorage.enable",
		"WebAuthn.enable",
		"Media.enable",
		"Audits.enable",
		"Inspector.enable",
		"Database.enable",
		"BackgroundService.enable",
		"Cast.enable",
		"DeviceAccess.enable",
	}

	for _, method := range domains {
		t.Run(method, func(t *testing.T) {
			b, _ := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: method, Params: json.RawMessage(`{}`)}
			result, cdpErr := b.handleStub(nil, msg)
			if cdpErr != nil {
				t.Errorf("unexpected error for %s: %s", method, cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
		})
	}
}

func TestHandleStub_EnableDisableSuffix(t *testing.T) {
	tests := []struct {
		method string
	}{
		{"SomeNewDomain.enable"},
		{"AnotherDomain.disable"},
	}

	for _, tt := range tests {
		t.Run(tt.method, func(t *testing.T) {
			b, _ := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: tt.method, Params: json.RawMessage(`{}`)}
			result, cdpErr := b.handleStub(nil, msg)
			if cdpErr != nil {
				t.Errorf("unexpected error for %s: %s", tt.method, cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
		})
	}
}

func TestHandleStub_RuntimeRunIfWaitingForDebugger(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Runtime.runIfWaitingForDebugger", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleStub(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleStub_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "UnknownDomain.unknownMethod", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleStub(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}
