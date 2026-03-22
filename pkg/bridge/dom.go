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

		// Check if element exists
		expr := fmt.Sprintf(`document.querySelector(%q) !== null`, params.Selector)
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

		var found bool
		if evalResult.Result.Value != nil {
			json.Unmarshal(evalResult.Result.Value, &found)
		}

		nodeID := 0
		if found {
			// Allocate a unique node ID using the context counter
			nodeID = b.nextCtxID()
		}

		return marshalResult(map[string]interface{}{
			"nodeId": nodeID,
		})

	case "DOM.querySelectorAll":
		var params struct {
			NodeID   int    `json:"nodeId"`
			Selector string `json:"selector"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Return node IDs starting from 3 for each match
		expr := fmt.Sprintf(`document.querySelectorAll(%q).length`, params.Selector)
		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			return marshalResult(map[string]interface{}{"nodeIds": []int{}})
		}

		var evalResult struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		json.Unmarshal(result, &evalResult)

		var count int
		if evalResult.Result.Value != nil {
			json.Unmarshal(evalResult.Result.Value, &count)
		}

		nodeIDs := make([]int, count)
		for i := range nodeIDs {
			nodeIDs[i] = 3 + i
		}
		return marshalResult(map[string]interface{}{"nodeIds": nodeIDs})

	case "DOM.describeNode":
		var params struct {
			NodeID        int    `json:"nodeId"`
			BackendNodeID int    `json:"backendNodeId"`
			ObjectID      string `json:"objectId"`
			Depth         int    `json:"depth"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		// If we have an objectId, get info via Runtime.callFunction
		if params.ObjectID != "" {
			expr := `function() {
				var n = this;
				return JSON.stringify({
					nodeType: n.nodeType,
					nodeName: n.nodeName,
					localName: n.localName || '',
					nodeValue: n.nodeValue || '',
					childCount: n.childNodes ? n.childNodes.length : 0,
					attrs: (function() {
						var a = [];
						if (n.attributes) {
							for (var i = 0; i < n.attributes.length; i++) {
								a.push(n.attributes[i].name, n.attributes[i].value);
							}
						}
						return a;
					})()
				});
			}`
			result, err := b.callJuggler(msg.SessionID, "Runtime.callFunction", map[string]interface{}{
				"functionDeclaration": expr,
				"objectId":           params.ObjectID,
				"returnByValue":      true,
			})
			if err == nil {
				var callResult struct {
					Result struct {
						Value json.RawMessage `json:"value"`
					} `json:"result"`
				}
				json.Unmarshal(result, &callResult)

				var nodeInfo struct {
					NodeType   int      `json:"nodeType"`
					NodeName   string   `json:"nodeName"`
					LocalName  string   `json:"localName"`
					NodeValue  string   `json:"nodeValue"`
					ChildCount int      `json:"childCount"`
					Attrs      []string `json:"attrs"`
				}
				if callResult.Result.Value != nil {
					var strVal string
					if json.Unmarshal(callResult.Result.Value, &strVal) == nil {
						json.Unmarshal([]byte(strVal), &nodeInfo)
					} else {
						json.Unmarshal(callResult.Result.Value, &nodeInfo)
					}
				}

				return marshalResult(map[string]interface{}{
					"node": map[string]interface{}{
						"nodeId":         params.NodeID,
						"backendNodeId":  params.BackendNodeID,
						"nodeType":       nodeInfo.NodeType,
						"nodeName":       nodeInfo.NodeName,
						"localName":      nodeInfo.LocalName,
						"nodeValue":      nodeInfo.NodeValue,
						"childNodeCount": nodeInfo.ChildCount,
						"attributes":     nodeInfo.Attrs,
					},
				})
			}
		}

		// Fallback for node IDs without object reference
		return marshalResult(map[string]interface{}{
			"node": map[string]interface{}{
				"nodeId":         params.NodeID,
				"backendNodeId":  params.BackendNodeID,
				"nodeType":       1,
				"nodeName":       "DIV",
				"localName":      "div",
				"nodeValue":      "",
				"childNodeCount": 0,
				"attributes":     []string{},
			},
		})

	case "DOM.resolveNode":
		var params struct {
			NodeID        int    `json:"nodeId"`
			BackendNodeID int    `json:"backendNodeId"`
			ObjectGroup   string `json:"objectGroup"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		// Resolve to a real Juggler object by evaluating document/documentElement.
		// We evaluate WITHOUT returnByValue to get an objectId reference.
		expr := "document"
		if params.NodeID == 2 || params.BackendNodeID == 2 {
			expr = "document.documentElement"
		}

		// Try to evaluate in the latest context
		execCtx := b.latestContextForSession(msg.SessionID)
		evalParams := map[string]interface{}{
			"expression":    expr,
			"returnByValue": false,
		}
		if execCtx != "" {
			evalParams["executionContextId"] = execCtx
		}
		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", evalParams)
		if err != nil {
			// Fallback to fake objectId
			return marshalResult(map[string]interface{}{
				"object": map[string]interface{}{
					"type":     "object",
					"subtype":  "node",
					"objectId": fmt.Sprintf("node-%d", params.NodeID),
				},
			})
		}

		// Extract the objectId from the evaluate result
		var evalResult struct {
			Result struct {
				ObjectID string `json:"objectId"`
				Type     string `json:"type"`
			} `json:"result"`
		}
		json.Unmarshal(result, &evalResult)

		if evalResult.Result.ObjectID != "" {
			return marshalResult(map[string]interface{}{
				"object": map[string]interface{}{
					"type":     evalResult.Result.Type,
					"subtype":  "node",
					"objectId": evalResult.Result.ObjectID,
				},
			})
		}

		// Fallback
		return marshalResult(map[string]interface{}{
			"object": map[string]interface{}{
				"type":     "object",
				"subtype":  "node",
				"objectId": fmt.Sprintf("node-%d", params.NodeID),
			},
		})

	case "DOM.getBoxModel":
		var params struct {
			NodeID        int    `json:"nodeId"`
			BackendNodeID int    `json:"backendNodeId"`
			ObjectID      string `json:"objectId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		// If we have an objectId, get bounding rect via callFunction
		if params.ObjectID != "" {
			expr := `function() {
				var r = this.getBoundingClientRect();
				return JSON.stringify({x: r.x, y: r.y, w: r.width, h: r.height});
			}`
			result, err := b.callJuggler(msg.SessionID, "Runtime.callFunction", map[string]interface{}{
				"functionDeclaration": expr,
				"objectId":           params.ObjectID,
				"returnByValue":      true,
			})
			if err == nil {
				var callResult struct {
					Result struct {
						Value json.RawMessage `json:"value"`
					} `json:"result"`
				}
				json.Unmarshal(result, &callResult)

				var rect struct {
					X float64 `json:"x"`
					Y float64 `json:"y"`
					W float64 `json:"w"`
					H float64 `json:"h"`
				}
				if callResult.Result.Value != nil {
					var strVal string
					if json.Unmarshal(callResult.Result.Value, &strVal) == nil {
						json.Unmarshal([]byte(strVal), &rect)
					} else {
						json.Unmarshal(callResult.Result.Value, &rect)
					}
				}

				// Box model quads: content, padding, border, margin (all same for simplicity)
				quad := []float64{
					rect.X, rect.Y,
					rect.X + rect.W, rect.Y,
					rect.X + rect.W, rect.Y + rect.H,
					rect.X, rect.Y + rect.H,
				}
				return marshalResult(map[string]interface{}{
					"model": map[string]interface{}{
						"content": quad,
						"padding": quad,
						"border":  quad,
						"margin":  quad,
						"width":   int(rect.W),
						"height":  int(rect.H),
					},
				})
			}
		}

		// Fallback
		quad := []float64{0, 0, 100, 0, 100, 100, 0, 100}
		return marshalResult(map[string]interface{}{
			"model": map[string]interface{}{
				"content": quad,
				"padding": quad,
				"border":  quad,
				"margin":  quad,
				"width":   100,
				"height":  100,
			},
		})

	case "DOM.getAttributes":
		var params struct {
			NodeID int `json:"nodeId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Without real node references, return empty attributes.
		return marshalResult(map[string]interface{}{
			"attributes": []string{},
		})

	case "DOM.removeNode":
		var params struct {
			NodeID int `json:"nodeId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}
		return json.RawMessage(`{}`), nil

	case "DOM.setAttributeValue":
		var params struct {
			NodeID int    `json:"nodeId"`
			Name   string `json:"name"`
			Value  string `json:"value"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}
		return json.RawMessage(`{}`), nil

	case "DOM.setNodeValue":
		return json.RawMessage(`{}`), nil

	case "DOM.getOuterHTML":
		var params struct {
			NodeID        int    `json:"nodeId"`
			BackendNodeID int    `json:"backendNodeId"`
			ObjectID      string `json:"objectId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		if params.ObjectID != "" {
			result, err := b.callJuggler(msg.SessionID, "Runtime.callFunction", map[string]interface{}{
				"functionDeclaration": `function() { return this.outerHTML; }`,
				"objectId":           params.ObjectID,
				"returnByValue":      true,
			})
			if err == nil {
				var callResult struct {
					Result struct {
						Value json.RawMessage `json:"value"`
					} `json:"result"`
				}
				json.Unmarshal(result, &callResult)
				var html string
				if callResult.Result.Value != nil {
					json.Unmarshal(callResult.Result.Value, &html)
				}
				return marshalResult(map[string]string{"outerHTML": html})
			}
		}
		return marshalResult(map[string]string{"outerHTML": ""})

	case "DOM.setOuterHTML":
		return json.RawMessage(`{}`), nil

	case "DOM.getContentQuads":
		var params struct {
			NodeID        int    `json:"nodeId"`
			BackendNodeID int    `json:"backendNodeId"`
			ObjectID      string `json:"objectId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		if params.ObjectID != "" {
			expr := `function() {
				var r = this.getBoundingClientRect();
				return JSON.stringify([r.x, r.y, r.x + r.width, r.y, r.x + r.width, r.y + r.height, r.x, r.y + r.height]);
			}`
			result, err := b.callJuggler(msg.SessionID, "Runtime.callFunction", map[string]interface{}{
				"functionDeclaration": expr,
				"objectId":           params.ObjectID,
				"returnByValue":      true,
			})
			if err == nil {
				var callResult struct {
					Result struct {
						Value json.RawMessage `json:"value"`
					} `json:"result"`
				}
				json.Unmarshal(result, &callResult)

				var quad []float64
				if callResult.Result.Value != nil {
					var strVal string
					if json.Unmarshal(callResult.Result.Value, &strVal) == nil {
						json.Unmarshal([]byte(strVal), &quad)
					} else {
						json.Unmarshal(callResult.Result.Value, &quad)
					}
				}
				if len(quad) == 8 {
					return marshalResult(map[string]interface{}{
						"quads": [][]float64{quad},
					})
				}
			}
		}

		return marshalResult(map[string]interface{}{
			"quads": [][]float64{{0, 0, 100, 0, 100, 100, 0, 100}},
		})

	case "DOM.scrollIntoViewIfNeeded":
		var params struct {
			NodeID        int    `json:"nodeId"`
			BackendNodeID int    `json:"backendNodeId"`
			ObjectID      string `json:"objectId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		if params.ObjectID != "" {
			b.callJuggler(msg.SessionID, "Runtime.callFunction", map[string]interface{}{
				"functionDeclaration": `function() { this.scrollIntoViewIfNeeded(true); }`,
				"objectId":           params.ObjectID,
				"returnByValue":      true,
			})
		}
		return json.RawMessage(`{}`), nil

	case "DOM.focus":
		var params struct {
			NodeID        int    `json:"nodeId"`
			BackendNodeID int    `json:"backendNodeId"`
			ObjectID      string `json:"objectId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		if params.ObjectID != "" {
			b.callJuggler(msg.SessionID, "Runtime.callFunction", map[string]interface{}{
				"functionDeclaration": `function() { this.focus(); }`,
				"objectId":           params.ObjectID,
				"returnByValue":      true,
			})
		}
		return json.RawMessage(`{}`), nil

	case "DOM.setFileInputFiles":
		var params struct {
			Files         []string `json:"files"`
			NodeID        int      `json:"nodeId"`
			BackendNodeID int      `json:"backendNodeId"`
			ObjectID      string   `json:"objectId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		if params.ObjectID != "" {
			// Juggler has Page.setFileInputFiles
			_, err := b.callJuggler(msg.SessionID, "Page.setFileInputFiles", map[string]interface{}{
				"objectId": params.ObjectID,
				"files":    params.Files,
			})
			if err != nil {
				return nil, &cdp.Error{Code: -32000, Message: err.Error()}
			}
		}
		return json.RawMessage(`{}`), nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
