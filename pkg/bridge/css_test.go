package bridge

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func TestHandleCSS_Enable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "CSS.enable"}
	result, cdpErr := b.handleCSS(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleCSS_Disable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "CSS.disable"}
	result, cdpErr := b.handleCSS(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleCSS_GetComputedStyleForNode(t *testing.T) {
	b, mb := newTestBridge()

	styleJSON := `{"computedStyle":[{"name":"color","value":"rgb(0, 0, 0)"},{"name":"display","value":"block"}]}`
	evalResult := fmt.Sprintf(`{"result":{"value":%q}}`, styleJSON)
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(evalResult), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "CSS.getComputedStyleForNode",
		Params: json.RawMessage(`{"nodeId":5}`),
	}

	result, cdpErr := b.handleCSS(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		ComputedStyle []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"computedStyle"`
	}
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("failed to parse result: %v", err)
	}
	if len(res.ComputedStyle) != 2 {
		t.Fatalf("computedStyle length = %d, want 2", len(res.ComputedStyle))
	}
	if res.ComputedStyle[0].Name != "color" {
		t.Errorf("first prop name = %q, want color", res.ComputedStyle[0].Name)
	}
	if res.ComputedStyle[1].Value != "block" {
		t.Errorf("second prop value = %q, want block", res.ComputedStyle[1].Value)
	}
}

func TestHandleCSS_GetComputedStyleForNode_EvalError(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.evaluate", nil, fmt.Errorf("eval failed"))

	msg := &cdp.Message{
		ID:     1,
		Method: "CSS.getComputedStyleForNode",
		Params: json.RawMessage(`{"nodeId":0}`),
	}

	_, cdpErr := b.handleCSS(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error")
	}
	if cdpErr.Code != -32000 {
		t.Errorf("error code = %d, want -32000", cdpErr.Code)
	}
}

func TestHandleCSS_GetComputedStyleForNode_EmptyResult(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(`{"result":{"value":""}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "CSS.getComputedStyleForNode",
		Params: json.RawMessage(`{"nodeId":99}`),
	}

	result, cdpErr := b.handleCSS(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		ComputedStyle []interface{} `json:"computedStyle"`
	}
	json.Unmarshal(result, &res)
	if len(res.ComputedStyle) != 0 {
		t.Errorf("computedStyle length = %d, want 0", len(res.ComputedStyle))
	}
}

func TestHandleCSS_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "CSS.getMatchedStylesForNode"}
	result, cdpErr := b.handleCSS(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}
