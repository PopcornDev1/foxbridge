package bridge

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func TestHandleDOMStorage_Enable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "DOMStorage.enable"}
	result, cdpErr := b.handleDOMStorage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleDOMStorage_Disable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "DOMStorage.disable"}
	result, cdpErr := b.handleDOMStorage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleDOMStorage_GetItems(t *testing.T) {
	b, mb := newTestBridge()

	evalResult := `{"result":{"value":"[[\"key1\",\"val1\"],[\"key2\",\"val2\"]]"}}`
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(evalResult), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOMStorage.getDOMStorageItems",
		Params: json.RawMessage(`{}`),
	}

	result, cdpErr := b.handleDOMStorage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Entries [][]string `json:"entries"`
	}
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(res.Entries) != 2 {
		t.Fatalf("entries length = %d, want 2", len(res.Entries))
	}
	if res.Entries[0][0] != "key1" || res.Entries[0][1] != "val1" {
		t.Errorf("first entry = %v, want [key1, val1]", res.Entries[0])
	}
}

func TestHandleDOMStorage_GetItems_Error(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.evaluate", nil, fmt.Errorf("no page"))

	msg := &cdp.Message{
		ID:     1,
		Method: "DOMStorage.getDOMStorageItems",
	}

	_, cdpErr := b.handleDOMStorage(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error")
	}
	if cdpErr.Code != -32000 {
		t.Errorf("error code = %d, want -32000", cdpErr.Code)
	}
}

func TestHandleDOMStorage_SetItem_LocalStorage(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(`{"result":{}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOMStorage.setDOMStorageItem",
		Params: json.RawMessage(`{"storageId":{"isLocalStorage":true},"key":"myKey","value":"myVal"}`),
	}

	result, cdpErr := b.handleDOMStorage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Runtime.evaluate")
	if len(calls) == 0 {
		t.Fatal("expected Runtime.evaluate call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	expr, _ := params["expression"].(string)
	if expr == "" {
		t.Fatal("expression not found")
	}
	// Should use localStorage
	if len(expr) < 12 || expr[:12] != "localStorage" {
		t.Errorf("expression should start with localStorage, got: %s", expr)
	}
}

func TestHandleDOMStorage_SetItem_SessionStorage(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(`{"result":{}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOMStorage.setDOMStorageItem",
		Params: json.RawMessage(`{"storageId":{"isLocalStorage":false},"key":"k","value":"v"}`),
	}

	result, cdpErr := b.handleDOMStorage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Runtime.evaluate")
	if len(calls) == 0 {
		t.Fatal("expected Runtime.evaluate call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	expr, _ := params["expression"].(string)
	if len(expr) < 14 || expr[:14] != "sessionStorage" {
		t.Errorf("expression should start with sessionStorage, got: %s", expr)
	}
}

func TestHandleDOMStorage_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "DOMStorage.removeDOMStorageItem"}
	result, cdpErr := b.handleDOMStorage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}
