package bridge

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func (b *Bridge) handlePage(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Page.enable", "Page.setLifecycleEventsEnabled":
		// No-op — Juggler always emits lifecycle events.
		return json.RawMessage(`{}`), nil

	case "Page.navigate":
		var params struct {
			URL            string `json:"url"`
			Referrer       string `json:"referrer"`
			TransitionType string `json:"transitionType"`
			FrameID        string `json:"frameId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"url": params.URL,
		}
		if params.Referrer != "" {
			jugglerParams["referer"] = params.Referrer
		}
		if params.FrameID != "" {
			jugglerParams["frameId"] = params.FrameID
		}

		// Use the stored frameId if available, otherwise try to discover it
		if _, hasFrame := jugglerParams["frameId"]; !hasFrame || jugglerParams["frameId"] == "main" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.FrameID != "" {
				jugglerParams["frameId"] = info.FrameID
			}
		}

		jp, _ := json.Marshal(jugglerParams)
		log.Printf("[page] navigate: params=%s cdpSession=%s", string(jp), msg.SessionID)
		result, err := b.callJuggler(msg.SessionID, "Page.navigate", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		// Juggler returns { navigationId, frameId }. CDP expects { frameId, loaderId }.
		var navResult struct {
			NavigationID string `json:"navigationId"`
			FrameID      string `json:"frameId"`
		}
		json.Unmarshal(result, &navResult)

		return marshalResult(map[string]interface{}{
			"frameId":  navResult.FrameID,
			"loaderId": navResult.NavigationID,
		})

	case "Page.reload":
		_, err := b.callJuggler(msg.SessionID, "Page.reload", nil)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Page.close":
		_, err := b.callJuggler(msg.SessionID, "Page.close", nil)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Page.captureScreenshot":
		var params struct {
			Format      string `json:"format"`
			Quality     int    `json:"quality"`
			Clip        *struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
				Scale  float64 `json:"scale"`
			} `json:"clip"`
			FromSurface bool `json:"fromSurface"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		jugglerParams := map[string]interface{}{}
		if params.Format != "" {
			mimeType := "image/png"
			if params.Format == "jpeg" || params.Format == "jpg" {
				mimeType = "image/jpeg"
			}
			jugglerParams["mimeType"] = mimeType
		}
		if params.Clip != nil {
			jugglerParams["clip"] = map[string]interface{}{
				"x":      params.Clip.X,
				"y":      params.Clip.Y,
				"width":  params.Clip.Width,
				"height": params.Clip.Height,
			}
		}

		result, err := b.callJuggler(msg.SessionID, "Page.screenshot", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		// Juggler returns { data }. CDP expects { data }.
		var ssResult struct {
			Data string `json:"data"`
		}
		json.Unmarshal(result, &ssResult)

		return marshalResult(map[string]string{"data": ssResult.Data})

	case "Page.getFrameTree":
		// Look up the real frame ID from the session (stored from events)
		frameID := ""
		pageURL := "about:blank"
		if info, ok := b.sessions.Get(msg.SessionID); ok {
			frameID = info.FrameID
			if info.URL != "" {
				pageURL = info.URL
			}
		}

		// If frameID is not yet known, trigger a page reload to generate navigation events
		// that include the frame ID, or query the page to discover it
		if frameID == "" {
			log.Printf("[page] getFrameTree: no frameID, calling Accessibility.getFullAXTree to trigger content process init")
			// Call a method that goes through the content process, which triggers
			// execution context events that include the frameId.
			_, probeErr := b.callJuggler(msg.SessionID, "Accessibility.getFullAXTree", map[string]interface{}{})
			if probeErr != nil {
				log.Printf("[page] getFrameTree: AX tree probe failed: %v", probeErr)
			}
			// After the call, check if frameId was stored from triggered events
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.FrameID != "" {
				frameID = info.FrameID
				log.Printf("[page] getFrameTree: discovered frameID=%s via AX tree probe", frameID)
			}
		}

		// Last resort: if still no frameID, fall back to a placeholder
		if frameID == "" {
			frameID = "main"
			log.Printf("[page] getFrameTree: WARNING no frameId available for session %s, using placeholder", msg.SessionID)
		}

		return marshalResult(map[string]interface{}{
			"frameTree": map[string]interface{}{
				"frame": map[string]interface{}{
					"id":                frameID,
					"loaderId":          "",
					"url":               pageURL,
					"securityOrigin":    "",
					"mimeType":          "text/html",
					"domainAndRegistry": "",
					"secureContextType": "InsecureScheme",
					"crossOriginIsolatedContextType": "NotIsolated",
					"gatedAPIFeatures": []string{},
				},
				"childFrames": []interface{}{},
			},
		})

	case "Page.setInterceptFileChooserDialog":
		return json.RawMessage(`{}`), nil

	case "Page.addScriptToEvaluateOnNewDocument":
		return marshalResult(map[string]string{"identifier": "1"})

	case "Page.createIsolatedWorld":
		return marshalResult(map[string]interface{}{"executionContextId": 1})

	case "Page.setBypassCSP":
		return json.RawMessage(`{}`), nil

	case "Page.bringToFront":
		return json.RawMessage(`{}`), nil

	case "Page.stopLoading":
		return json.RawMessage(`{}`), nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
