package bridge

import (
	"encoding/json"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func TestToJSString(t *testing.T) {
	tests := []struct {
		input string
		want  string
	}{
		{`hello`, `"hello"`},
		{`he said "hi"`, `"he said \"hi\""`},
		{"line1\nline2", `"line1\nline2"`},
		{`back\slash`, `"back\\slash"`},
		{`<script>alert('xss')</script>`, `"\u003cscript\u003ealert('xss')\u003c/script\u003e"`},
		{"tab\there", `"tab\there"`},
		{``, `""`},
	}

	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := toJSString(tt.input)
			if got != tt.want {
				t.Errorf("toJSString(%q) = %q, want %q", tt.input, got, tt.want)
			}

			// Verify the output is valid JSON
			var s string
			if err := json.Unmarshal([]byte(got), &s); err != nil {
				t.Errorf("toJSString(%q) output is not valid JSON: %v", tt.input, err)
			}
		})
	}
}

func TestMarshalResult_ValidJSON(t *testing.T) {
	tests := []struct {
		name  string
		input interface{}
	}{
		{"string map", map[string]string{"key": "val"}},
		{"nested map", map[string]interface{}{
			"outer": map[string]interface{}{
				"inner": 42,
			},
		}},
		{"slice", map[string]interface{}{"items": []int{1, 2, 3}}},
		{"empty", map[string]interface{}{}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			result, cdpErr := marshalResult(tt.input)
			if cdpErr != nil {
				t.Fatalf("marshalResult error: %s", cdpErr.Message)
			}

			// Verify it's valid JSON
			var parsed interface{}
			if err := json.Unmarshal(result, &parsed); err != nil {
				t.Errorf("marshalResult output is not valid JSON: %v\nraw: %s", err, string(result))
			}
		})
	}
}

func TestMarshalResult_Unmarshalable(t *testing.T) {
	// channels are not JSON-serializable
	_, cdpErr := marshalResult(make(chan int))
	if cdpErr == nil {
		t.Fatal("expected error for unmarshalable value")
	}
	if cdpErr.Code != -32000 {
		t.Errorf("error code = %d, want -32000", cdpErr.Code)
	}
}

