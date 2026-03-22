package bridge

import (
	"encoding/json"
	"testing"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func TestHandleConsole_Enable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Console.enable", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleConsole(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleConsole_Disable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Console.disable", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handleConsole(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandleConsole_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Console.clearMessages", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleConsole(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}
