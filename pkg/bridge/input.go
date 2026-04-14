package bridge

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
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

		// CDP uses camelCase event types, Juggler uses lowercase
		mouseType := params.Type
		switch mouseType {
		case "mouseMoved":
			mouseType = "mousemove"
		case "mousePressed":
			mouseType = "mousedown"
		case "mouseReleased":
			mouseType = "mouseup"
		case "mouseWheel":
			mouseType = "wheel"
		}

		jugglerParams := map[string]interface{}{
			"type": mouseType,
			"x":    params.X,
			"y":    params.Y,
		}
		// Juggler expects button as a number: 0=left, 1=middle, 2=right
		// CDP sends it as a string: "left", "middle", "right", "none"
		// Juggler requires button for ALL event types including mousemove
		buttonNum := 0
		switch params.Button {
		case "left":
			buttonNum = 0
		case "middle":
			buttonNum = 1
		case "right":
			buttonNum = 2
		}
		jugglerParams["button"] = buttonNum
		jugglerParams["clickCount"] = params.ClickCount
		jugglerParams["modifiers"] = params.Modifiers
		// Juggler requires "buttons" (bitmask of pressed buttons) for all mouse events
		// button 0=left→buttons 1, button 1=middle→buttons 4, button 2=right→buttons 2
		buttons := 0
		if mouseType == "mousedown" {
			switch buttonNum {
			case 0:
				buttons = 1
			case 1:
				buttons = 4
			case 2:
				buttons = 2
			}
		}
		jugglerParams["buttons"] = buttons
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
		log.Printf("[input] dispatchKeyEvent params: %s", string(msg.Params)[:min(len(msg.Params), 300)])
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

		// CDP uses camelCase (keyDown/keyUp/rawKeyDown), Juggler uses lowercase (keydown/keyup)
		keyType := strings.ToLower(params.Type)
		// CDP sends "rawKeyDown" for physical key presses, Juggler uses "keydown"
		if keyType == "rawkeydown" {
			keyType = "keydown"
		}

		jugglerParams := map[string]interface{}{
			"type":     keyType,
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

		jpData, _ := json.Marshal(jugglerParams)
		log.Printf("[input] Juggler key params: %s", string(jpData))
		_, err := b.callJuggler(msg.SessionID, "Page.dispatchKeyEvent", jugglerParams)
		if err != nil {
			log.Printf("[input] dispatchKeyEvent error: %v", err)
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
		// Translate CDP touch params to Juggler format.
		// CDP includes "id" in touchPoints which Juggler rejects.
		var params struct {
			Type        string `json:"type"`
			TouchPoints []struct {
				X             float64 `json:"x"`
				Y             float64 `json:"y"`
				RadiusX       float64 `json:"radiusX"`
				RadiusY       float64 `json:"radiusY"`
				RotationAngle float64 `json:"rotationAngle"`
				Force         float64 `json:"force"`
			} `json:"touchPoints"`
			Modifiers int `json:"modifiers"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Build Juggler-compatible touchPoints (no "id" field)
		points := make([]map[string]interface{}, len(params.TouchPoints))
		for i, tp := range params.TouchPoints {
			points[i] = map[string]interface{}{
				"x": tp.X,
				"y": tp.Y,
			}
			if tp.RadiusX > 0 {
				points[i]["radiusX"] = tp.RadiusX
			}
			if tp.RadiusY > 0 {
				points[i]["radiusY"] = tp.RadiusY
			}
			if tp.Force > 0 {
				points[i]["force"] = tp.Force
			}
		}

		_, err := b.callJuggler(msg.SessionID, "Page.dispatchTouchEvent", map[string]interface{}{
			"type":        params.Type,
			"touchPoints": points,
			"modifiers":   params.Modifiers,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Input.dispatchDragEvent":
		var params struct {
			Type      string  `json:"type"`
			X         float64 `json:"x"`
			Y         float64 `json:"y"`
			Modifiers int     `json:"modifiers"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Map CDP drag types to Juggler types
		dragType := strings.ToLower(params.Type)
		log.Printf("[input] drag: type=%s at (%v,%v)", dragType, params.X, params.Y)

		_, err := b.callJuggler(msg.SessionID, "Page.dispatchDragEvent", map[string]interface{}{
			"type":      dragType,
			"x":         params.X,
			"y":         params.Y,
			"modifiers": params.Modifiers,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
