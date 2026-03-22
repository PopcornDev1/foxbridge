package bridge

import (
	"encoding/json"
	"fmt"
	"log"
	"sync"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
	"github.com/google/uuid"
)

// targetPair holds the tab+page CDP session IDs for a Juggler target.
type targetPair struct {
	tabSessionID  string
	tabTargetID   string
	pageSessionID string
	pageTargetID  string
	browserCtxID  string
	url           string
}

// autoAttachState tracks auto-attach configuration and pending targets.
type autoAttachState struct {
	mu      sync.Mutex
	enabled bool
	// targets that arrived before setAutoAttach — need retroactive emission
	pending []*targetPair
	// all known pairs for lookup
	pairs map[string]*targetPair // keyed by juggler session ID
	// pendingFrameIDs stores frameIDs from executionContextCreated events that
	// fired before the CDP session was registered (keyed by Juggler session ID)
	pendingFrameIDs map[string]string
}

func newAutoAttachState() *autoAttachState {
	return &autoAttachState{
		pairs:           make(map[string]*targetPair),
		pendingFrameIDs: make(map[string]string),
	}
}

// SetupEventSubscriptions subscribes to Juggler events and translates them to CDP events.
func (b *Bridge) SetupEventSubscriptions() {
	// Browser.attachedToTarget — new page created, register session and emit CDP events.
	b.backend.Subscribe("Browser.attachedToTarget", func(sessionID string, params json.RawMessage) {
		log.Printf("[event] Browser.attachedToTarget received: %s", string(params))
		var ev struct {
			SessionID  string `json:"sessionId"`
			TargetInfo struct {
				TargetID         string `json:"targetId"`
				BrowserContextID string `json:"browserContextId"`
				Type             string `json:"type"`
				URL              string `json:"url"`
				OpenerId         string `json:"openerId"`
			} `json:"targetInfo"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Browser.attachedToTarget: %v", err)
			return
		}

		targetID := ev.TargetInfo.TargetID
		jugglerSessionID := ev.SessionID
		targetType := ev.TargetInfo.Type

		// Workers (worker/service_worker) use a single CDP session — no tab/page dual model.
		if targetType == "worker" || targetType == "service_worker" {
			workerSessionID := uuid.New().String()
			b.sessions.Add(&cdp.SessionInfo{
				SessionID:        workerSessionID,
				JugglerSessionID: jugglerSessionID,
				TargetID:         targetID,
				BrowserContextID: ev.TargetInfo.BrowserContextID,
				URL:              ev.TargetInfo.URL,
				Type:             targetType,
			})

			log.Printf("[event] registered %s target=%s session=%s", targetType, targetID, workerSessionID)

			b.autoAttach.mu.Lock()
			autoEnabled := b.autoAttach.enabled
			b.autoAttach.mu.Unlock()

			if autoEnabled {
				b.emitEvent("Target.attachedToTarget", map[string]interface{}{
					"sessionId": workerSessionID,
					"targetInfo": map[string]interface{}{
						"targetId":         targetID,
						"type":             targetType,
						"title":            "",
						"url":              ev.TargetInfo.URL,
						"attached":         true,
						"canAccessOpener":  false,
						"browserContextId": ev.TargetInfo.BrowserContextID,
					},
					"waitingForDebugger": false,
				}, "")
			}
			return
		}

		tabSessionID := uuid.New().String()
		pageSessionID := uuid.New().String()
		tabTargetID := uuid.New().String()

		pair := &targetPair{
			tabSessionID:  tabSessionID,
			tabTargetID:   tabTargetID,
			pageSessionID: pageSessionID,
			pageTargetID:  targetID,
			browserCtxID:  ev.TargetInfo.BrowserContextID,
			url:           ev.TargetInfo.URL,
		}

		// Register the PAGE session (what actually talks to Juggler)
		pageInfo := &cdp.SessionInfo{
			SessionID:        pageSessionID,
			JugglerSessionID: jugglerSessionID,
			TargetID:         targetID,
			BrowserContextID: ev.TargetInfo.BrowserContextID,
			URL:              ev.TargetInfo.URL,
			Type:             "page",
		}
		// Apply any pending frameID that was buffered before session registration
		b.autoAttach.mu.Lock()
		if pendingFrameID, ok := b.autoAttach.pendingFrameIDs[jugglerSessionID]; ok {
			pageInfo.FrameID = pendingFrameID
			delete(b.autoAttach.pendingFrameIDs, jugglerSessionID)
			log.Printf("[event] applied buffered frameID=%s to new session %s", pendingFrameID, pageSessionID)
		}
		b.autoAttach.mu.Unlock()
		b.sessions.Add(pageInfo)
		// Register the TAB session (stub — doesn't map to Juggler session to avoid
		// overwriting the PAGE session in jugglerSessions lookup)
		b.sessions.Add(&cdp.SessionInfo{
			SessionID:        tabSessionID,
			JugglerSessionID: "tab:" + jugglerSessionID,
			TargetID:         tabTargetID,
			BrowserContextID: ev.TargetInfo.BrowserContextID,
			URL:              ev.TargetInfo.URL,
			Type:             "tab",
		})

		b.autoAttach.mu.Lock()
		b.autoAttach.pairs[jugglerSessionID] = pair
		if b.autoAttach.enabled {
			// Auto-attach is active — emit TAB attachment immediately.
			// PAGE attachment will be emitted when Puppeteer sends setAutoAttach on the tab session.
			b.autoAttach.mu.Unlock()
			b.emitTabAttach(pair)
		} else {
			// Auto-attach not yet active — queue for later
			b.autoAttach.pending = append(b.autoAttach.pending, pair)
			b.autoAttach.mu.Unlock()
		}
	})

	// Browser.detachedFromTarget — page destroyed.
	b.backend.Subscribe("Browser.detachedFromTarget", func(sessionID string, params json.RawMessage) {
		var ev struct {
			SessionID string `json:"sessionId"`
			TargetID  string `json:"targetId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Browser.detachedFromTarget: %v", err)
			return
		}

		targetID := ev.TargetID
		// Find the CDP session for this target.
		info, ok := b.sessions.GetByTarget(targetID)
		if !ok {
			// Try by juggler session ID.
			info, ok = b.sessions.GetByJugglerSession(ev.SessionID)
		}

		cdpSessionID := ""
		if ok {
			cdpSessionID = info.SessionID
		}

		// Also find and clean up the tab session
		b.autoAttach.mu.Lock()
		if pair, exists := b.autoAttach.pairs[ev.SessionID]; exists {
			// Emit destroy for both tab and page
			b.autoAttach.mu.Unlock()

			b.emitEvent("Target.targetDestroyed", map[string]interface{}{
				"targetId": pair.pageTargetID,
			}, "")
			b.emitEvent("Target.targetDestroyed", map[string]interface{}{
				"targetId": pair.tabTargetID,
			}, "")

			if cdpSessionID != "" {
				b.emitEvent("Target.detachedFromTarget", map[string]interface{}{
					"sessionId": cdpSessionID,
					"targetId":  targetID,
				}, "")
			}

			b.sessions.Remove(pair.pageSessionID)
			b.sessions.Remove(pair.tabSessionID)

			b.autoAttach.mu.Lock()
			delete(b.autoAttach.pairs, ev.SessionID)
			b.autoAttach.mu.Unlock()
		} else {
			b.autoAttach.mu.Unlock()

			b.emitEvent("Target.targetDestroyed", map[string]interface{}{
				"targetId": targetID,
			}, "")

			if cdpSessionID != "" {
				b.emitEvent("Target.detachedFromTarget", map[string]interface{}{
					"sessionId": cdpSessionID,
					"targetId":  targetID,
				}, "")
				b.sessions.Remove(cdpSessionID)
			}
		}
	})

	// Page.navigationCommitted → Page.frameNavigated (session-scoped)
	b.backend.Subscribe("Page.navigationCommitted", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			FrameID      string `json:"frameId"`
			URL          string `json:"url"`
			Name         string `json:"name"`
			NavigationID string `json:"navigationId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Page.navigationCommitted: %v", err)
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		// Update session URL
		if info, ok := b.sessions.GetByJugglerSession(jugglerSessionID); ok {
			info.URL = ev.URL
		}

		// Use the Juggler navigationId as loaderId for consistency
		loaderId := ev.NavigationID
		if loaderId == "" {
			loaderId = fmt.Sprintf("loader-%s", jugglerSessionID[:8])
		}

		// Store loaderId so Page.eventFired can use the same one
		b.loaderMapMu.Lock()
		b.loaderMap[cdpSessionID] = loaderId
		b.loaderMapMu.Unlock()

		// Emit lifecycle events in Chrome's order
		b.emitEvent("Page.lifecycleEvent", map[string]interface{}{
			"frameId":   ev.FrameID,
			"loaderId":  loaderId,
			"name":      "init",
			"timestamp": 0,
		}, cdpSessionID)
		b.emitEvent("Page.lifecycleEvent", map[string]interface{}{
			"frameId":   ev.FrameID,
			"loaderId":  loaderId,
			"name":      "commit",
			"timestamp": 0,
		}, cdpSessionID)

		b.emitEvent("Page.frameNavigated", map[string]interface{}{
			"frame": map[string]interface{}{
				"id":                ev.FrameID,
				"url":               ev.URL,
				"loaderId":          loaderId,
				"securityOrigin":    "",
				"mimeType":          "text/html",
				"domainAndRegistry": "",
			},
			"type": "Navigation",
		}, cdpSessionID)
	})

	// Page.eventFired — maps to Page.loadEventFired or Page.domContentEventFired
	b.backend.Subscribe("Page.eventFired", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			Name    string `json:"name"`
			FrameID string `json:"frameId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Page.eventFired: %v", err)
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		// Use the same loaderId as the navigation that triggered this event
		b.loaderMapMu.RLock()
		loaderId := b.loaderMap[cdpSessionID]
		b.loaderMapMu.RUnlock()
		if loaderId == "" {
			loaderId = "loader-unknown"
		}

		switch ev.Name {
		case "load":
			b.emitEvent("Page.loadEventFired", map[string]interface{}{
				"timestamp": 0,
			}, cdpSessionID)
			b.emitEvent("Page.lifecycleEvent", map[string]interface{}{
				"frameId":   ev.FrameID,
				"loaderId":  loaderId,
				"name":      "load",
				"timestamp": 0,
			}, cdpSessionID)
			b.emitEvent("Page.frameStoppedLoading", map[string]interface{}{
				"frameId": ev.FrameID,
			}, cdpSessionID)
		case "DOMContentLoaded":
			b.emitEvent("Page.domContentEventFired", map[string]interface{}{
				"timestamp": 0,
			}, cdpSessionID)
			b.emitEvent("Page.lifecycleEvent", map[string]interface{}{
				"frameId":   ev.FrameID,
				"loaderId":  loaderId,
				"name":      "DOMContentLoaded",
				"timestamp": 0,
			}, cdpSessionID)
		}
	})

	// Runtime.executionContextsCleared → Runtime.executionContextsCleared
	// Also clear the ctxMap since all old context IDs are now stale
	b.backend.Subscribe("Runtime.executionContextsCleared", func(jugglerSessionID string, params json.RawMessage) {
		cdpSessionID := b.resolveCDPSession(jugglerSessionID)
		if cdpSessionID != "" {
			// Clear stale context mappings
			b.ctxMapMu.Lock()
			b.ctxMap = make(map[int]string)
			b.ctxMapMu.Unlock()

			b.emitEvent("Runtime.executionContextsCleared", map[string]interface{}{}, cdpSessionID)
		}
	})

	// Runtime.executionContextCreated → Runtime.executionContextCreated
	b.backend.Subscribe("Runtime.executionContextCreated", func(jugglerSessionID string, params json.RawMessage) {
		cdpSessionID := b.resolveCDPSession(jugglerSessionID)
		ctxID := b.nextCtxID()

		var ev struct {
			ExecutionContextID string `json:"executionContextId"`
			AuxData            struct {
				FrameID string `json:"frameId"`
				Name    string `json:"name"`
			} `json:"auxData"`
		}
		json.Unmarshal(params, &ev)

		// Store frame ID if not already set
		if ev.AuxData.FrameID != "" {
			if info, ok := b.sessions.GetByJugglerSession(jugglerSessionID); ok && info.FrameID == "" {
				info.FrameID = ev.AuxData.FrameID
				log.Printf("[event] stored frameID=%s for juggler session %s", ev.AuxData.FrameID, jugglerSessionID)
			} else if !ok {
				// Session not registered yet — buffer the frameId for later
				b.autoAttach.mu.Lock()
				if _, exists := b.autoAttach.pendingFrameIDs[jugglerSessionID]; !exists {
					b.autoAttach.pendingFrameIDs[jugglerSessionID] = ev.AuxData.FrameID
					log.Printf("[event] buffered pending frameID=%s for juggler session %s", ev.AuxData.FrameID, jugglerSessionID)
				}
				b.autoAttach.mu.Unlock()
			}
		}

		// Store the mapping: numeric CDP ID → Juggler string ID
		b.ctxMapMu.Lock()
		b.ctxMap[ctxID] = ev.ExecutionContextID
		b.ctxMapMu.Unlock()

		b.emitEvent("Runtime.executionContextCreated", map[string]interface{}{
			"context": map[string]interface{}{
				"id":       ctxID,
				"origin":   "",
				"name":     ev.AuxData.Name,
				"uniqueId": ev.ExecutionContextID,
				"auxData": map[string]interface{}{
					"isDefault": true,
					"type":      "default",
					"frameId":   ev.AuxData.FrameID,
				},
			},
		}, cdpSessionID)
	})

	// Runtime.executionContextDestroyed → Runtime.executionContextDestroyed
	b.backend.Subscribe("Runtime.executionContextDestroyed", func(jugglerSessionID string, params json.RawMessage) {
		cdpSessionID := b.resolveCDPSession(jugglerSessionID)
		var ev struct {
			ExecutionContextID string `json:"executionContextId"`
		}
		json.Unmarshal(params, &ev)

		// Find the numeric CDP context ID for this Juggler context
		var numericID int
		b.ctxMapMu.RLock()
		for k, v := range b.ctxMap {
			if v == ev.ExecutionContextID {
				numericID = k
				break
			}
		}
		b.ctxMapMu.RUnlock()

		// Clean up the mapping
		if numericID > 0 {
			b.ctxMapMu.Lock()
			delete(b.ctxMap, numericID)
			b.ctxMapMu.Unlock()
		}

		b.emitEvent("Runtime.executionContextDestroyed", map[string]interface{}{
			"executionContextId":       numericID,
			"executionContextUniqueId": ev.ExecutionContextID,
		}, cdpSessionID)
	})

	// Runtime.console → Runtime.consoleAPICalled
	b.backend.Subscribe("Runtime.console", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			Type string          `json:"type"`
			Args json.RawMessage `json:"args"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Runtime.console: %v", err)
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		b.emitEvent("Runtime.consoleAPICalled", map[string]interface{}{
			"type":               ev.Type,
			"args":               ev.Args,
			"executionContextId": 0,
			"timestamp":          0,
		}, cdpSessionID)
	})

	// Page.frameAttached → Page.frameAttached
	b.backend.Subscribe("Page.frameAttached", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			FrameID       string `json:"frameId"`
			ParentFrameID string `json:"parentFrameId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Page.frameAttached: %v", err)
			return
		}

		// Store the main frame ID (parentFrameId is empty for the main frame)
		if ev.ParentFrameID == "" && ev.FrameID != "" {
			if info, ok := b.sessions.GetByJugglerSession(jugglerSessionID); ok {
				info.FrameID = ev.FrameID
			}
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		b.emitEvent("Page.frameAttached", map[string]interface{}{
			"frameId":       ev.FrameID,
			"parentFrameId": ev.ParentFrameID,
		}, cdpSessionID)
	})

	// Page.frameDetached → Page.frameDetached
	b.backend.Subscribe("Page.frameDetached", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			FrameID string `json:"frameId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Page.frameDetached: %v", err)
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		b.emitEvent("Page.frameDetached", map[string]interface{}{
			"frameId": ev.FrameID,
			"reason":  "remove",
		}, cdpSessionID)
	})

	// Page.dialogOpened → Page.javascriptDialogOpening
	b.backend.Subscribe("Page.dialogOpened", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			Type         string `json:"type"`
			Message      string `json:"message"`
			DefaultValue string `json:"defaultValue"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Page.dialogOpened: %v", err)
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		b.emitEvent("Page.javascriptDialogOpening", map[string]interface{}{
			"type":               ev.Type,
			"message":            ev.Message,
			"defaultPrompt":      ev.DefaultValue,
			"hasBrowserHandler":  false,
			"url":                "",
		}, cdpSessionID)
	})

	// Page.dialogClosed → Page.javascriptDialogClosed
	b.backend.Subscribe("Page.dialogClosed", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			Accepted bool `json:"accepted"`
		}
		json.Unmarshal(params, &ev)

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		b.emitEvent("Page.javascriptDialogClosed", map[string]interface{}{
			"result":    ev.Accepted,
			"userInput": "",
		}, cdpSessionID)
	})

	// Network.requestWillBeSent → Network.requestWillBeSent
	b.backend.Subscribe("Network.requestWillBeSent", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			RequestID    string `json:"requestId"`
			FrameID      string `json:"frameId"`
			URL          string `json:"url"`
			Method       string `json:"method"`
			Headers      map[string]string `json:"headers"`
			IsNavigation bool   `json:"isNavigationRequest"`
			RedirectURL  string `json:"redirectedFrom"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		cdpHeaders := map[string]string{}
		for k, v := range ev.Headers {
			cdpHeaders[k] = v
		}

		b.emitEvent("Network.requestWillBeSent", map[string]interface{}{
			"requestId": ev.RequestID,
			"loaderId":  ev.RequestID,
			"documentURL": ev.URL,
			"request": map[string]interface{}{
				"url":             ev.URL,
				"method":          ev.Method,
				"headers":         cdpHeaders,
				"initialPriority": "High",
				"referrerPolicy":  "strict-origin-when-cross-origin",
			},
			"timestamp": 0,
			"wallTime":  0,
			"initiator": map[string]interface{}{
				"type": "other",
			},
			"type":    "Document",
			"frameId": ev.FrameID,
		}, cdpSessionID)
	})

	// Network.responseReceived → Network.responseReceived
	b.backend.Subscribe("Network.responseReceived", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			RequestID  string `json:"requestId"`
			SecurityDetails json.RawMessage `json:"securityDetails"`
			FromCache  bool   `json:"fromCache"`
			Headers    map[string]string `json:"headers"`
			Status     int    `json:"status"`
			StatusText string `json:"statusText"`
			URL        string `json:"url"`
			FrameID    string `json:"frameId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		b.emitEvent("Network.responseReceived", map[string]interface{}{
			"requestId": ev.RequestID,
			"loaderId":  ev.RequestID,
			"timestamp": 0,
			"type":      "Document",
			"response": map[string]interface{}{
				"url":                ev.URL,
				"status":             ev.Status,
				"statusText":         ev.StatusText,
				"headers":            ev.Headers,
				"mimeType":           "",
				"connectionReused":   false,
				"connectionId":       0,
				"encodedDataLength":  0,
				"fromDiskCache":      ev.FromCache,
				"fromServiceWorker":  false,
				"fromPrefetchCache":  false,
				"securityState":      "secure",
			},
			"frameId": ev.FrameID,
		}, cdpSessionID)
	})

	// Network.requestFinished → Network.loadingFinished
	b.backend.Subscribe("Network.requestFinished", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			RequestID string `json:"requestId"`
		}
		json.Unmarshal(params, &ev)

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		b.emitEvent("Network.loadingFinished", map[string]interface{}{
			"requestId":         ev.RequestID,
			"timestamp":         0,
			"encodedDataLength": 0,
		}, cdpSessionID)
	})

	// Network.requestFailed → Network.loadingFailed
	b.backend.Subscribe("Network.requestFailed", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			RequestID    string `json:"requestId"`
			ErrorCode    string `json:"errorCode"`
		}
		json.Unmarshal(params, &ev)

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		b.emitEvent("Network.loadingFailed", map[string]interface{}{
			"requestId":    ev.RequestID,
			"timestamp":    0,
			"type":         "Document",
			"errorText":    ev.ErrorCode,
			"canceled":     false,
		}, cdpSessionID)
	})

	// Browser.requestIntercepted → Fetch.requestPaused
	b.backend.Subscribe("Browser.requestIntercepted", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			RequestID string `json:"requestId"`
			Request   struct {
				URL     string            `json:"url"`
				Method  string            `json:"method"`
				Headers map[string]string `json:"headers"`
			} `json:"request"`
			FrameID              string `json:"frameId"`
			IsNavigationRequest  bool   `json:"isNavigationRequest"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Browser.requestIntercepted: %v", err)
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		// Determine resource type from navigation flag
		resourceType := "Other"
		if ev.IsNavigationRequest {
			resourceType = "Document"
		}

		b.emitEvent("Fetch.requestPaused", map[string]interface{}{
			"requestId": ev.RequestID,
			"request": map[string]interface{}{
				"url":     ev.Request.URL,
				"method":  ev.Request.Method,
				"headers": ev.Request.Headers,
			},
			"frameId":      ev.FrameID,
			"resourceType": resourceType,
		}, cdpSessionID)
	})
}

// emitTabAttach emits ONLY the tab-level attachment on the browser session.
// The page-level attachment is deferred until Puppeteer sends setAutoAttach on the tab session.
func (b *Bridge) emitTabAttach(pair *targetPair) {
	b.emitEvent("Target.attachedToTarget", map[string]interface{}{
		"sessionId": pair.tabSessionID,
		"targetInfo": map[string]interface{}{
			"targetId":         pair.tabTargetID,
			"type":             "tab",
			"title":            "",
			"url":              pair.url,
			"attached":         true,
			"canAccessOpener":  false,
			"browserContextId": pair.browserCtxID,
		},
		"waitingForDebugger": true,
	}, "")
}

// emitPageAttach emits the page-level attachment on a tab session.
func (b *Bridge) emitPageAttach(pair *targetPair) {
	url := pair.url
	if url == "" {
		url = "about:blank"
	}
	b.emitEvent("Target.attachedToTarget", map[string]interface{}{
		"sessionId": pair.pageSessionID,
		"targetInfo": map[string]interface{}{
			"targetId":         pair.pageTargetID,
			"type":             "page",
			"title":            "",
			"url":              url,
			"attached":         true,
			"canAccessOpener":  false,
			"browserContextId": pair.browserCtxID,
		},
		"waitingForDebugger": true,
	}, pair.tabSessionID)
}

// resolveCDPSession maps a Juggler sessionID to a CDP sessionID.
// For page-level events, we want the PAGE session (not the tab).
func (b *Bridge) resolveCDPSession(jugglerSessionID string) string {
	if jugglerSessionID == "" {
		return ""
	}
	// Look up the pair to get the page session ID
	b.autoAttach.mu.Lock()
	pair, ok := b.autoAttach.pairs[jugglerSessionID]
	b.autoAttach.mu.Unlock()
	if ok {
		return pair.pageSessionID
	}
	// Fallback to session manager
	if info, ok := b.sessions.GetByJugglerSession(jugglerSessionID); ok {
		return info.SessionID
	}
	return ""
}

// emitEventRaw sends a CDP event with raw JSON params.
func (b *Bridge) emitEventRaw(method string, params json.RawMessage, sessionID string) {
	b.server.Broadcast(&cdp.Message{
		Method:    method,
		Params:    params,
		SessionID: sessionID,
	})
}
