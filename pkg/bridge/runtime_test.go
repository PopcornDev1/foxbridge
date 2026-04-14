package bridge

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func TestHandleRuntime_Enable(t *testing.T) {
	b, _ := newTestBridge()
	// Add a session with a frameID so executionContextCreated is emitted
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		JugglerSessionID: "jug-1",
		TargetID:         "t1",
		FrameID:          "frame-1",
	})

	msg := &cdp.Message{ID: 1, Method: "Runtime.enable", Params: json.RawMessage(`{}`), SessionID: "s1"}
	result, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// The goroutine emitting executionContextCreated runs async — we just verify no error.
}

func TestHandleRuntime_Evaluate_Basic(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(`{"result":{"type":"number","value":42}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Runtime.evaluate",
		Params: json.RawMessage(`{"expression":"1+1","returnByValue":true}`),
	}
	result, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	var res map[string]interface{}
	json.Unmarshal(result, &res)
	if res["result"] == nil {
		t.Error("expected result field in response")
	}

	last, _ := mb.LastCall()
	if last.Method != "Runtime.evaluate" {
		t.Errorf("method = %q, want Runtime.evaluate", last.Method)
	}
	var p map[string]interface{}
	json.Unmarshal(last.Params, &p)
	if p["expression"] != "1+1" {
		t.Errorf("expression = %v, want 1+1", p["expression"])
	}
	if p["returnByValue"] != true {
		t.Errorf("returnByValue = %v, want true", p["returnByValue"])
	}
}

func TestHandleRuntime_Evaluate_ContextIDMapping(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(`{}`), nil)

	// Pre-populate a context mapping
	b.ctxMapMu.Lock()
	b.ctxMap[101] = "juggler-ctx-abc"
	b.ctxMapMu.Unlock()

	msg := &cdp.Message{
		ID:     1,
		Method: "Runtime.evaluate",
		Params: json.RawMessage(`{"expression":"x","contextId":101}`),
	}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var p map[string]interface{}
	json.Unmarshal(last.Params, &p)
	if p["executionContextId"] != "juggler-ctx-abc" {
		t.Errorf("executionContextId = %v, want juggler-ctx-abc", p["executionContextId"])
	}
}

func TestHandleRuntime_Evaluate_AwaitPromise(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Runtime.evaluate",
		Params: json.RawMessage(`{"expression":"fetch('/api')","awaitPromise":true}`),
	}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var p map[string]interface{}
	json.Unmarshal(last.Params, &p)
	expr, _ := p["expression"].(string)
	if expr != `(async () => { return await (fetch('/api')) })()` {
		t.Errorf("expression = %q, want async-wrapped", expr)
	}
}

func TestHandleRuntime_CallFunctionOn(t *testing.T) {
	tests := []struct {
		name       string
		params     string
		checkKey   string
		checkValue interface{}
	}{
		{
			name:       "basic function",
			params:     `{"functionDeclaration":"function(){return 1}","objectId":"obj-1","returnByValue":true}`,
			checkKey:   "returnByValue",
			checkValue: true,
		},
		{
			name:       "with arguments",
			params:     `{"functionDeclaration":"function(a){return a}","arguments":[{"value":42}],"returnByValue":false}`,
			checkKey:   "functionDeclaration",
			checkValue: "function(a){return a}",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, mb := newTestBridge()
			mb.SetResponse("", "Runtime.callFunction", json.RawMessage(`{"result":{"type":"number"}}`), nil)
			msg := &cdp.Message{ID: 1, Method: "Runtime.callFunctionOn", Params: json.RawMessage(tt.params)}
			result, cdpErr := b.handleRuntime(nil, msg)
			if cdpErr != nil {
				t.Fatalf("unexpected error: %s", cdpErr.Message)
			}
			if result == nil {
				t.Fatal("expected non-nil result")
			}
			last, _ := mb.LastCall()
			if last.Method != "Runtime.callFunction" {
				t.Errorf("method = %q, want Runtime.callFunction", last.Method)
			}
			var p map[string]interface{}
			json.Unmarshal(last.Params, &p)
			if p[tt.checkKey] != tt.checkValue {
				t.Errorf("%s = %v, want %v", tt.checkKey, p[tt.checkKey], tt.checkValue)
			}
		})
	}
}

func TestHandleRuntime_CallFunctionOn_AwaitPromise(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.callFunction", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Runtime.callFunctionOn",
		Params: json.RawMessage(`{"functionDeclaration":"function(){return fetch('/')}","awaitPromise":true}`),
	}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var p map[string]interface{}
	json.Unmarshal(last.Params, &p)
	funcDecl, _ := p["functionDeclaration"].(string)
	if !strings.Contains(funcDecl, "function(){return fetch('/')}") {
		t.Errorf("functionDeclaration = %q, want async-wrapped", funcDecl)
	}
}

func TestHandleRuntime_CallFunctionOn_ExecutionContextIDMapping(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.callFunction", json.RawMessage(`{}`), nil)

	b.ctxMapMu.Lock()
	b.ctxMap[200] = "juggler-ctx-xyz"
	b.ctxMapMu.Unlock()

	msg := &cdp.Message{
		ID:     1,
		Method: "Runtime.callFunctionOn",
		Params: json.RawMessage(`{"functionDeclaration":"function(){}","executionContextId":200}`),
	}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var p map[string]interface{}
	json.Unmarshal(last.Params, &p)
	if p["executionContextId"] != "juggler-ctx-xyz" {
		t.Errorf("executionContextId = %v, want juggler-ctx-xyz", p["executionContextId"])
	}
}

func TestHandleRuntime_ReleaseObject(t *testing.T) {
	b, mb := newTestBridge()
	msg := &cdp.Message{
		ID:     1,
		Method: "Runtime.releaseObject",
		Params: json.RawMessage(`{"objectId":"obj-42"}`),
	}
	result, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	last, _ := mb.LastCall()
	if last.Method != "Runtime.disposeObject" {
		t.Errorf("method = %q, want Runtime.disposeObject", last.Method)
	}
	var p map[string]string
	json.Unmarshal(last.Params, &p)
	if p["objectId"] != "obj-42" {
		t.Errorf("objectId = %q, want obj-42", p["objectId"])
	}
}

func TestHandleRuntime_GetProperties(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.getObjectProperties", json.RawMessage(`{"properties":[{"name":"x","value":{"type":"number"}}]}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Runtime.getProperties",
		Params: json.RawMessage(`{"objectId":"obj-99","ownProperties":true}`),
	}
	result, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if result == nil {
		t.Fatal("expected non-nil result")
	}
	last, _ := mb.LastCall()
	if last.Method != "Runtime.getObjectProperties" {
		t.Errorf("method = %q, want Runtime.getObjectProperties", last.Method)
	}
	var p map[string]string
	json.Unmarshal(last.Params, &p)
	if p["objectId"] != "obj-99" {
		t.Errorf("objectId = %q, want obj-99", p["objectId"])
	}
}

func TestHandleRuntime_NoOps(t *testing.T) {
	noops := []string{
		"Runtime.releaseObjectGroup",
		"Runtime.runIfWaitingForDebugger",
		"Runtime.addBinding",
		"Runtime.discardConsoleEntries",
	}
	for _, method := range noops {
		t.Run(method, func(t *testing.T) {
			b, _ := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: method, Params: json.RawMessage(`{}`)}
			result, cdpErr := b.handleRuntime(nil, msg)
			if cdpErr != nil {
				t.Fatalf("unexpected error for %s: %s", method, cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
		})
	}
}

func TestHandleRuntime_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Runtime.doesNotExist", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}

func TestHandleRuntime_Evaluate_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Runtime.evaluate", Params: json.RawMessage(`not-json`)}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestHandleRuntime_CallFunctionOn_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Runtime.callFunctionOn", Params: json.RawMessage(`not-json`)}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestHandleRuntime_ReleaseObject_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Runtime.releaseObject", Params: json.RawMessage(`not-json`)}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestHandleRuntime_GetProperties_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Runtime.getProperties", Params: json.RawMessage(`not-json`)}
	_, cdpErr := b.handleRuntime(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}
