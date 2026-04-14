package bridge

import (
	"encoding/json"
	"testing"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func TestEmulationSetGeolocationOverride_AllParams(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setGeolocationOverride",
		Params: json.RawMessage(`{"latitude":37.7749,"longitude":-122.4194,"accuracy":100}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setGeolocationOverride" {
		t.Errorf("method = %q, want Browser.setGeolocationOverride", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["latitude"] != 37.7749 {
		t.Errorf("latitude = %v, want 37.7749", params["latitude"])
	}
	if params["longitude"] != -122.4194 {
		t.Errorf("longitude = %v, want -122.4194", params["longitude"])
	}
	if params["accuracy"] != float64(100) {
		t.Errorf("accuracy = %v, want 100", params["accuracy"])
	}
}

func TestEmulationSetGeolocationOverride_NoParams(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setGeolocationOverride",
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	// No latitude/longitude/accuracy should be set
	if _, ok := params["latitude"]; ok {
		t.Error("latitude should not be set when not provided")
	}
	if _, ok := params["longitude"]; ok {
		t.Error("longitude should not be set when not provided")
	}
	if _, ok := params["accuracy"]; ok {
		t.Error("accuracy should not be set when not provided")
	}
}

func TestEmulationSetGeolocationOverride_PartialParams(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setGeolocationOverride",
		Params: json.RawMessage(`{"latitude":51.5074}`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["latitude"] != 51.5074 {
		t.Errorf("latitude = %v, want 51.5074", params["latitude"])
	}
	if _, ok := params["longitude"]; ok {
		t.Error("longitude should not be set")
	}
}

func TestEmulationSetGeolocationOverride_WithSession(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s1",
		BrowserContextID: "ctx-geo",
		TargetID:         "t1",
	})

	msg := &cdp.Message{
		ID:        1,
		Method:    "Emulation.setGeolocationOverride",
		SessionID: "s1",
		Params:    json.RawMessage(`{"latitude":0,"longitude":0}`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["browserContextId"] != "ctx-geo" {
		t.Errorf("browserContextId = %v, want ctx-geo", params["browserContextId"])
	}
}

func TestEmulationSetUserAgentOverride(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setUserAgentOverride",
		Params: json.RawMessage(`{"userAgent":"Mozilla/5.0 Custom"}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setUserAgentOverride" {
		t.Errorf("method = %q, want Browser.setUserAgentOverride", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)
	if params["userAgent"] != "Mozilla/5.0 Custom" {
		t.Errorf("userAgent = %v, want Mozilla/5.0 Custom", params["userAgent"])
	}
}

func TestEmulationSetUserAgentOverride_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setUserAgentOverride",
		Params: json.RawMessage(`bad`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestEmulationSetUserAgentOverride_WithSession(t *testing.T) {
	b, mb := newTestBridge()
	b.sessions.Add(&cdp.SessionInfo{
		SessionID:        "s-ua",
		BrowserContextID: "ctx-ua",
		TargetID:         "t-ua",
	})

	msg := &cdp.Message{
		ID:        1,
		Method:    "Emulation.setUserAgentOverride",
		SessionID: "s-ua",
		Params:    json.RawMessage(`{"userAgent":"Test/1.0"}`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)
	if params["browserContextId"] != "ctx-ua" {
		t.Errorf("browserContextId = %v, want ctx-ua", params["browserContextId"])
	}
}

func TestEmulationSetTimezoneOverride(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setTimezoneOverride",
		Params: json.RawMessage(`{"timezoneId":"America/New_York"}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setTimezoneOverride" {
		t.Errorf("method = %q, want Browser.setTimezoneOverride", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)
	if params["timezoneId"] != "America/New_York" {
		t.Errorf("timezoneId = %v, want America/New_York", params["timezoneId"])
	}
}

func TestEmulationSetTimezoneOverride_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setTimezoneOverride",
		Params: json.RawMessage(`nope`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestEmulationSetLocaleOverride(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setLocaleOverride",
		Params: json.RawMessage(`{"locale":"fr-FR"}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setLocaleOverride" {
		t.Errorf("method = %q, want Browser.setLocaleOverride", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)
	if params["locale"] != "fr-FR" {
		t.Errorf("locale = %v, want fr-FR", params["locale"])
	}
}

func TestEmulationSetLocaleOverride_InvalidParams(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setLocaleOverride",
		Params: json.RawMessage(`bad`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for invalid params")
	}
	if cdpErr.Code != -32602 {
		t.Errorf("error code = %d, want -32602", cdpErr.Code)
	}
}

func TestEmulationSetDeviceMetricsOverride(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setDeviceMetricsOverride",
		Params: json.RawMessage(`{"width":1920,"height":1080,"deviceScaleFactor":2,"mobile":false}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setDefaultViewport" {
		t.Errorf("method = %q, want Browser.setDefaultViewport", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["width"] != float64(1920) {
		t.Errorf("width = %v, want 1920", params["width"])
	}
	if params["height"] != float64(1080) {
		t.Errorf("height = %v, want 1080", params["height"])
	}
	if params["deviceScaleFactor"] != float64(2) {
		t.Errorf("deviceScaleFactor = %v, want 2", params["deviceScaleFactor"])
	}
}

func TestEmulationSetDeviceMetricsOverride_NoScaleFactor(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setDeviceMetricsOverride",
		Params: json.RawMessage(`{"width":800,"height":600,"deviceScaleFactor":0}`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	// deviceScaleFactor=0 should be omitted
	if _, ok := params["deviceScaleFactor"]; ok {
		t.Error("deviceScaleFactor should not be set when 0")
	}
	if params["width"] != float64(800) {
		t.Errorf("width = %v, want 800", params["width"])
	}
	if params["height"] != float64(600) {
		t.Errorf("height = %v, want 600", params["height"])
	}
}

func TestEmulationClearDeviceMetricsOverride(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.clearDeviceMetricsOverride",
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// No-op stub
	if mb.CallCount() != 0 {
		t.Errorf("expected 0 backend calls, got %d", mb.CallCount())
	}
}

func TestEmulationSetTouchEmulationEnabled(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setTouchEmulationEnabled",
		Params: json.RawMessage(`{"enabled":true}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Browser.setTouchOverride" {
		t.Errorf("method = %q, want Browser.setTouchOverride", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)
	if params["enabled"] != true {
		t.Errorf("enabled = %v, want true", params["enabled"])
	}
}

func TestEmulationSetTouchEmulationEnabled_Disabled(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setTouchEmulationEnabled",
		Params: json.RawMessage(`{"enabled":false}`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)
	if params["enabled"] != false {
		t.Errorf("enabled = %v, want false", params["enabled"])
	}
}

func TestEmulationSetEmulatedMedia(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setEmulatedMedia",
		Params: json.RawMessage(`{
			"media": "print",
			"features": [{"name":"prefers-color-scheme","value":"dark"}]
		}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Page.setEmulatedMedia" {
		t.Errorf("method = %q, want Page.setEmulatedMedia", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if params["type"] != "print" {
		t.Errorf("type = %v, want print", params["type"])
	}

	features, ok := params["features"].([]interface{})
	if !ok {
		t.Fatalf("features not an array: %T", params["features"])
	}
	if len(features) != 1 {
		t.Errorf("features count = %d, want 1", len(features))
	}
}

func TestEmulationSetEmulatedMedia_EmptyParams(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setEmulatedMedia",
		Params: json.RawMessage(`{}`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	if _, ok := params["type"]; ok {
		t.Error("type should not be set for empty media")
	}
	if _, ok := params["features"]; ok {
		t.Error("features should not be set for nil features")
	}
}

func TestEmulationSetScrollbarsHidden(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setScrollbarsHidden",
		Params: json.RawMessage(`{"hidden":true}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// Stub — no backend calls
	if mb.CallCount() != 0 {
		t.Errorf("expected 0 backend calls for stub, got %d", mb.CallCount())
	}
}

func TestEmulationSetDefaultBackgroundColorOverride(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setDefaultBackgroundColorOverride",
		Params: json.RawMessage(`{"color":{"r":255,"g":0,"b":0,"a":1}}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}
	// Stub — no backend calls
	if mb.CallCount() != 0 {
		t.Errorf("expected 0 backend calls for stub, got %d", mb.CallCount())
	}
}

func TestEmulationSetScriptExecutionDisabled(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setScriptExecutionDisabled",
		Params: json.RawMessage(`{"value":true}`),
	}

	result, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}
	if string(result) != "{}" {
		t.Errorf("result = %s, want {}", string(result))
	}

	last, _ := mb.LastCall()
	if last.Method != "Runtime.evaluate" {
		t.Errorf("method = %q, want Runtime.evaluate", last.Method)
	}

	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	expr, ok := params["expression"].(string)
	if !ok {
		t.Fatal("expression not a string")
	}
	// value=true means disable JS, so allowJavascript=false
	if expr != "void(document.docShell && (document.docShell.allowJavascript = false))" {
		t.Errorf("expression = %q, unexpected", expr)
	}
}

func TestEmulationSetScriptExecutionDisabled_Enable(t *testing.T) {
	b, mb := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.setScriptExecutionDisabled",
		Params: json.RawMessage(`{"value":false}`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr != nil {
		t.Fatalf("unexpected error: %s", cdpErr.Message)
	}

	last, _ := mb.LastCall()
	var params map[string]interface{}
	json.Unmarshal(last.Params, &params)

	expr := params["expression"].(string)
	// value=false means enable JS, so allowJavascript=true
	if expr != "void(document.docShell && (document.docShell.allowJavascript = true))" {
		t.Errorf("expression = %q, unexpected", expr)
	}
}

func TestEmulationUnknownMethod(t *testing.T) {
	b, _ := newTestBridge()

	msg := &cdp.Message{
		ID:     1,
		Method: "Emulation.doesNotExist",
		Params: json.RawMessage(`{}`),
	}

	_, cdpErr := b.handleEmulation(nil, msg)
	if cdpErr == nil {
		t.Fatal("expected error for unknown method")
	}
	if cdpErr.Code != -32601 {
		t.Errorf("error code = %d, want -32601", cdpErr.Code)
	}
}
