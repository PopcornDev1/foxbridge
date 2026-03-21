package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func (b *Bridge) handleConsole(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Console.enable", "Console.disable":
		// Console is always on in Juggler — no-op.
		return json.RawMessage(`{}`), nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
