package bridge

import (
	"encoding/json"
	"testing"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func TestHandleAccessibility_Enable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Accessibility.enable", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleAccessibility(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleAccessibility_Disable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Accessibility.disable", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleAccessibility(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleAccessibility_GetFullAXTree(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Accessibility.getFullAXTree",
		json.RawMessage(`{"nodes":[{"nodeId":"1","role":{"value":"document"}}]}`), nil)

	msg := &cdp.Message{ID: 1, Method: "Accessibility.getFullAXTree", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleAccessibility(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	var res struct {
		Nodes []map[string]interface{} `json:"nodes"`
	}
	json.Unmarshal(result, &res)
	if len(res.Nodes) != 1 {
		t.Errorf("nodes length = %d, want 1", len(res.Nodes))
	}
}

func TestHandleAccessibility_GetFullAXTree_SessionResolution(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "cdp-s1",
		JugglerSessionID: "jug-s1",
		TargetID:         "t1",
	})
	mb.SetResponse("jug-s1", "Accessibility.getFullAXTree", json.RawMessage(`{"nodes":[]}`), nil)

	msg := &cdp.Message{ID: 1, Method: "Accessibility.getFullAXTree", SessionID: "cdp-s1", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleAccessibility(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	if last.SessionID != "jug-s1" {
		t.Errorf("sessionID = %q, want jug-s1", last.SessionID)
	}
}

func TestHandleAccessibility_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Accessibility.doesNotExist", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleAccessibility(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}
