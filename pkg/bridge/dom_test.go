package bridge

import (
	"encoding/json"
	"fmt"
	"testing"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
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
			Width   int       `json:"width"`
			Height  int       `json:"height"`
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

func TestHandleDOM_Focus(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.callFunction", json.RawMessage(`{"result":{}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.focus",
		Params: json.RawMessage(`{"objectId":"node-5"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Runtime.callFunction")
	if len(calls) == 0 {
		t.Fatal("expected Runtime.callFunction call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["objectId"] != "node-5" {
		t.Errorf("objectId = %v, want node-5", params["objectId"])
	}
	decl, _ := params["functionDeclaration"].(string)
	if decl == "" {
		t.Error("functionDeclaration should not be empty")
	}
}

func TestHandleDOM_Focus_NoObjectID(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.focus",
		Params: json.RawMessage(`{"nodeId":3}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	// Should NOT call Runtime.callFunction without objectId
	calls := mb.CallsForMethod("Runtime.callFunction")
	if len(calls) != 0 {
		t.Errorf("expected no Runtime.callFunction call without objectId, got %d", len(calls))
	}
}

func TestHandleDOM_GetOuterHTML_WithObjectID(t *testing.T) {
	b, mb := newTestBridge()
	evalResult := `{"result":{"value":"<div>hello</div>"}}`
	mb.SetResponse("", "Runtime.callFunction", json.RawMessage(evalResult), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.getOuterHTML",
		Params: json.RawMessage(`{"objectId":"node-10"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		OuterHTML string `json:"outerHTML"`
	}
	json.Unmarshal(result, &res)
	if res.OuterHTML != "<div>hello</div>" {
		t.Errorf("outerHTML = %q, want <div>hello</div>", res.OuterHTML)
	}

	calls := mb.CallsForMethod("Runtime.callFunction")
	if len(calls) == 0 {
		t.Fatal("expected Runtime.callFunction call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["objectId"] != "node-10" {
		t.Errorf("objectId = %v, want node-10", params["objectId"])
	}
}

func TestHandleDOM_GetOuterHTML_NoObjectID(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.getOuterHTML",
		Params: json.RawMessage(`{"nodeId":1}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		OuterHTML string `json:"outerHTML"`
	}
	json.Unmarshal(result, &res)
	if res.OuterHTML != "" {
		t.Errorf("outerHTML = %q, want empty string for fallback", res.OuterHTML)
	}
}

func TestHandleDOM_ScrollIntoViewIfNeeded(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Runtime.callFunction", json.RawMessage(`{"result":{}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.scrollIntoViewIfNeeded",
		Params: json.RawMessage(`{"objectId":"node-7"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Runtime.callFunction")
	if len(calls) == 0 {
		t.Fatal("expected Runtime.callFunction call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["objectId"] != "node-7" {
		t.Errorf("objectId = %v, want node-7", params["objectId"])
	}
}

func TestHandleDOM_ScrollIntoViewIfNeeded_NoObjectID(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.scrollIntoViewIfNeeded",
		Params: json.RawMessage(`{"nodeId":3}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Runtime.callFunction")
	if len(calls) != 0 {
		t.Errorf("expected no Runtime.callFunction call without objectId, got %d", len(calls))
	}
}

func TestHandleDOM_SetFileInputFiles(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.setFileInputFiles", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.setFileInputFiles",
		Params: json.RawMessage(`{"files":["/tmp/a.txt","/tmp/b.txt"],"objectId":"node-12"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Page.setFileInputFiles")
	if len(calls) == 0 {
		t.Fatal("expected Page.setFileInputFiles call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["objectId"] != "node-12" {
		t.Errorf("objectId = %v, want node-12", params["objectId"])
	}
	files, ok := params["files"].([]interface{})
	if !ok {
		t.Fatal("files not found in params")
	}
	if len(files) != 2 {
		t.Fatalf("files len = %d, want 2", len(files))
	}
	if files[0] != "/tmp/a.txt" {
		t.Errorf("files[0] = %v, want /tmp/a.txt", files[0])
	}
}

func TestHandleDOM_SetFileInputFiles_NoObjectID(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.setFileInputFiles",
		Params: json.RawMessage(`{"files":["/tmp/a.txt"],"nodeId":5}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	// Without objectId, should not call Page.setFileInputFiles
	calls := mb.CallsForMethod("Page.setFileInputFiles")
	if len(calls) != 0 {
		t.Errorf("expected no Page.setFileInputFiles call without objectId, got %d", len(calls))
	}
}

func TestHandleDOM_SetFileInputFiles_Error(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.setFileInputFiles", nil, fmt.Errorf("file input failed"))

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.setFileInputFiles",
		Params: json.RawMessage(`{"files":["/tmp/a.txt"],"objectId":"node-1"}`),
	}

	_, cdpErr := b.handleDOM(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error when Page.setFileInputFiles fails")
	}
	if cdpErr.Code != -32000 {
		t.Errorf("error code = %d, want -32000", cdpErr.Code)
	}
}

func TestHandleDOM_DescribeNode_WithContentFrameId(t *testing.T) {
	b, mb := newTestBridge()
	// Page.describeNode returns contentFrameId for iframes
	mb.SetResponse("", "Page.describeNode", json.RawMessage(`{"contentFrameId":"frame-iframe-1"}`), nil)
	mb.SetResponse("", "Runtime.callFunction", json.RawMessage(`{"result":{"value":"{\"nodeType\":1,\"nodeName\":\"IFRAME\",\"localName\":\"iframe\",\"nodeValue\":\"\",\"childCount\":0,\"attrs\":[\"src\",\"https://example.com\"]}"}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.describeNode",
		Params: json.RawMessage(`{"objectId":"obj-iframe-1"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var parsed struct {
		Node struct {
			NodeName       string `json:"nodeName"`
			LocalName      string `json:"localName"`
			ContentFrameID string `json:"contentFrameId"`
		} `json:"node"`
	}
	json.Unmarshal(result, &parsed)

	if parsed.Node.ContentFrameID != "frame-iframe-1" {
		t.Errorf("contentFrameId = %q, want frame-iframe-1", parsed.Node.ContentFrameID)
	}
	if parsed.Node.NodeName != "IFRAME" {
		t.Errorf("nodeName = %q, want IFRAME", parsed.Node.NodeName)
	}
	if parsed.Node.LocalName != "iframe" {
		t.Errorf("localName = %q, want iframe", parsed.Node.LocalName)
	}
}

func TestHandleDOM_DescribeNode_NoContentFrameId(t *testing.T) {
	b, mb := newTestBridge()
	// Page.describeNode returns no contentFrameId for non-iframe elements
	mb.SetResponse("", "Page.describeNode", json.RawMessage(`{}`), nil)
	mb.SetResponse("", "Runtime.callFunction", json.RawMessage(`{"result":{"value":"{\"nodeType\":1,\"nodeName\":\"DIV\",\"localName\":\"div\",\"nodeValue\":\"\",\"childCount\":2,\"attrs\":[\"class\",\"container\"]}"}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.describeNode",
		Params: json.RawMessage(`{"objectId":"obj-div-1"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var parsed struct {
		Node struct {
			NodeName       string `json:"nodeName"`
			ContentFrameID string `json:"contentFrameId"`
		} `json:"node"`
	}
	json.Unmarshal(result, &parsed)

	if parsed.Node.ContentFrameID != "" {
		t.Errorf("contentFrameId should be empty for non-iframe, got %q", parsed.Node.ContentFrameID)
	}
	if parsed.Node.NodeName != "DIV" {
		t.Errorf("nodeName = %q, want DIV", parsed.Node.NodeName)
	}
}

func TestHandleDOM_DescribeNode_JugglerFallback(t *testing.T) {
	b, mb := newTestBridge()
	// Page.describeNode fails — should fall back to Runtime.callFunction only
	mb.SetResponse("", "Page.describeNode", nil, fmt.Errorf("not supported"))
	mb.SetResponse("", "Runtime.callFunction", json.RawMessage(`{"result":{"value":"{\"nodeType\":1,\"nodeName\":\"SPAN\",\"localName\":\"span\",\"nodeValue\":\"\",\"childCount\":1,\"attrs\":[]}"}}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "DOM.describeNode",
		Params: json.RawMessage(`{"objectId":"obj-span-1"}`),
	}

	result, cdpErr := b.handleDOM(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var parsed struct {
		Node struct {
			NodeName string `json:"nodeName"`
		} `json:"node"`
	}
	json.Unmarshal(result, &parsed)

	if parsed.Node.NodeName != "SPAN" {
		t.Errorf("nodeName = %q, want SPAN", parsed.Node.NodeName)
	}
}
