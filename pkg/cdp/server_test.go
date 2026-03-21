package cdp

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
)

// serverForTest creates an http.ServeMux with the Server's HTTP handlers wired up.
// This avoids starting a real listener while testing the JSON endpoints.
func serverForTest(sessions *SessionManager) *http.ServeMux {
	s := NewServer(9222, nil, sessions)
	mux := http.NewServeMux()
	mux.HandleFunc("/json/version", s.handleVersion)
	mux.HandleFunc("/json", s.handleList)
	mux.HandleFunc("/json/list", s.handleList)
	return mux
}

func TestServer_JSONVersion(t *testing.T) {
	mux := serverForTest(NewSessionManager())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/json/version")
	if err != nil {
		t.Fatalf("GET /json/version: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != 200 {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}

	ct := resp.Header.Get("Content-Type")
	if ct != "application/json" {
		t.Errorf("Content-Type = %q, want application/json", ct)
	}

	body, _ := io.ReadAll(resp.Body)
	var info map[string]string
	if err := json.Unmarshal(body, &info); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if info["Browser"] != "foxbridge/1.0" {
		t.Errorf("Browser = %q, want foxbridge/1.0", info["Browser"])
	}
	if info["Protocol-Version"] != "1.3" {
		t.Errorf("Protocol-Version = %q, want 1.3", info["Protocol-Version"])
	}
	if info["webSocketDebuggerUrl"] == "" {
		t.Error("webSocketDebuggerUrl is empty")
	}
}

func TestServer_JSONList_Empty(t *testing.T) {
	mux := serverForTest(NewSessionManager())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	for _, path := range []string{"/json", "/json/list"} {
		t.Run(path, func(t *testing.T) {
			resp, err := http.Get(ts.URL + path)
			if err != nil {
				t.Fatalf("GET %s: %v", path, err)
			}
			defer resp.Body.Close()

			body, _ := io.ReadAll(resp.Body)
			var targets []interface{}
			if err := json.Unmarshal(body, &targets); err != nil {
				t.Fatalf("unmarshal: %v", err)
			}

			if len(targets) != 0 {
				t.Errorf("targets len = %d, want 0", len(targets))
			}
		})
	}
}

func TestServer_JSONList_WithSessions(t *testing.T) {
	sm := NewSessionManager()
	sm.Add(&SessionInfo{
		SessionID: "s1",
		TargetID:  "target-1",
		URL:       "https://example.com",
		Title:     "Example",
		Type:      "page",
	})
	sm.Add(&SessionInfo{
		SessionID: "s2",
		TargetID:  "target-2",
		URL:       "https://test.com",
		Title:     "Test",
		Type:      "page",
	})
	// Non-page type should be filtered out
	sm.Add(&SessionInfo{
		SessionID: "s3",
		TargetID:  "target-3",
		Type:      "tab",
	})

	mux := serverForTest(sm)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/json/list")
	if err != nil {
		t.Fatalf("GET /json/list: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var targets []map[string]interface{}
	if err := json.Unmarshal(body, &targets); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}

	if len(targets) != 2 {
		t.Fatalf("targets len = %d, want 2 (tab should be filtered)", len(targets))
	}

	// Check first target
	found := false
	for _, tgt := range targets {
		if tgt["id"] == "target-1" {
			found = true
			if tgt["title"] != "Example" {
				t.Errorf("title = %v, want Example", tgt["title"])
			}
			if tgt["url"] != "https://example.com" {
				t.Errorf("url = %v, want https://example.com", tgt["url"])
			}
			if tgt["type"] != "page" {
				t.Errorf("type = %v, want page", tgt["type"])
			}
			break
		}
	}
	if !found {
		t.Error("target-1 not found in /json/list response")
	}
}

func TestServer_JSONList_BlankURL(t *testing.T) {
	sm := NewSessionManager()
	sm.Add(&SessionInfo{
		SessionID: "s1",
		TargetID:  "target-1",
		URL:       "", // empty URL
		Type:      "page",
	})

	mux := serverForTest(sm)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/json/list")
	if err != nil {
		t.Fatalf("GET /json/list: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var targets []map[string]interface{}
	json.Unmarshal(body, &targets)

	if len(targets) != 1 {
		t.Fatalf("targets len = %d, want 1", len(targets))
	}
	if targets[0]["url"] != "about:blank" {
		t.Errorf("url = %v, want about:blank for empty URL", targets[0]["url"])
	}
}

func TestServer_JSONList_TargetHasWebSocketURL(t *testing.T) {
	sm := NewSessionManager()
	sm.Add(&SessionInfo{
		SessionID: "s1",
		TargetID:  "target-abc",
		URL:       "https://example.com",
		Type:      "page",
	})

	mux := serverForTest(sm)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/json/list")
	if err != nil {
		t.Fatalf("GET /json/list: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var targets []map[string]interface{}
	json.Unmarshal(body, &targets)

	wsURL, ok := targets[0]["webSocketDebuggerUrl"].(string)
	if !ok || wsURL == "" {
		t.Error("webSocketDebuggerUrl missing or empty")
	}
}

func TestServer_JSONList_NilSessions(t *testing.T) {
	s := NewServer(9222, nil, nil)
	mux := http.NewServeMux()
	mux.HandleFunc("/json/list", s.handleList)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/json/list")
	if err != nil {
		t.Fatalf("GET /json/list: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if string(body) != "[]" {
		t.Errorf("body = %q, want []", string(body))
	}
}

func TestServer_JSONVersion_Fields(t *testing.T) {
	mux := serverForTest(NewSessionManager())
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/json/version")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	var info map[string]string
	json.Unmarshal(body, &info)

	required := []string{"Browser", "Protocol-Version", "User-Agent", "webSocketDebuggerUrl"}
	for _, key := range required {
		if _, exists := info[key]; !exists {
			t.Errorf("missing required field %q in /json/version", key)
		}
	}

	// Verify the WS URL has the expected format
	expected := fmt.Sprintf("ws://127.0.0.1:%d/devtools/browser/foxbridge", 9222)
	if info["webSocketDebuggerUrl"] != expected {
		t.Errorf("webSocketDebuggerUrl = %q, want %q", info["webSocketDebuggerUrl"], expected)
	}
}
