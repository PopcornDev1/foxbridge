package bridge

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
	"github.com/google/uuid"
)

func (b *Bridge) handleTarget(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Target.setDiscoverTargets":
		// Emit targetCreated for all known targets (both tabs and pages).
		for _, info := range b.sessions.All() {
			url := info.URL
			if url == "" && info.Type == "page" {
				url = "about:blank"
			}
			b.emitEvent("Target.targetCreated", map[string]interface{}{
				"targetInfo": map[string]interface{}{
					"targetId":         info.TargetID,
					"type":             info.Type,
					"title":            info.Title,
					"url":              url,
					"attached":         true,
					"canAccessOpener":  false,
					"browserContextId": info.BrowserContextID,
				},
			}, "")
		}
		return json.RawMessage(`{}`), nil

	case "Target.setAutoAttach":
		var params struct {
			AutoAttach             bool `json:"autoAttach"`
			WaitForDebuggerOnStart bool `json:"waitForDebuggerOnStart"`
			Flatten                bool `json:"flatten"`
		}
		json.Unmarshal(msg.Params, &params)

		if msg.SessionID == "" {
			// Browser-level setAutoAttach: emit TAB attachedToTarget for all pending targets.
			// The PAGE attachment happens when Puppeteer sends setAutoAttach on the tab session.
			b.autoAttach.mu.Lock()
			b.autoAttach.enabled = params.AutoAttach
			pending := b.autoAttach.pending
			b.autoAttach.pending = nil
			b.autoAttach.mu.Unlock()

			if params.AutoAttach {
				log.Printf("[target] setAutoAttach on browser session, emitting %d pending tab targets", len(pending))
				for _, pair := range pending {
					b.emitTabAttach(pair)
				}
			}
		} else {
			// Session-level setAutoAttach (tab or page session).
			// If this is a tab session, emit the page attachment.
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.Type == "tab" {
				// Find the page pair for this tab
				b.autoAttach.mu.Lock()
				for _, pair := range b.autoAttach.pairs {
					if pair.tabSessionID == msg.SessionID {
						b.autoAttach.mu.Unlock()
						b.emitPageAttach(pair)
						goto autoAttachDone
					}
				}
				b.autoAttach.mu.Unlock()
			}
			// Page-session or no match: no-op
		}
	autoAttachDone:
		return json.RawMessage(`{}`), nil

	case "Target.createTarget":
		var params struct {
			URL              string `json:"url"`
			BrowserContextID string `json:"browserContextId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{}
		if params.BrowserContextID != "" {
			jugglerParams["browserContextId"] = params.BrowserContextID
		}

		result, err := b.callJuggler("", "Browser.newPage", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		// Juggler returns { targetId }
		log.Printf("[target] Browser.newPage response: %s", string(result))
		var pageResult struct {
			TargetID  string `json:"targetId"`
		}
		json.Unmarshal(result, &pageResult)

		targetID := pageResult.TargetID
		if targetID == "" {
			targetID = uuid.New().String()
		}

		// Return the PAGE targetId (not the tab).
		// Puppeteer's waitForTarget matches on this ID, and TAB targets are filtered
		// out by _isTargetExposed(). The tab attachment happens via the event handler.
		log.Printf("[target] createTarget returning page targetId=%s", targetID)
		return marshalResult(map[string]string{"targetId": targetID})

	case "Target.closeTarget":
		var params struct {
			TargetID string `json:"targetId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		info, ok := b.sessions.GetByTarget(params.TargetID)
		if !ok {
			return nil, &cdp.Error{Code: -32000, Message: fmt.Sprintf("target %s not found", params.TargetID)}
		}

		_, err := b.callJuggler(info.SessionID, "Page.close", nil)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		b.sessions.Remove(info.SessionID)

		return json.RawMessage(`{"success":true}`), nil

	case "Target.createBrowserContext":
		result, err := b.callJuggler("", "Browser.createBrowserContext", nil)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		var ctxResult struct {
			BrowserContextID string `json:"browserContextId"`
		}
		json.Unmarshal(result, &ctxResult)

		return marshalResult(map[string]string{"browserContextId": ctxResult.BrowserContextID})

	case "Target.disposeBrowserContext":
		var params struct {
			BrowserContextID string `json:"browserContextId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		_, err := b.callJuggler("", "Browser.removeBrowserContext", map[string]string{
			"browserContextId": params.BrowserContextID,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Target.getTargets":
		targets := []map[string]interface{}{}
		for _, s := range b.sessions.All() {
			targets = append(targets, map[string]interface{}{
				"targetId":         s.TargetID,
				"type":             s.Type,
				"title":            s.Title,
				"url":              s.URL,
				"attached":         true,
				"browserContextId": s.BrowserContextID,
			})
		}
		return marshalResult(map[string]interface{}{"targetInfos": targets})

	case "Target.attachToTarget":
		var params struct {
			TargetID string `json:"targetId"`
			Flatten  bool   `json:"flatten"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Check if we already have a session for this target.
		if info, ok := b.sessions.GetByTarget(params.TargetID); ok {
			return marshalResult(map[string]string{"sessionId": info.SessionID})
		}

		// Create a new CDP session for this target.
		sessionID := uuid.New().String()
		b.sessions.Add(&cdp.SessionInfo{
			SessionID:        sessionID,
			JugglerSessionID: params.TargetID, // use targetID as juggler session
			TargetID:         params.TargetID,
			Type:             "page",
		})

		// Emit Target.attachedToTarget event.
		b.emitEvent("Target.attachedToTarget", map[string]interface{}{
			"sessionId": sessionID,
			"targetInfo": map[string]interface{}{
				"targetId": params.TargetID,
				"type":     "page",
				"title":    "",
				"url":      "",
				"attached": true,
			},
			"waitingForDebugger": false,
		}, "")

		return marshalResult(map[string]string{"sessionId": sessionID})

	case "Target.activateTarget":
		return json.RawMessage(`{}`), nil

	case "Target.getBrowserContexts":
		contexts := b.sessions.GetBrowserContexts()
		if contexts == nil {
			contexts = []string{}
		}
		// Puppeteer expects defaultBrowserContextId to be present
		defaultCtxID := ""
		if len(contexts) > 0 {
			defaultCtxID = contexts[0]
		}
		return marshalResult(map[string]interface{}{
			"browserContextIds":      contexts,
			"defaultBrowserContextId": defaultCtxID,
		})

	case "Target.getTargetInfo":
		var params struct {
			TargetID string `json:"targetId"`
		}
		json.Unmarshal(msg.Params, &params)
		if info, ok := b.sessions.GetByTarget(params.TargetID); ok {
			return marshalResult(map[string]interface{}{
				"targetInfo": map[string]interface{}{
					"targetId":         info.TargetID,
					"type":             info.Type,
					"title":            info.Title,
					"url":              info.URL,
					"attached":         true,
					"browserContextId": info.BrowserContextID,
				},
			})
		}
		return nil, &cdp.Error{Code: -32000, Message: "target not found"}

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}

func marshalResult(v interface{}) (json.RawMessage, *cdp.Error) {
	data, err := json.Marshal(v)
	if err != nil {
		return nil, &cdp.Error{Code: -32000, Message: err.Error()}
	}
	return data, nil
}
