package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func (b *Bridge) handleAccessibility(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Accessibility.enable", "Accessibility.disable":
		return json.RawMessage(`{}`), nil

	case "Accessibility.getFullAXTree":
		// Direct pass-through to Juggler's Accessibility.getFullAXTree.
		result, err := b.callJuggler(msg.SessionID, "Accessibility.getFullAXTree", nil)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return result, nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
