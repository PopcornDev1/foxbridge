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
			Format                string `json:"format"`
			Quality               int    `json:"quality"`
			Clip                  *struct {
				X      float64 `json:"x"`
				Y      float64 `json:"y"`
				Width  float64 `json:"width"`
				Height float64 `json:"height"`
				Scale  float64 `json:"scale"`
			} `json:"clip"`
			FromSurface           bool `json:"fromSurface"`
			CaptureBeyondViewport bool `json:"captureBeyondViewport"`
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
		if params.CaptureBeyondViewport {
			jugglerParams["fullPage"] = true
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
		var params struct {
			Source string `json:"source"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		if params.Source != "" {
			result, err := b.callJuggler(msg.SessionID, "Page.addScriptToEvaluateOnNewDocument", map[string]interface{}{
				"script": params.Source,
			})
			if err != nil {
				log.Printf("[page] addScriptToEvaluateOnNewDocument failed: %v, returning stub", err)
				return marshalResult(map[string]string{"identifier": "1"})
			}
			// Juggler returns { scriptId }
			var scriptResult struct {
				ScriptID string `json:"scriptId"`
			}
			json.Unmarshal(result, &scriptResult)
			id := scriptResult.ScriptID
			if id == "" {
				id = "1"
			}
			return marshalResult(map[string]string{"identifier": id})
		}
		return marshalResult(map[string]string{"identifier": "1"})

	case "Page.createIsolatedWorld":
		var params struct {
			FrameID              string `json:"frameId"`
			WorldName            string `json:"worldName"`
			GrantUniversalAccess bool   `json:"grantUniveralAccess"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		// Allocate a unique numeric context ID
		ctxID := b.nextCtxID()

		// Map it to the MAIN world's Juggler execution context for this session.
		// Juggler doesn't have true isolated worlds — evaluations go through the same context.
		b.ctxMapMu.Lock()
		// Find the latest Juggler context for this session
		latestJugglerCtx := ""
		for _, v := range b.ctxMap {
			latestJugglerCtx = v
		}
		if latestJugglerCtx != "" {
			b.ctxMap[ctxID] = latestJugglerCtx
		}
		b.ctxMapMu.Unlock()

		uniqueID := fmt.Sprintf("isolated-%s-%s", params.FrameID, params.WorldName)

		// Emit Runtime.executionContextCreated so Puppeteer registers this world
		go func() {
			b.emitEvent("Runtime.executionContextCreated", map[string]interface{}{
				"context": map[string]interface{}{
					"id":       ctxID,
					"origin":   "",
					"name":     params.WorldName,
					"uniqueId": uniqueID,
					"auxData": map[string]interface{}{
						"isDefault": false,
						"type":      "isolated",
						"frameId":   params.FrameID,
					},
				},
			}, msg.SessionID)
		}()

		return marshalResult(map[string]interface{}{"executionContextId": ctxID})

	case "Page.setContent":
		// Puppeteer uses this to set page HTML content.
		// Juggler doesn't have a direct equivalent — use Runtime.evaluate.
		var params struct {
			HTML    string `json:"html"`
			FrameID string `json:"frameId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}
		if params.HTML == "" {
			return json.RawMessage(`{}`), nil
		}

		// Use document.open/write/close to set content
		expr := fmt.Sprintf(`(function() {
			document.open();
			document.write(%s);
			document.close();
		})()`, toJSString(params.HTML))

		_, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Page.getLayoutMetrics":
		// Return viewport metrics via Runtime.evaluate.
		expr := `JSON.stringify({
			width: window.innerWidth,
			height: window.innerHeight,
			devicePixelRatio: window.devicePixelRatio,
			scrollX: window.scrollX,
			scrollY: window.scrollY,
			docWidth: document.documentElement.scrollWidth,
			docHeight: document.documentElement.scrollHeight
		})`
		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			// Return sensible defaults
			return marshalResult(map[string]interface{}{
				"layoutViewport": map[string]interface{}{
					"pageX":          0,
					"pageY":          0,
					"clientWidth":    1280,
					"clientHeight":   720,
				},
				"visualViewport": map[string]interface{}{
					"offsetX":  0,
					"offsetY":  0,
					"pageX":    0,
					"pageY":    0,
					"clientWidth":  1280,
					"clientHeight": 720,
					"scale":    1,
					"zoom":     1,
				},
				"contentSize": map[string]interface{}{
					"x":      0,
					"y":      0,
					"width":  1280,
					"height": 720,
				},
				"cssLayoutViewport": map[string]interface{}{
					"clientWidth":  1280,
					"clientHeight": 720,
				},
				"cssVisualViewport": map[string]interface{}{
					"offsetX": 0,
					"offsetY": 0,
					"pageX":   0,
					"pageY":   0,
					"clientWidth":  1280,
					"clientHeight": 720,
				},
				"cssContentSize": map[string]interface{}{
					"x":      0,
					"y":      0,
					"width":  1280,
					"height": 720,
				},
			})
		}

		// Parse the evaluate result
		var evalResult struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		json.Unmarshal(result, &evalResult)

		var metrics struct {
			Width     float64 `json:"width"`
			Height    float64 `json:"height"`
			DPR       float64 `json:"devicePixelRatio"`
			ScrollX   float64 `json:"scrollX"`
			ScrollY   float64 `json:"scrollY"`
			DocWidth  float64 `json:"docWidth"`
			DocHeight float64 `json:"docHeight"`
		}
		if evalResult.Result.Value != nil {
			var strVal string
			if json.Unmarshal(evalResult.Result.Value, &strVal) == nil {
				json.Unmarshal([]byte(strVal), &metrics)
			} else {
				json.Unmarshal(evalResult.Result.Value, &metrics)
			}
		}
		if metrics.Width == 0 {
			metrics.Width = 1280
		}
		if metrics.Height == 0 {
			metrics.Height = 720
		}
		if metrics.DPR == 0 {
			metrics.DPR = 1
		}

		return marshalResult(map[string]interface{}{
			"layoutViewport": map[string]interface{}{
				"pageX":       metrics.ScrollX,
				"pageY":       metrics.ScrollY,
				"clientWidth":  metrics.Width,
				"clientHeight": metrics.Height,
			},
			"visualViewport": map[string]interface{}{
				"offsetX":      0,
				"offsetY":      0,
				"pageX":        metrics.ScrollX,
				"pageY":        metrics.ScrollY,
				"clientWidth":  metrics.Width,
				"clientHeight": metrics.Height,
				"scale":        1,
				"zoom":         metrics.DPR,
			},
			"contentSize": map[string]interface{}{
				"x":      0,
				"y":      0,
				"width":  metrics.DocWidth,
				"height": metrics.DocHeight,
			},
			"cssLayoutViewport": map[string]interface{}{
				"clientWidth":  metrics.Width,
				"clientHeight": metrics.Height,
			},
			"cssVisualViewport": map[string]interface{}{
				"offsetX":      0,
				"offsetY":      0,
				"pageX":        metrics.ScrollX,
				"pageY":        metrics.ScrollY,
				"clientWidth":  metrics.Width,
				"clientHeight": metrics.Height,
			},
			"cssContentSize": map[string]interface{}{
				"x":      0,
				"y":      0,
				"width":  metrics.DocWidth,
				"height": metrics.DocHeight,
			},
		})

	case "Page.handleJavaScriptDialog":
		var params struct {
			Accept     bool   `json:"accept"`
			PromptText string `json:"promptText"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		jugglerParams := map[string]interface{}{
			"accept": params.Accept,
		}
		if params.PromptText != "" {
			jugglerParams["promptText"] = params.PromptText
		}

		_, err := b.callJuggler(msg.SessionID, "Page.handleDialog", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Page.setBypassCSP":
		return json.RawMessage(`{}`), nil

	case "Page.bringToFront":
		return json.RawMessage(`{}`), nil

	case "Page.stopLoading":
		return json.RawMessage(`{}`), nil

	case "Page.getNavigationHistory":
		return marshalResult(map[string]interface{}{
			"currentIndex": 0,
			"entries": []map[string]interface{}{
				{
					"id":             0,
					"url":            "about:blank",
					"userTypedURL":   "about:blank",
					"title":          "",
					"transitionType": "typed",
				},
			},
		})

	case "Page.navigateToHistoryEntry":
		return json.RawMessage(`{}`), nil

	case "Page.printToPDF":
		var params struct {
			Landscape           bool    `json:"landscape"`
			DisplayHeaderFooter bool    `json:"displayHeaderFooter"`
			PrintBackground     bool    `json:"printBackground"`
			Scale               float64 `json:"scale"`
			PaperWidth          float64 `json:"paperWidth"`
			PaperHeight         float64 `json:"paperHeight"`
			MarginTop           float64 `json:"marginTop"`
			MarginBottom        float64 `json:"marginBottom"`
			MarginLeft          float64 `json:"marginLeft"`
			MarginRight         float64 `json:"marginRight"`
			HeaderTemplate      string  `json:"headerTemplate"`
			FooterTemplate      string  `json:"footerTemplate"`
			PreferCSSPageSize   bool    `json:"preferCSSPageSize"`
			PageRanges          string  `json:"pageRanges"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		jugglerParams := map[string]interface{}{}
		if params.Landscape {
			jugglerParams["landscape"] = true
		}
		if params.DisplayHeaderFooter {
			jugglerParams["displayHeaderFooter"] = true
		}
		if params.PrintBackground {
			jugglerParams["printBackground"] = true
		}
		if params.Scale != 0 {
			jugglerParams["scale"] = params.Scale
		}
		if params.PaperWidth != 0 {
			jugglerParams["paperWidth"] = params.PaperWidth
		}
		if params.PaperHeight != 0 {
			jugglerParams["paperHeight"] = params.PaperHeight
		}
		if params.MarginTop != 0 {
			jugglerParams["marginTop"] = params.MarginTop
		}
		if params.MarginBottom != 0 {
			jugglerParams["marginBottom"] = params.MarginBottom
		}
		if params.MarginLeft != 0 {
			jugglerParams["marginLeft"] = params.MarginLeft
		}
		if params.MarginRight != 0 {
			jugglerParams["marginRight"] = params.MarginRight
		}
		if params.HeaderTemplate != "" {
			jugglerParams["headerTemplate"] = params.HeaderTemplate
		}
		if params.FooterTemplate != "" {
			jugglerParams["footerTemplate"] = params.FooterTemplate
		}
		if params.PreferCSSPageSize {
			jugglerParams["preferCSSPageSize"] = true
		}
		if params.PageRanges != "" {
			jugglerParams["pageRanges"] = params.PageRanges
		}

		result, err := b.callJuggler(msg.SessionID, "Page.printToPDF", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		var pdfResult struct {
			Data string `json:"data"`
		}
		json.Unmarshal(result, &pdfResult)

		return marshalResult(map[string]interface{}{
			"data":   pdfResult.Data,
			"stream": nil,
		})

	case "Page.removeScriptToEvaluateOnNewDocument":
		var params struct {
			Identifier string `json:"identifier"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		if params.Identifier != "" {
			_, err := b.callJuggler(msg.SessionID, "Page.removeScriptToEvaluateOnNewDocument", map[string]interface{}{
				"scriptId": params.Identifier,
			})
			if err != nil {
				log.Printf("[page] removeScriptToEvaluateOnNewDocument failed: %v", err)
			}
		}
		return json.RawMessage(`{}`), nil

	case "Page.setExtraHTTPHeaders":
		var params struct {
			Headers map[string]string `json:"headers"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"headers": params.Headers,
		}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}

		_, err := b.callJuggler("", "Browser.setExtraHTTPHeaders", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Page.resetNavigationHistory":
		// No-op — Juggler does not support navigation history manipulation.
		return json.RawMessage(`{}`), nil

	case "Page.getResourceTree":
		frameID := ""
		pageURL := "about:blank"
		if info, ok := b.sessions.Get(msg.SessionID); ok {
			frameID = info.FrameID
			if info.URL != "" {
				pageURL = info.URL
			}
		}
		if frameID == "" {
			frameID = "main"
		}

		return marshalResult(map[string]interface{}{
			"frameTree": map[string]interface{}{
				"frame": map[string]interface{}{
					"id":             frameID,
					"loaderId":       "",
					"url":            pageURL,
					"securityOrigin": "",
					"mimeType":       "text/html",
				},
				"childFrames": []interface{}{},
				"resources":   []interface{}{},
			},
		})

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}

// toJSString converts a Go string to a JavaScript string literal (JSON-encoded).
func toJSString(s string) string {
	data, _ := json.Marshal(s)
	return string(data)
}
