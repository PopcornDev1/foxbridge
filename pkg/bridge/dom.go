package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func (b *Bridge) handleDOM(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "DOM.enable", "DOM.disable":
		return json.RawMessage(`{}`), nil

	case "DOM.getDocument":
		// Evaluate to get document info via Runtime.evaluate.
		expr := `(function() {
			return JSON.stringify({
				title: document.title,
				url: document.location.href,
				baseURL: document.baseURI
			});
		})()`

		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			// Fallback to a minimal document node.
			return marshalResult(map[string]interface{}{
				"root": map[string]interface{}{
					"nodeId":         1,
					"backendNodeId":  1,
					"nodeType":       9,
					"nodeName":       "#document",
					"localName":      "",
					"nodeValue":      "",
					"childNodeCount": 1,
					"documentURL":    "",
					"baseURL":        "",
					"children": []interface{}{
						map[string]interface{}{
							"nodeId":         2,
							"backendNodeId":  2,
							"nodeType":       1,
							"nodeName":       "HTML",
							"localName":      "html",
							"nodeValue":      "",
							"childNodeCount": 2,
						},
					},
				},
			})
		}

		// Parse the evaluate result to extract document info.
		var evalResult struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		json.Unmarshal(result, &evalResult)

		var docInfo struct {
			Title   string `json:"title"`
			URL     string `json:"url"`
			BaseURL string `json:"baseURL"`
		}
		if evalResult.Result.Value != nil {
			// Value may be a string (JSON-encoded) or an object.
			var strVal string
			if json.Unmarshal(evalResult.Result.Value, &strVal) == nil {
				json.Unmarshal([]byte(strVal), &docInfo)
			} else {
				json.Unmarshal(evalResult.Result.Value, &docInfo)
			}
		}

		return marshalResult(map[string]interface{}{
			"root": map[string]interface{}{
				"nodeId":         1,
				"backendNodeId":  1,
				"nodeType":       9,
				"nodeName":       "#document",
				"localName":      "",
				"nodeValue":      "",
				"childNodeCount": 1,
				"documentURL":    docInfo.URL,
				"baseURL":        docInfo.BaseURL,
				"children": []interface{}{
					map[string]interface{}{
						"nodeId":         2,
						"backendNodeId":  2,
						"nodeType":       1,
						"nodeName":       "HTML",
						"localName":      "html",
						"nodeValue":      "",
						"childNodeCount": 2,
					},
				},
			},
		})

	case "DOM.querySelector":
		var params struct {
			NodeID   int    `json:"nodeId"`
			Selector string `json:"selector"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		expr := fmt.Sprintf(`document.querySelector(%q) !== null ? 3 : 0`, params.Selector)
		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		var evalResult struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		json.Unmarshal(result, &evalResult)

		var nodeID int
		if evalResult.Result.Value != nil {
			json.Unmarshal(evalResult.Result.Value, &nodeID)
		}

		return marshalResult(map[string]interface{}{
			"nodeId": nodeID,
		})

	case "DOM.getAttributes":
		var params struct {
			NodeID int `json:"nodeId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Without real node references, return empty attributes.
		// A full implementation would require a node ID registry.
		return marshalResult(map[string]interface{}{
			"attributes": []string{},
		})

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
