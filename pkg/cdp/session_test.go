package cdp

import (
	"fmt"
	"sync"
	"testing"
)

func TestSessionManager_AddGet(t *testing.T) {
	sm := NewSessionManager()

	info := &SessionInfo{
		SessionID:        "cdp-1",
		JugglerSessionID: "jug-1",
		TargetID:         "target-1",
		BrowserContextID: "ctx-1",
		URL:              "https://example.com",
		Type:             "page",
	}
	sm.Add(info)

	got, ok := sm.Get("cdp-1")
	if !ok {
		t.Fatal("Get returned false for existing session")
	}
	if got.JugglerSessionID != "jug-1" {
		t.Errorf("JugglerSessionID = %q, want %q", got.JugglerSessionID, "jug-1")
	}
	if got.URL != "https://example.com" {
		t.Errorf("URL = %q, want %q", got.URL, "https://example.com")
	}
}

func TestSessionManager_GetMiss(t *testing.T) {
	sm := NewSessionManager()

	_, ok := sm.Get("nonexistent")
	if ok {
		t.Error("Get returned true for nonexistent session")
	}
}

func TestSessionManager_Remove(t *testing.T) {
	sm := NewSessionManager()

	sm.Add(&SessionInfo{
		SessionID:        "cdp-1",
		JugglerSessionID: "jug-1",
		TargetID:         "target-1",
	})

	sm.Remove("cdp-1")

	if _, ok := sm.Get("cdp-1"); ok {
		t.Error("session still found after Remove")
	}
	if _, ok := sm.GetByTarget("target-1"); ok {
		t.Error("target still found after Remove")
	}
	if _, ok := sm.GetByJugglerSession("jug-1"); ok {
		t.Error("juggler session still found after Remove")
	}
}

func TestSessionManager_RemoveNonexistent(t *testing.T) {
	sm := NewSessionManager()
	// Should not panic
	sm.Remove("does-not-exist")
}

func TestSessionManager_GetByTarget(t *testing.T) {
	sm := NewSessionManager()

	sm.Add(&SessionInfo{
		SessionID: "cdp-1",
		TargetID:  "target-42",
		URL:       "https://test.com",
	})

	got, ok := sm.GetByTarget("target-42")
	if !ok {
		t.Fatal("GetByTarget returned false")
	}
	if got.SessionID != "cdp-1" {
		t.Errorf("SessionID = %q, want cdp-1", got.SessionID)
	}
}

func TestSessionManager_GetByTarget_Miss(t *testing.T) {
	sm := NewSessionManager()
	_, ok := sm.GetByTarget("nope")
	if ok {
		t.Error("GetByTarget returned true for nonexistent target")
	}
}

func TestSessionManager_GetByJugglerSession(t *testing.T) {
	sm := NewSessionManager()

	sm.Add(&SessionInfo{
		SessionID:        "cdp-1",
		JugglerSessionID: "jug-99",
		TargetID:         "t-1",
	})

	got, ok := sm.GetByJugglerSession("jug-99")
	if !ok {
		t.Fatal("GetByJugglerSession returned false")
	}
	if got.SessionID != "cdp-1" {
		t.Errorf("SessionID = %q, want cdp-1", got.SessionID)
	}
}

func TestSessionManager_GetByJugglerSession_Miss(t *testing.T) {
	sm := NewSessionManager()
	_, ok := sm.GetByJugglerSession("nope")
	if ok {
		t.Error("GetByJugglerSession returned true for nonexistent session")
	}
}

func TestSessionManager_All(t *testing.T) {
	sm := NewSessionManager()

	for i := 0; i < 5; i++ {
		sm.Add(&SessionInfo{
			SessionID: fmt.Sprintf("cdp-%d", i),
			TargetID:  fmt.Sprintf("target-%d", i),
		})
	}

	all := sm.All()
	if len(all) != 5 {
		t.Errorf("All() len = %d, want 5", len(all))
	}
}

func TestSessionManager_All_Empty(t *testing.T) {
	sm := NewSessionManager()
	all := sm.All()
	if len(all) != 0 {
		t.Errorf("All() len = %d, want 0", len(all))
	}
}

