package bridge

import (
	"encoding/json"
	"testing"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func TestNetworkEnable(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.enable",
		Params: json.RawMessage(`{}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// No-op: should not call backend
	if mb.CallCount() != 0 {
		t.Errorf("expected 0 backend calls, got %d", mb.CallCount())
	}
}

func TestNetworkDisable(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.disable",
		Params: json.RawMessage(`{}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	if mb.CallCount() != 0 {
		t.Errorf("expected 0 backend calls, got %d", mb.CallCount())
	}
}

func TestNetworkSetCacheDisabled(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.setCacheDisabled",
		Params: json.RawMessage(`{"cacheDisabled":true}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// No backend call — it's a stub
	if mb.CallCount() != 0 {
		t.Errorf("expected 0 backend calls, got %d", mb.CallCount())
	}
}

func TestNetworkSetCookies(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.setCookies",
		Params: json.RawMessage(`{"cookies":[{"name":"a","value":"1"},{"name":"b","value":"2"}]}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setCookies" {
		t.Errorf("method = %q, want Browser.setCookies", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	cookies, ok := params["cookies"].([]interface{})
	if !ok {
		t.Fatalf("cookies not an array: %T", params["cookies"])
	}
	if len(cookies) != 2 {
		t.Errorf("cookies count = %d, want 2", len(cookies))
	}
}

func TestNetworkSetCookiesWithSession(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		BrowserContextID: "ctx-abc",
		TargetID:         "t1",
	})

	msg := &cdp.Message{
		ID:        1,
		Method:    "Network.setCookies",
		SessionID: "s1",
		Params:    json.RawMessage(`{"cookies":[]}`),
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["browserContextId"] != "ctx-abc" {
		t.Errorf("browserContextId = %v, want ctx-abc", params["browserContextId"])
	}
}

func TestNetworkSetCookies_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.setCookies",
		Params: json.RawMessage(`not-json`),
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestNetworkGetCookies_WithURLs(t *testing.T) {
	b, mb := newTestBridge()

	resp, _ := json.Marshal(map[string]interface{}{
		"cookies": []map[string]string{{"name": "c1", "value": "v1"}},
	})
	mb.SetResponse("", "Browser.getCookies", resp, nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.getCookies",
		Params: json.RawMessage(`{"urls":["https://example.com"]}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.getCookies" {
		t.Errorf("method = %q, want Browser.getCookies", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)
	// Juggler's getCookies doesn't support urls — verify it's not sent
	if _, hasURLs := params["urls"]; hasURLs {
		t.Error("urls should not be sent to Juggler")
	}
	// Result should pass through from backend
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestNetworkGetCookies_WithoutURLs(t *testing.T) {
	b, mb := newTestBridge()

	resp, _ := json.Marshal(map[string]interface{}{"cookies": []interface{}{}})
	mb.SetResponse("", "Browser.getCookies", resp, nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.getCookies",
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if _, ok := params["urls"]; ok {
		t.Error("urls should not be set when not provided")
	}
}

func TestNetworkGetCookiesWithSession(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s2",
		BrowserContextID: "ctx-def",
		TargetID:         "t2",
	})

	resp, _ := json.Marshal(map[string]interface{}{"cookies": []interface{}{}})
	mb.SetResponse("", "Browser.getCookies", resp, nil)

	msg := &cdp.Message{
		ID:        1,
		Method:    "Network.getCookies",
		SessionID: "s2",
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["browserContextId"] != "ctx-def" {
		t.Errorf("browserContextId = %v, want ctx-def", params["browserContextId"])
	}
}

func TestNetworkClearBrowserCookies(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.clearBrowserCookies",
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.clearCookies" {
		t.Errorf("method = %q, want Browser.clearCookies", last.Method)
	}
}

func TestNetworkClearBrowserCookiesWithSession(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s3",
		BrowserContextID: "ctx-ghi",
		TargetID:         "t3",
	})

	msg := &cdp.Message{
		ID:        1,
		Method:    "Network.clearBrowserCookies",
		SessionID: "s3",
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["browserContextId"] != "ctx-ghi" {
		t.Errorf("browserContextId = %v, want ctx-ghi", params["browserContextId"])
	}
}

func TestNetworkSetExtraHTTPHeaders(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.setExtraHTTPHeaders",
		Params: json.RawMessage(`{"headers":{"X-Custom":"value","Accept":"text/html"}}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setExtraHTTPHeaders" {
		t.Errorf("method = %q, want Browser.setExtraHTTPHeaders", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	headers, ok := params["headers"].(map[string]interface{})
	if !ok {
		t.Fatalf("headers not a map: %T", params["headers"])
	}
	if headers["X-Custom"] != "value" {
		t.Errorf("X-Custom = %v, want value", headers["X-Custom"])
	}
}

func TestNetworkSetExtraHTTPHeaders_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.setExtraHTTPHeaders",
		Params: json.RawMessage(`broken`),
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestNetworkSetRequestInterception_WithPatterns(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.setRequestInterception",
		Params: json.RawMessage(`{"patterns":[{"urlPattern":"*"}]}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setRequestInterception" {
		t.Errorf("method = %q, want Browser.setRequestInterception", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["enabled"] != true {
		t.Errorf("enabled = %v, want true", params["enabled"])
	}
}

func TestNetworkSetRequestInterception_EmptyPatterns(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.setRequestInterception",
		Params: json.RawMessage(`{"patterns":[]}`),
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["enabled"] != false {
		t.Errorf("enabled = %v, want false (empty patterns)", params["enabled"])
	}
}

func TestNetworkSetUserAgentOverride(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.setUserAgentOverride",
		Params: json.RawMessage(`{"userAgent":"Mozilla/5.0"}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// Should call Browser.setUserAgentOverride on Juggler
	if mb.CallCount() != 1 {
		t.Errorf("expected 1 backend call, got %d", mb.CallCount())
	}
	last, _ := mb.LastCall()
	if last.Method != "Browser.setUserAgentOverride" {
		t.Errorf("method = %q, want Browser.setUserAgentOverride", last.Method)
	}
}

func TestNetworkEmulateNetworkConditions(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.emulateNetworkConditions",
		Params: json.RawMessage(`{"offline":false,"latency":100}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// No-op
	if mb.CallCount() != 0 {
		t.Errorf("expected 0 backend calls for no-op, got %d", mb.CallCount())
	}
}

func TestNetworkGetResponseBody(t *testing.T) {
	b, mb := newTestBridge()

	resp, _ := json.Marshal(map[string]interface{}{
		"body":          "SGVsbG8=",
		"base64Encoded": true,
	})
	mb.SetResponse("", "Browser.getResponseBody", resp, nil)

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.getResponseBody",
		Params: json.RawMessage(`{"requestId":"req-net-1"}`),
	}

	result, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.getResponseBody" {
		t.Errorf("method = %q, want Browser.getResponseBody", last.Method)
	}

	// Result is passed through directly
	if result == nil {
		t.Fatal("expected non-nil result")
	}
}

func TestNetworkGetResponseBody_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.getResponseBody",
		Params: json.RawMessage(`invalid`),
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestNetworkUnknownMethod(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Network.doesNotExist",
		Params: json.RawMessage(`{}`),
	}

	_, cdpErr := b.handleNetwork(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}
