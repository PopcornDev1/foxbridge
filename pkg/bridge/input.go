package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func (b *Bridge) handleInput(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Input.dispatchMouseEvent":
		var params struct {
			Type       string  `json:"type"`
			X          float64 `json:"x"`
			Y          float64 `json:"y"`
			Button     string  `json:"button"`
			ClickCount int     `json:"clickCount"`
			Modifiers  int     `json:"modifiers"`
			DeltaX     float64 `json:"deltaX"`
			DeltaY     float64 `json:"deltaY"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"type": params.Type,
			"x":    params.X,
			"y":    params.Y,
		}
		if params.Button != "" {
			jugglerParams["button"] = params.Button
		}
		if params.ClickCount > 0 {
			jugglerParams["clickCount"] = params.ClickCount
		}
		if params.Modifiers > 0 {
			jugglerParams["modifiers"] = params.Modifiers
		}
		if params.DeltaX != 0 {
			jugglerParams["deltaX"] = params.DeltaX
		}
		if params.DeltaY != 0 {
			jugglerParams["deltaY"] = params.DeltaY
		}

		_, err := b.callJuggler(msg.SessionID, "Page.dispatchMouseEvent", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Input.dispatchKeyEvent":
		var params struct {
			Type                  string `json:"type"`
			Key                   string `json:"key"`
			Code                  string `json:"code"`
			Text                  string `json:"text"`
			UnmodifiedText        string `json:"unmodifiedText"`
			KeyIdentifier         string `json:"keyIdentifier"`
			WindowsVirtualKeyCode int    `json:"windowsVirtualKeyCode"`
			NativeVirtualKeyCode  int    `json:"nativeVirtualKeyCode"`
			Modifiers             int    `json:"modifiers"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"type":     params.Type,
			"key":      params.Key,
			"code":     params.Code,
			"keyCode":  params.WindowsVirtualKeyCode,
			"location": 0,
			"repeat":   false,
		}
		if params.Text != "" {
			jugglerParams["text"] = params.Text
		}
		if params.Key == "" {
			jugglerParams["key"] = ""
		}
		if params.Code == "" {
			jugglerParams["code"] = ""
		}

		_, err := b.callJuggler(msg.SessionID, "Page.dispatchKeyEvent", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Input.insertText":
		var params struct {
			Text string `json:"text"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		_, err := b.callJuggler(msg.SessionID, "Page.insertText", map[string]string{
			"text": params.Text,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Input.dispatchTouchEvent":
		// Pass through to Juggler's touch event handler.
		_, err := b.callJuggler(msg.SessionID, "Page.dispatchTouchEvent", msg.Params)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
