package bridge

import (
	"encoding/json"
	"testing"

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
	mb.SetResponse("", "Runtime.evaluate", json.RawMessage(`{"result":{}}`), nil)

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

	// Verify the Runtime.evaluate call was made
	calls := mb.CallsForMethod("Runtime.evaluate")
	if len(calls) == 0 {
		t.Fatal("expected Runtime.evaluate call for setContent")
	}

	// The expression should contain the HTML
	var params map[string]interface{}
	json.Unmarshal(calls[0].Params, &params)
	expr, ok := params["expression"].(string)
	if !ok {
		t.Fatal("expression not found in params")
	}
	if len(expr) == 0 {
		t.Error("expression is empty")
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
		"Page.bringToFront",
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
