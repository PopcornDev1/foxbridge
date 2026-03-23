package bridge

import (
	"encoding/json"
	"testing"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func TestHandleInput_DispatchMouseEvent(t *testing.T) {
	tests := []struct {
		name       string
		params     string
		wantMethod string
		checkParam string
		checkValue interface{}
	}{
		{
			name:       "basic click",
			params:     `{"type":"mousedown","x":100,"y":200,"button":"left","clickCount":1}`,
			wantMethod: "Page.dispatchMouseEvent",
			checkParam: "type",
			checkValue: "mousedown",
		},
		{
			name:       "with modifiers",
			params:     `{"type":"mousedown","x":10,"y":20,"button":"right","clickCount":2,"modifiers":8}`,
			wantMethod: "Page.dispatchMouseEvent",
			checkParam: "modifiers",
			checkValue: float64(8),
		},
		{
			name:       "mouseMoved no button",
			params:     `{"type":"mouseMoved","x":50,"y":60}`,
			wantMethod: "Page.dispatchMouseEvent",
			checkParam: "x",
			checkValue: float64(50),
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, mb := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: "Input.dispatchMouseEvent", Params: json.RawMessage(tt.params)}
			result, cdpErr := b.handleInput(nil, msg)
			if cdpErr != nil {
				t.Fatalf("unexpected error: %s", cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
			last, err := mb.LastCall()
			if err != nil {
				t.Fatal(err)
			}
			if last.Method != tt.wantMethod {
				t.Errorf("method = %q, want %q", last.Method, tt.wantMethod)
			}
			var p map[string]interface{}
			json.Unmarshal(last.Params, &p)
			if p[tt.checkParam] != tt.checkValue {
				t.Errorf("%s = %v, want %v", tt.checkParam, p[tt.checkParam], tt.checkValue)
			}
		})
	}
}

func TestHandleInput_DispatchMouseEvent_AllFields(t *testing.T) {
	b, mb := newTestBridge()
	msg := &cdp.Message{
		ID:     1,
		Method: "Input.dispatchMouseEvent",
		Params: json.RawMessage(`{"type":"mousedown","x":100.5,"y":200.5,"button":"left","clickCount":3,"modifiers":12,"deltaX":5.5,"deltaY":-3.2}`),
	}
	result, cdpErr := b.handleInput(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	last, _ := mb.LastCall()
	var p map[string]interface{}
	json.Unmarshal(last.Params, &p)

	checks := map[string]interface{}{
		"type":       "mousedown",
		"x":          100.5,
		"y":          200.5,
		"button":     float64(0),
		"clickCount": float64(3),
		"modifiers":  float64(12),
		"deltaX":     5.5,
		"deltaY":     -3.2,
	}
	for k, want := range checks {
		if p[k] != want {
			t.Errorf("%s = %v, want %v", k, p[k], want)
		}
	}
}

func TestHandleInput_MouseWheel(t *testing.T) {
	b, mb := newTestBridge()
	msg := &cdp.Message{
		ID:     1,
		Method: "Input.dispatchMouseEvent",
		Params: json.RawMessage(`{"type":"mouseWheel","x":0,"y":0,"deltaX":10,"deltaY":-20}`),
	}
	result, cdpErr := b.handleInput(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	last, _ := mb.LastCall()
	var p map[string]interface{}
	json.Unmarshal(last.Params, &p)
	if p["deltaX"] != float64(10) {
		t.Errorf("deltaX = %v, want 10", p["deltaX"])
	}
	if p["deltaY"] != float64(-20) {
		t.Errorf("deltaY = %v, want -20", p["deltaY"])
	}
}

func TestHandleInput_DispatchKeyEvent(t *testing.T) {
	tests := []struct {
		name   string
		params string
		checks map[string]interface{}
	}{
		{
			name:   "keyDown with all fields",
			params: `{"type":"keyDown","key":"a","code":"KeyA","text":"a","keyIdentifier":"U+0041","modifiers":2,"windowsVirtualKeyCode":65}`,
			checks: map[string]interface{}{
				"type":    "keydown",
				"key":     "a",
				"code":    "KeyA",
				"text":    "a",
				"keyCode": float64(65),
			},
		},
		{
			name:   "keyUp minimal",
			params: `{"type":"keyUp","key":"Enter"}`,
			checks: map[string]interface{}{
				"type": "keyup",
				"key":  "Enter",
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			b, mb := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: "Input.dispatchKeyEvent", Params: json.RawMessage(tt.params)}
			result, cdpErr := b.handleInput(nil, msg)
			if cdpErr != nil {
				t.Fatalf("unexpected error: %s", cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
			last, _ := mb.LastCall()
			if last.Method != "Page.dispatchKeyEvent" {
				t.Errorf("method = %q, want Page.dispatchKeyEvent", last.Method)
			}
			var p map[string]interface{}
			json.Unmarshal(last.Params, &p)
			for k, want := range tt.checks {
				if p[k] != want {
					t.Errorf("%s = %v, want %v", k, p[k], want)
				}
			}
		})
	}
}

func TestHandleInput_InsertText(t *testing.T) {
	b, mb := newTestBridge()
	msg := &cdp.Message{
		ID:     1,
		Method: "Input.insertText",
		Params: json.RawMessage(`{"text":"hello world"}`),
	}
	result, cdpErr := b.handleInput(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	last, _ := mb.LastCall()
	if last.Method != "Page.insertText" {
		t.Errorf("method = %q, want Page.insertText", last.Method)
	}
	var p map[string]string
	json.Unmarshal(last.Params, &p)
	if p["text"] != "hello world" {
		t.Errorf("text = %q, want %q", p["text"], "hello world")
	}
}

func TestHandleInput_DispatchTouchEvent(t *testing.T) {
	b, mb := newTestBridge()
	params := `{"type":"touchStart","touchPoints":[{"x":100,"y":200}]}`
	msg := &cdp.Message{
		ID:     1,
		Method: "Input.dispatchTouchEvent",
		Params: json.RawMessage(params),
	}
	result, cdpErr := b.handleInput(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	last, _ := mb.LastCall()
	if last.Method != "Page.dispatchTouchEvent" {
		t.Errorf("method = %q, want Page.dispatchTouchEvent", last.Method)
	}
	// Params should be passed through as-is
	if string(last.Params) != params {
		t.Errorf("params = %s, want %s", string(last.Params), params)
	}
}

func TestHandleInput_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Input.doesNotExist", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleInput(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}

func TestHandleInput_InvalidParams(t *testing.T) {
	methods := []string{"Input.dispatchMouseEvent", "Input.dispatchKeyEvent", "Input.insertText"}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			b, _ := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: method, Params: json.RawMessage(`not-json`)}
			_, cdpErr := b.handleInput(nil, msg)
			if cdpErr == nil {
				t.Fatal("expected error for invalid params")
			}
			if cdpErr.Code != -32602 {
				t.Errorf("error code = %d, want -32602", cdpErr.Code)
			}
		})
	}
}

func TestHandleInput_MouseEvent_OptionalFieldsOmitted(t *testing.T) {
	b, mb := newTestBridge()
	msg := &cdp.Message{
		ID:     1,
		Method: "Input.dispatchMouseEvent",
		Params: json.RawMessage(`{"type":"mouseMoved","x":50,"y":60}`),
	}
	_, cdpErr := b.handleInput(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	last, _ := mb.LastCall()
	var p map[string]interface{}
	json.Unmarshal(last.Params, &p)
	// button, clickCount, deltaX, deltaY should not be present (modifiers is always sent)
	for _, key := range []string{"button", "clickCount", "deltaX", "deltaY"} {
		if _, ok := p[key]; ok {
			t.Errorf("unexpected key %q in params", key)
		}
	}
}
