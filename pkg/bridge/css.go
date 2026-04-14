package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func (b *Bridge) handleCSS(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "CSS.enable", "CSS.disable":
		return json.RawMessage(`{}`), nil

	case "CSS.getComputedStyleForNode":
		var params struct {
			NodeID int `json:"nodeId"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		// Use Runtime.evaluate to get computed styles via JS
		js := `(() => {
			const nodes = document.querySelectorAll('*');
			const node = nodes[` + fmt.Sprintf("%d", params.NodeID) + `];
			if (!node) return JSON.stringify({computedStyle: []});
			const style = window.getComputedStyle(node);
			const props = [];
			for (let i = 0; i < style.length; i++) {
				props.push({name: style[i], value: style.getPropertyValue(style[i])});
			}
			return JSON.stringify({computedStyle: props});
		})()`

		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    js,
			"returnByValue": true,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		// Parse the evaluate result
		var evalResult struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		json.Unmarshal(result, &evalResult)

		if evalResult.Result.Value == "" {
			return json.RawMessage(`{"computedStyle":[]}`), nil
		}

		return json.RawMessage(evalResult.Result.Value), nil

	default:
		return json.RawMessage(`{}`), nil
	}
}