func TestSessionManager_GetBrowserContexts(t *testing.T) {
	sm := NewSessionManager()

	sm.Add(&SessionInfo{SessionID: "s1", TargetID: "t1", BrowserContextID: "ctx-a"})
	sm.Add(&SessionInfo{SessionID: "s2", TargetID: "t2", BrowserContextID: "ctx-b"})
	sm.Add(&SessionInfo{SessionID: "s3", TargetID: "t3", BrowserContextID: "ctx-a"}) // duplicate
	sm.Add(&SessionInfo{SessionID: "s4", TargetID: "t4", BrowserContextID: ""})      // empty

	contexts := sm.GetBrowserContexts()

	// Should have exactly 2 unique contexts (ctx-a, ctx-b)
	if len(contexts) != 2 {
		t.Fatalf("GetBrowserContexts() len = %d, want 2", len(contexts))
	}

	seen := map[string]bool{}
	for _, c := range contexts {
		seen[c] = true
	}
	if !seen["ctx-a"] {
		t.Error("missing ctx-a")
	}
	if !seen["ctx-b"] {
		t.Error("missing ctx-b")
	}
}

func TestSessionManager_GetBrowserContexts_Empty(t *testing.T) {
	sm := NewSessionManager()
	contexts := sm.GetBrowserContexts()
	if contexts != nil {
		t.Errorf("GetBrowserContexts() = %v, want nil for empty manager", contexts)
	}
}

func TestSessionManager_ConcurrentAccess(t *testing.T) {
	sm := NewSessionManager()

	const goroutines = 50
	const opsPerRoutine = 100

	var wg sync.WaitGroup
	for g := 0; g < goroutines; g++ {
		wg.Add(1)
		go func(gid int) {
			defer wg.Done()
			for i := 0; i < opsPerRoutine; i++ {
				sid := fmt.Sprintf("cdp-%d-%d", gid, i)
				tid := fmt.Sprintf("target-%d-%d", gid, i)
				jid := fmt.Sprintf("jug-%d-%d", gid, i)

				sm.Add(&SessionInfo{
					SessionID:        sid,
					JugglerSessionID: jid,
					TargetID:         tid,
					BrowserContextID: fmt.Sprintf("ctx-%d", gid),
				})

				// Read operations
				sm.Get(sid)
				sm.GetByTarget(tid)
				sm.GetByJugglerSession(jid)
				sm.All()
				sm.GetBrowserContexts()

				// Remove half the sessions
				if i%2 == 0 {
					sm.Remove(sid)
				}
			}
		}(g)
	}
	wg.Wait()

	// Verify no panic occurred and state is consistent
	all := sm.All()
	for _, info := range all {
		if _, ok := sm.Get(info.SessionID); !ok {
			t.Errorf("session %s in All() but not found via Get()", info.SessionID)
		}
	}
}

func TestSessionManager_OverwriteSession(t *testing.T) {
	sm := NewSessionManager()

	sm.Add(&SessionInfo{
		SessionID:        "cdp-1",
		JugglerSessionID: "jug-1",
		TargetID:         "target-1",
		URL:              "https://old.com",
	})
	sm.Add(&SessionInfo{
		SessionID:        "cdp-1",
		JugglerSessionID: "jug-2",
		TargetID:         "target-2",
		URL:              "https://new.com",
	})

	got, ok := sm.Get("cdp-1")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.URL != "https://new.com" {
		t.Errorf("URL = %q, want https://new.com", got.URL)
	}
	if got.JugglerSessionID != "jug-2" {
		t.Errorf("JugglerSessionID = %q, want jug-2", got.JugglerSessionID)
	}
}

func TestSessionManager_EmptyTargetAndJugglerSession(t *testing.T) {
	sm := NewSessionManager()

	// Session with no target or juggler session
	sm.Add(&SessionInfo{
		SessionID: "cdp-orphan",
	})

	got, ok := sm.Get("cdp-orphan")
	if !ok {
		t.Fatal("Get returned false")
	}
	if got.TargetID != "" {
		t.Errorf("TargetID = %q, want empty", got.TargetID)
	}

	// Should be able to remove cleanly
	sm.Remove("cdp-orphan")
	if _, ok := sm.Get("cdp-orphan"); ok {
		t.Error("orphan session still found after Remove")
	}
}
