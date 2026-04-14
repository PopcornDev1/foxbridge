package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func (b *Bridge) handleDOMStorage(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "DOMStorage.enable", "DOMStorage.disable":
		return json.RawMessage(`{}`), nil

	case "DOMStorage.getDOMStorageItems":
		js := `JSON.stringify(Object.entries(localStorage).map(([k,v]) => [k,v]))`
		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    js,
			"returnByValue": true,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		var evalResult struct {
			Result struct {
				Value string `json:"value"`
			} `json:"result"`
		}
		json.Unmarshal(result, &evalResult)

		var items [][]string
		json.Unmarshal([]byte(evalResult.Result.Value), &items)

		entries := make([][]string, 0)
		for _, item := range items {
			entries = append(entries, item)
		}

		return mustMarshal(map[string]interface{}{"entries": entries}), nil

	case "DOMStorage.setDOMStorageItem":
		var params struct {
			StorageId struct {
				IsLocalStorage bool `json:"isLocalStorage"`
			} `json:"storageId"`
			Key   string `json:"key"`
			Value string `json:"value"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		storage := "localStorage"
		if !params.StorageId.IsLocalStorage {
			storage = "sessionStorage"
		}
		js := fmt.Sprintf(`%s.setItem(%q, %q)`, storage, params.Key, params.Value)
		b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    js,
			"returnByValue": true,
		})
		return json.RawMessage(`{}`), nil

	default:
		return json.RawMessage(`{}`), nil
	}
}