func TestHandlePage_Reload(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.reload", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{ID: 1, Method: "Page.reload", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandlePage_Close(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.close", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{ID: 1, Method: "Page.close", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandlePage_SetContent(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.navigate", json.RawMessage(`{"navigationId":"nav-1"}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.setContent",
		Params: json.RawMessage(`{"html":"<h1>Hello</h1>"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	// Verify the Page.navigate call was made with a data URI
	calls := mb.CallsForMethod("Page.navigate")
	if len(calls) == 0 {
		t.Fatal("expected Page.navigate call for setContent")
	}

	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	url, ok := params["url"].(string)
	if !ok {
		t.Fatal("url not found in params")
	}
	if !strings.Contains(url, "data:text/html,") {
		t.Errorf("url should be data URI, got %s", url)
	}
	if !strings.Contains(url, "Hello") {
		t.Errorf("url should contain HTML content, got %s", url)
	}
}

func TestHandlePage_SetContentEmpty(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.setContent",
		Params: json.RawMessage(`{"html":""}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandlePage_NoopMethods(t *testing.T) {
	noops := []string{
		"Page.setInterceptFileChooserDialog",
		"Page.setBypassCSP",
		"Page.stopLoading",
		"Page.navigateToHistoryEntry",
		"Page.resetNavigationHistory",
	}

	for _, method := range noops {
		t.Run(method, func(t *testing.T) {
			b, _ := newTestBridge()
			msg := &cdp.Message{ID: 1, Method: method, Params: json.RawMessage(`{}`)}
			result, cdpErr := b.handlePage(nil, msg)
			if cdpErr != nil {
				t.Errorf("error: %s", cdpErr.Message)
			}
			if string(result) != "{}" {
				t.Errorf("result = %s, want {}", string(result))
			}
		})
	}
}

func TestHandlePage_BringToFront(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.bringToFront", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{ID: 1, Method: "Page.bringToFront", SessionID: "s1"}
	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Page.bringToFront")
	if len(calls) == 0 {
		t.Fatal("expected Page.bringToFront call to Juggler")
	}
}

func TestHandlePage_BringToFront_Error(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.bringToFront", nil, fmt.Errorf("page not found"))

	msg := &cdp.Message{ID: 1, Method: "Page.bringToFront", SessionID: "s1"}
	_, cdpErr := b.handlePage(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error")
	}
	if cdpErr.Code != -32000 {
		t.Errorf("error code = %d, want -32000", cdpErr.Code)
	}
}

func TestHandlePage_GetNavigationHistory(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{ID: 1, Method: "Page.getNavigationHistory", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		CurrentIndex int `json:"currentIndex"`
		Entries      []struct {
			URL string `json:"url"`
		} `json:"entries"`
	}
	json.Unmarshal(result, &res)

	if res.CurrentIndex != 0 {
		t.Errorf("currentIndex = %d, want 0", res.CurrentIndex)
	}
	if len(res.Entries) != 1 {
		t.Fatalf("entries len = %d, want 1", len(res.Entries))
	}
	if res.Entries[0].URL != "about:blank" {
		t.Errorf("entry url = %q, want about:blank", res.Entries[0].URL)
	}
}

func TestHandlePage_ScreenshotJpeg(t *testing.T) {
	b, mb := newTestBridge()

	mb.SetResponse("", "Page.screenshot", json.RawMessage(`{"data":"jpeg-data"}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.captureScreenshot",
		Params: json.RawMessage(`{"format":"jpeg","quality":80}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	// Verify juggler received the correct mimeType
	calls := mb.CallsForMethod("Page.screenshot")
	if len(calls) == 0 {
		t.Fatal("expected Page.screenshot call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["mimeType"] != "image/jpeg" {
		t.Errorf("mimeType = %v, want image/jpeg", params["mimeType"])
	}

	var res struct {
		Data string `json:"data"`
	}
	json.Unmarshal(result, &res)
	if res.Data != "jpeg-data" {
		t.Errorf("data = %q, want jpeg-data", res.Data)
	}
}

func TestHandlePage_AddScriptToEvaluateOnNewDocument(t *testing.T) {
	b, mb := newTestBridge()

	mb.SetResponse("", "Page.addScriptToEvaluateOnNewDocument",
		json.RawMessage(`{"scriptId":"script-42"}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.addScriptToEvaluateOnNewDocument",
		Params: json.RawMessage(`{"source":"console.log('hi')"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Identifier string `json:"identifier"`
	}
	json.Unmarshal(result, &res)
	if res.Identifier != "script-42" {
		t.Errorf("identifier = %q, want script-42", res.Identifier)
	}
}

func TestHandlePage_GetResourceTree(t *testing.T) {
	b, _ := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID: "s1",
		TargetID:  "t1",
		FrameID:   "frame-xyz",
		URL:       "https://test.com",
	})

	msg := &cdp.Message{
		ID:        1,
		Method:    "Page.getResourceTree",
		SessionID: "s1",
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		FrameTree struct {
			Frame struct {
				ID  string `json:"id"`
				URL string `json:"url"`
			} `json:"frame"`
		} `json:"frameTree"`
	}
	json.Unmarshal(result, &res)

	if res.FrameTree.Frame.ID != "frame-xyz" {
		t.Errorf("frame id = %q, want frame-xyz", res.FrameTree.Frame.ID)
	}
}

func TestHandlePage_Enable(t *testing.T) {
	b, _ := newTestBridge()
	msg := &cdp.Message{ID: 1, Method: "Page.enable", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}

func TestHandlePage_CreateIsolatedWorld(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:        1,
		Method:    "Page.createIsolatedWorld",
		SessionID: "s1",
		Params:    json.RawMessage(`{"frameId":"frame-1","worldName":"utility","grantUniveralAccess":true}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		ExecutionContextID int `json:"executionContextId"`
	}
	json.Unmarshal(result, &res)

	if res.ExecutionContextID <= 100 {
		t.Errorf("executionContextId = %d, want > 100", res.ExecutionContextID)
	}

	// Second call should produce a different (higher) ID
	result2, cdpErr2 := b.handlePage(nil, msg)
	if cdpErr2 != nil {
		t.Fatalf("second call error: %s", cdpErr2.Message)
	}
	var res2 struct {
		ExecutionContextID int `json:"executionContextId"`
	}
	json.Unmarshal(result2, &res2)

	if res2.ExecutionContextID <= res.ExecutionContextID {
		t.Errorf("second id %d should be > first id %d", res2.ExecutionContextID, res.ExecutionContextID)
	}

	// Wait briefly for the async event emission goroutine
	time.Sleep(50 * time.Millisecond)
}

func TestHandlePage_GetLayoutMetrics_Fallback(t *testing.T) {
	b, mb := newTestBridge()
	// Make Runtime.evaluate fail to trigger fallback defaults
	mb.SetResponse("", "Runtime.evaluate", nil, fmt.Errorf("eval failed"))

	msg := &cdp.Message{ID: 1, Method: "Page.getLayoutMetrics", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		LayoutViewport struct {
			ClientWidth  float64 `json:"clientWidth"`
			ClientHeight float64 `json:"clientHeight"`
		} `json:"layoutViewport"`
		VisualViewport struct {
			ClientWidth  float64 `json:"clientWidth"`
			ClientHeight float64 `json:"clientHeight"`
			Zoom         float64 `json:"zoom"`
		} `json:"visualViewport"`
		ContentSize struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"contentSize"`
	}
	json.Unmarshal(result, &res)

	if res.LayoutViewport.ClientWidth != 1280 {
		t.Errorf("layoutViewport.clientWidth = %v, want 1280", res.LayoutViewport.ClientWidth)
	}
	if res.LayoutViewport.ClientHeight != 720 {
		t.Errorf("layoutViewport.clientHeight = %v, want 720", res.LayoutViewport.ClientHeight)
	}
	if res.VisualViewport.Zoom != 1 {
		t.Errorf("visualViewport.zoom = %v, want 1", res.VisualViewport.Zoom)
	}
}

func TestHandlePage_GetLayoutMetrics_WithEval(t *testing.T) {
	b, mb := newTestBridge()
	evalResult := `{"result":{"value":"{\"width\":1920,\"height\":1080,\"devicePixelRatio\":2,\"scrollX\":0,\"scrollY\":100,\"docWidth\":1920,\"docHeight\":5000}"}}`
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(evalResult), nil)

	msg := &cdp.Message{ID: 1, Method: "Page.getLayoutMetrics", Params: json.RawMessage(`{}`)}
	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		LayoutViewport struct {
			ClientWidth  float64 `json:"clientWidth"`
			ClientHeight float64 `json:"clientHeight"`
			PageY        float64 `json:"pageY"`
		} `json:"layoutViewport"`
		ContentSize struct {
			Width  float64 `json:"width"`
			Height float64 `json:"height"`
		} `json:"contentSize"`
		VisualViewport struct {
			Zoom float64 `json:"zoom"`
		} `json:"visualViewport"`
	}
	json.Unmarshal(result, &res)

	if res.LayoutViewport.ClientWidth != 1920 {
		t.Errorf("clientWidth = %v, want 1920", res.LayoutViewport.ClientWidth)
	}
	if res.LayoutViewport.ClientHeight != 1080 {
		t.Errorf("clientHeight = %v, want 1080", res.LayoutViewport.ClientHeight)
	}
	if res.LayoutViewport.PageY != 100 {
		t.Errorf("pageY = %v, want 100", res.LayoutViewport.PageY)
	}
	if res.ContentSize.Height != 5000 {
		t.Errorf("contentSize.height = %v, want 5000", res.ContentSize.Height)
	}
	if res.VisualViewport.Zoom != 2 {
		t.Errorf("visualViewport.zoom = %v, want 2 (DPR)", res.VisualViewport.Zoom)
	}
}

func TestHandlePage_HandleJavaScriptDialog_Accept(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.handleDialog", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.handleJavaScriptDialog",
		Params: json.RawMessage(`{"accept":true,"promptText":"hello"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Page.handleDialog")
	if len(calls) == 0 {
		t.Fatal("expected Page.handleDialog call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["accept"] != true {
		t.Errorf("accept = %v, want true", params["accept"])
	}
	if params["promptText"] != "hello" {
		t.Errorf("promptText = %v, want hello", params["promptText"])
	}
}

func TestHandlePage_HandleJavaScriptDialog_Reject(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.handleDialog", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.handleJavaScriptDialog",
		Params: json.RawMessage(`{"accept":false}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Page.handleDialog")
	if len(calls) == 0 {
		t.Fatal("expected Page.handleDialog call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["accept"] != false {
		t.Errorf("accept = %v, want false", params["accept"])
	}
	// promptText should not be present when empty
	if _, ok := params["promptText"]; ok {
		t.Error("promptText should not be present when empty")
	}
}

func TestHandlePage_PrintToPDF(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.printToPDF", json.RawMessage(`{"data":"pdf-base64"}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.printToPDF",
		Params: json.RawMessage(`{"landscape":true,"printBackground":true,"scale":0.8,"paperWidth":8.5,"paperHeight":11,"pageRanges":"1-3"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}

	var res struct {
		Data string `json:"data"`
	}
	json.Unmarshal(result, &res)
	if res.Data != "pdf-base64" {
		t.Errorf("data = %q, want pdf-base64", res.Data)
	}

	// Verify juggler params were mapped correctly
	calls := mb.CallsForMethod("Page.printToPDF")
	if len(calls) == 0 {
		t.Fatal("expected Page.printToPDF call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["landscape"] != true {
		t.Errorf("landscape = %v, want true", params["landscape"])
	}
	if params["printBackground"] != true {
		t.Errorf("printBackground = %v, want true", params["printBackground"])
	}
	if params["scale"] != 0.8 {
		t.Errorf("scale = %v, want 0.8", params["scale"])
	}
	if params["paperWidth"] != 8.5 {
		t.Errorf("paperWidth = %v, want 8.5", params["paperWidth"])
	}
	if params["pageRanges"] != "1-3" {
		t.Errorf("pageRanges = %v, want 1-3", params["pageRanges"])
	}
}

func TestHandlePage_SetExtraHTTPHeaders(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		TargetID:         "t1",
		BrowserContextID: "ctx-42",
	})
	mb.SetResponse("", "Browser.setExtraHTTPHeaders", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:        1,
		Method:    "Page.setExtraHTTPHeaders",
		SessionID: "s1",
		Params:    json.RawMessage(`{"headers":{"X-Custom":"value","Accept-Language":"en"}}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Browser.setExtraHTTPHeaders")
	if len(calls) == 0 {
		t.Fatal("expected Browser.setExtraHTTPHeaders call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)

	headers, ok := params["headers"].(map[string]interface{})
	if !ok {
		t.Fatal("headers not found in params")
	}
	if headers["X-Custom"] != "value" {
		t.Errorf("X-Custom = %v, want value", headers["X-Custom"])
	}
	if params["browserContextId"] != "ctx-42" {
		t.Errorf("browserContextId = %v, want ctx-42", params["browserContextId"])
	}
}

func TestHandlePage_RemoveScriptToEvaluateOnNewDocument(t *testing.T) {
	b, mb := newTestBridge()
	mb.SetResponse("", "Page.removeScriptToEvaluateOnNewDocument", json.RawMessage(`{}`), nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.removeScriptToEvaluateOnNewDocument",
		Params: json.RawMessage(`{"identifier":"script-99"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Page.removeScriptToEvaluateOnNewDocument")
	if len(calls) == 0 {
		t.Fatal("expected Page.removeScriptToEvaluateOnNewDocument call")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["scriptId"] != "script-99" {
		t.Errorf("scriptId = %v, want script-99", params["scriptId"])
	}
}

func TestHandlePage_SetDownloadBehavior_Allow(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		TargetID:         "t1",
		BrowserContextID: "ctx-7",
	})

	msg := &cdp.Message{
		ID:        1,
		Method:    "Page.setDownloadBehavior",
		SessionID: "s1",
		Params:    json.RawMessage(`{"behavior":"allow","downloadPath":"/tmp/downloads"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	calls := mb.CallsForMethod("Browser.setDownloadOptions")
	if len(calls) == 0 {
		t.Fatal("expected Browser.setDownloadOptions call for allow behavior")
	}
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	if params["downloadPath"] != "/tmp/downloads" {
		t.Errorf("downloadPath = %v, want /tmp/downloads", params["downloadPath"])
	}
	if params["browserContextId"] != "ctx-7" {
		t.Errorf("browserContextId = %v, want ctx-7", params["browserContextId"])
	}
}

func TestHandlePage_SetDownloadBehavior_Deny(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Page.setDownloadBehavior",
		Params: json.RawMessage(`{"behavior":"deny"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	// Deny should NOT call Browser.setDownloadOptions
	calls := mb.CallsForMethod("Browser.setDownloadOptions")
	if len(calls) != 0 {
		t.Errorf("expected no Browser.setDownloadOptions call for deny, got %d", len(calls))
	}
}

func TestHandlePage_BrowserSetDownloadBehavior(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Browser.setDownloadBehavior",
		Params: json.RawMessage(`{"behavior":"allow","downloadPath":"/tmp/dl"}`),
	}

	result, cdpErr := b.handlePage(nil, msg)
	if cdpErr != nil {
		t.Fatalf("error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
}
