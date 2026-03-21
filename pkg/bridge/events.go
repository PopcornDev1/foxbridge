package bridge

import (
	"encoding/json"
	"log"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
	"github.com/google/uuid"
)

// SetupEventSubscriptions subscribes to Juggler events and translates them to CDP events.
func (b *Bridge) SetupEventSubscriptions() {
	// Browser.attachedToTarget — new page created, register session and emit CDP events.
	b.backend.Subscribe("Browser.attachedToTarget", func(sessionID string, params json.RawMessage) {
		var ev struct {
			SessionID        string `json:"sessionId"`
			TargetInfo       struct {
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
		if targetType == "" {
			targetType = "page"
		}

		// Create a CDP session for this target.
		cdpSessionID := uuid.New().String()
		b.sessions.Add(&cdp.SessionInfo{
			SessionID:        cdpSessionID,
			JugglerSessionID: jugglerSessionID,
			TargetID:         targetID,
			BrowserContextID: ev.TargetInfo.BrowserContextID,
			URL:              ev.TargetInfo.URL,
			Type:             targetType,
		})

		// Emit Target.targetCreated
		b.emitEvent("Target.targetCreated", map[string]interface{}{
			"targetInfo": map[string]interface{}{
				"targetId":         targetID,
				"type":             targetType,
				"title":            "",
				"url":              ev.TargetInfo.URL,
				"attached":         true,
				"browserContextId": ev.TargetInfo.BrowserContextID,
				"openerId":         ev.TargetInfo.OpenerId,
			},
		}, "")

		// Emit Target.attachedToTarget
		b.emitEvent("Target.attachedToTarget", map[string]interface{}{
			"sessionId": cdpSessionID,
			"targetInfo": map[string]interface{}{
				"targetId":         targetID,
				"type":             targetType,
				"title":            "",
				"url":              ev.TargetInfo.URL,
				"attached":         true,
				"browserContextId": ev.TargetInfo.BrowserContextID,
			},
			"waitingForDebugger": false,
		}, "")
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

		// Emit Target.targetDestroyed
		b.emitEvent("Target.targetDestroyed", map[string]interface{}{
			"targetId": targetID,
		}, "")

		// Emit Target.detachedFromTarget
		if cdpSessionID != "" {
			b.emitEvent("Target.detachedFromTarget", map[string]interface{}{
				"sessionId": cdpSessionID,
				"targetId":  targetID,
			}, "")
			b.sessions.Remove(cdpSessionID)
		}
	})

	// Page.navigationCommitted → Page.frameNavigated (session-scoped)
	b.backend.Subscribe("Page.navigationCommitted", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			FrameID string `json:"frameId"`
			URL     string `json:"url"`
			Name    string `json:"name"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Page.navigationCommitted: %v", err)
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		// Update session URL if this is the main frame.
		if info, ok := b.sessions.GetByJugglerSession(jugglerSessionID); ok {
			info.URL = ev.URL
		}

		b.emitEvent("Page.frameNavigated", map[string]interface{}{
			"frame": map[string]interface{}{
				"id":             ev.FrameID,
				"url":            ev.URL,
				"loaderId":       "",
				"securityOrigin": "",
				"mimeType":       "text/html",
				"domainAndRegistry": "",
			},
			"type": "Navigation",
		}, cdpSessionID)
	})

	// Page.eventFired — maps to Page.loadEventFired or Page.domContentEventFired
	b.backend.Subscribe("Page.eventFired", func(jugglerSessionID string, params json.RawMessage) {
		var ev struct {
			Name    string  `json:"name"`
			FrameID string  `json:"frameId"`
		}
		if err := json.Unmarshal(params, &ev); err != nil {
			log.Printf("events: failed to parse Page.eventFired: %v", err)
			return
		}

		cdpSessionID := b.resolveCDPSession(jugglerSessionID)

		switch ev.Name {
		case "load":
			b.emitEvent("Page.loadEventFired", map[string]interface{}{
				"timestamp": 0,
			}, cdpSessionID)
		case "DOMContentLoaded":
			b.emitEvent("Page.domContentEventFired", map[string]interface{}{
				"timestamp": 0,
			}, cdpSessionID)
		}
	})

	// Runtime.executionContextCreated → Runtime.executionContextCreated
	b.backend.Subscribe("Runtime.executionContextCreated", func(jugglerSessionID string, params json.RawMessage) {
		cdpSessionID := b.resolveCDPSession(jugglerSessionID)
		// Pass through — Juggler format is close enough to CDP.
		b.emitEventRaw("Runtime.executionContextCreated", params, cdpSessionID)
	})

	// Runtime.executionContextDestroyed → Runtime.executionContextDestroyed
	b.backend.Subscribe("Runtime.executionContextDestroyed", func(jugglerSessionID string, params json.RawMessage) {
		cdpSessionID := b.resolveCDPSession(jugglerSessionID)
		b.emitEventRaw("Runtime.executionContextDestroyed", params, cdpSessionID)
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
			"type":            ev.Type,
			"args":            ev.Args,
			"executionContextId": 0,
			"timestamp":       0,
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
}

// resolveCDPSession maps a Juggler sessionID to a CDP sessionID.
func (b *Bridge) resolveCDPSession(jugglerSessionID string) string {
	if jugglerSessionID == "" {
		return ""
	}
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
