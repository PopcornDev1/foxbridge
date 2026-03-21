package bridge

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func TestHandleDOM_EnableDisable(t *testing.T) {
	for _, method := range []string{"DOM.enable", "DOM.disable"} {
		t.Run(method, func(t *testing.T) {
			b, _ := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: method, Params: json.RawMessage(`{}`)}
			result, cdpErr := b.handleDOM(nil, msg)
			if cdpErr != nil {
				t.Fatalf("error: %s", cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
		})
	}
}

func TestHandleDOM_GetDocument_Fallback(t *testing.T) {
	b, mb := newTestBridge()
	// Make Runtime.evaluate fail so we get the fallback document
	mb.SetResponse("", "Runtime.evaluate", nil, fmt.Errorf("eval failed"))

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.getDocument",
		Params: json.RawMessage(`{}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Root struct {
			NodeID   int    `json:"nodeId"`
			NodeType int    `json:"nodeType"`
			NodeName string `json:"nodeName"`
			Children []struct {
				NodeName string `json:"nodeName"`
			} `json:"children"`
		} `json:"root"`
	}
	if err := json.Unmarshal(result, &res); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if res.Root.NodeType != 9 {
		t.Errorf("root nodeType = %d, want 9 (document)", res.Root.NodeType)
	}
	if res.Root.NodeName != "#document" {
		t.Errorf("root nodeName = %q, want #document", res.Root.NodeName)
	}
	if len(res.Root.Children) != 1 {
		t.Fatalf("root children len = %d, want 1", len(res.Root.Children))
	}
	if res.Root.Children[0].NodeName != "HTML" {
		t.Errorf("child nodeName = %q, want HTML", res.Root.Children[0].NodeName)
	}
}

func TestHandleDOM_GetDocument_WithEval(t *testing.T) {
	b, mb := newTestBridge()

	// Runtime.evaluate returns a JSON-stringified value
	evalResult := `{"result":{"value":"{\"title\":\"Test\",\"url\":\"https://example.com\",\"baseURL\":\"https://example.com/\"}"}}`
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(evalResult), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.getDocument",
		Params: json.RawMessage(`{}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Root struct {
			DocumentURL string `json:"documentURL"`
			BaseURL     string `json:"baseURL"`
		} `json:"root"`
	}
	json.Unmarshal(result, &res)

	if res.Root.DocumentURL != "https://example.com" {
		t.Errorf("documentURL = %q, want https://example.com", res.Root.DocumentURL)
	}
	if res.Root.BaseURL != "https://example.com/" {
		t.Errorf("baseURL = %q, want https://example.com/", res.Root.BaseURL)
	}
}

func TestHandleDOM_QuerySelector_Found(t *testing.T) {
	b, mb := newTestBridge()

	evalResult := `{"result":{"value":true}}`
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(evalResult), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.querySelector",
		Params: json.RawMessage(`{"nodeId":1,"selector":"#main"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		NodeID int `json:"nodeId"`
	}
	json.Unmarshal(result, &res)

	if res.NodeID == 0 {
		t.Error("nodeId should be non-zero when element is found")
	}
}

func TestHandleDOM_QuerySelector_NotFound(t *testing.T) {
	b, mb := newTestBridge()

	evalResult := `{"result":{"value":false}}`
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(evalResult), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.querySelector",
		Params: json.RawMessage(`{"nodeId":1,"selector":"#nonexistent"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		NodeID int `json:"nodeId"`
	}
	json.Unmarshal(result, &res)

	if res.NodeID != 0 {
		t.Errorf("nodeId = %d, want 0 for not found", res.NodeID)
	}
}

func TestHandleDOM_QuerySelectorAll(t *testing.T) {
	b, mb := newTestBridge()

	evalResult := `{"result":{"value":3}}`
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(evalResult), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.querySelectorAll",
		Params: json.RawMessage(`{"nodeId":1,"selector":".item"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		NodeIDs []int `json:"nodeIds"`
	}
	json.Unmarshal(result, &res)

	if len(res.NodeIDs) != 3 {
		t.Fatalf("nodeIds len = %d, want 3", len(res.NodeIDs))
	}
	// IDs should start from 3
	for i, id := range res.NodeIDs {
		if id != 3+i {
			t.Errorf("nodeIds[%d] = %d, want %d", i, id, 3+i)
		}
	}
}

func TestHandleDOM_ResolveNode(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.resolveNode",
		Params: json.RawMessage(`{"nodeId":5}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Object struct {
			Type     string `json:"type"`
			Subtype  string `json:"subtype"`
			ObjectID string `json:"objectId"`
		} `json:"object"`
	}
	json.Unmarshal(result, &res)

	if res.Object.Type != "object" {
		t.Errorf("type = %q, want object", res.Object.Type)
	}
	if res.Object.Subtype != "node" {
		t.Errorf("subtype = %q, want node", res.Object.Subtype)
	}
	if res.Object.ObjectID != "node-5" {
		t.Errorf("objectId = %q, want node-5", res.Object.ObjectID)
	}
}

func TestHandleDOM_GetBoxModel_Fallback(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.getBoxModel",
		Params: json.RawMessage(`{"nodeId":1}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Model struct {
			Width  int       `json:"width"`
			Height int       `json:"height"`
			Content []float64 `json:"content"`
		} `json:"model"`
	}
	json.Unmarshal(result, &res)

	if res.Model.Width != 100 || res.Model.Height != 100 {
		t.Errorf("fallback box model size = %dx%d, want 100x100", res.Model.Width, res.Model.Height)
	}
	if len(res.Model.Content) != 8 {
		t.Errorf("content quad len = %d, want 8", len(res.Model.Content))
	}
}

func TestHandleDOM_GetAttributes(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.getAttributes",
		Params: json.RawMessage(`{"nodeId":1}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Attributes []string `json:"attributes"`
	}
	json.Unmarshal(result, &res)

	if res.Attributes == nil {
		t.Error("attributes should not be nil")
	}
	if len(res.Attributes) != 0 {
		t.Errorf("attributes len = %d, want 0", len(res.Attributes))
	}
}

func TestHandleDOM_DescribeNode_Fallback(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.describeNode",
		Params: json.RawMessage(`{"nodeId":5,"backendNodeId":5}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Node struct {
			NodeID   int    `json:"nodeId"`
			NodeName string `json:"nodeName"`
			NodeType int    `json:"nodeType"`
		} `json:"node"`
	}
	json.Unmarshal(result, &res)

	if res.Node.NodeID != 5 {
		t.Errorf("nodeId = %d, want 5", res.Node.NodeID)
	}
	if res.Node.NodeName != "DIV" {
		t.Errorf("nodeName = %q, want DIV", res.Node.NodeName)
	}
	if res.Node.NodeType != 1 {
		t.Errorf("nodeType = %d, want 1", res.Node.NodeType)
	}
}

func TestHandleDOM_NoopMethods(t *testing.T) {
	noops := []string{
		"DOM.removeNode",
		"DOM.setAttributeValue",
		"DOM.setNodeValue",
		"DOM.setOuterHTML",
	}

	for _, method := range noops {
		t.Run(method, func(t *testing.T) {
			b, _ := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: method, Params: json.RawMessage(`{}`)}
			result, cdpErr := b.handleDOM(nil, msg)
			if cdpErr != nil {
				t.Errorf("error: %s", cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
		})
	}
}

func TestHandleDOM_GetContentQuads_Fallback(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.getContentQuads",
		Params: json.RawMessage(`{"nodeId":1}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Quads [][]float64 `json:"quads"`
	}
	json.Unmarshal(result, &res)

	if len(res.Quads) != 1 {
		t.Fatalf("quads len = %d, want 1", len(res.Quads))
	}
	if len(res.Quads[0]) != 8 {
		t.Errorf("quad points = %d, want 8", len(res.Quads[0]))
	}
}

func TestHandleDOM_UnknownMethod(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{ID: 1, Method: "DOM.doesNotExist", Params: json.RawMessage(`{}`)}
	_, cdpErr := b.handleDOM(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown DOM method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}
