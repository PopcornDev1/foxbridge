package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func (b *Bridge) handleEmulation(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Emulation.setGeolocationOverride":
		var params struct {
			Latitude  *float64 `json:"latitude"`
			Longitude *float64 `json:"longitude"`
			Accuracy  *float64 `json:"accuracy"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		jugglerParams := map[string]interface{}{}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}
		if params.Latitude != nil {
			jugglerParams["latitude"] = *params.Latitude
		}
		if params.Longitude != nil {
			jugglerParams["longitude"] = *params.Longitude
		}
		if params.Accuracy != nil {
			acc := *params.Accuracy
			if acc <= 0 {
				acc = 1
			}
			jugglerParams["accuracy"] = acc
		}

		// Try to set geolocation — return success even if Juggler doesn't support it
		b.callJuggler("", "Browser.setGeolocationOverride", jugglerParams)
		return json.RawMessage(`{}`), nil

	case "Emulation.setUserAgentOverride":
		var params struct {
			UserAgent string `json:"userAgent"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"userAgent": params.UserAgent,
		}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}

		_, err := b.callJuggler("", "Browser.setUserAgentOverride", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Emulation.setTimezoneOverride":
		var params struct {
			TimezoneID string `json:"timezoneId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"timezoneId": params.TimezoneID,
		}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}

		_, err := b.callJuggler("", "Browser.setTimezoneOverride", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Emulation.setLocaleOverride":
		var params struct {
			Locale string `json:"locale"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"locale": params.Locale,
		}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}

		_, err := b.callJuggler("", "Browser.setLocaleOverride", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Emulation.setDeviceMetricsOverride":
		var params struct {
			Width             int  `json:"width"`
			Height            int  `json:"height"`
			DeviceScaleFactor float64 `json:"deviceScaleFactor"`
			Mobile            bool `json:"mobile"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		jugglerParams := map[string]interface{}{
			"width":  params.Width,
			"height": params.Height,
		}
		if params.DeviceScaleFactor > 0 {
			jugglerParams["deviceScaleFactor"] = params.DeviceScaleFactor
		}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}

		b.callJuggler("", "Browser.setDefaultViewport", jugglerParams) // ignore error
		return json.RawMessage(`{}`), nil

	case "Emulation.clearDeviceMetricsOverride":
		return json.RawMessage(`{}`), nil

	case "Emulation.setTouchEmulationEnabled":
		var params struct {
			Enabled bool `json:"enabled"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		jugglerParams := map[string]interface{}{
			"enabled": params.Enabled,
		}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}

		b.callJuggler("", "Browser.setTouchOverride", jugglerParams) // ignore error
		return json.RawMessage(`{}`), nil

	case "Emulation.setEmulatedMedia":
		var params struct {
			Media    string                       `json:"media"`
			Features []map[string]string          `json:"features"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		jugglerParams := map[string]interface{}{}
		if params.Media != "" {
			jugglerParams["type"] = params.Media
		}
		if params.Features != nil {
			jugglerParams["features"] = params.Features
		}

		_, err := b.callJuggler(msg.SessionID, "Page.setEmulatedMedia", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Emulation.setScrollbarsHidden":
		return json.RawMessage(`{}`), nil

	case "Emulation.setDefaultBackgroundColorOverride":
		return json.RawMessage(`{}`), nil

	case "Emulation.setScriptExecutionDisabled":
		var params struct {
			Value bool `json:"value"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		// Use Runtime.evaluate to toggle JavaScript on the page
		// Juggler doesn't have a direct equivalent, but we can use the docShell
		_, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    fmt.Sprintf(`void(document.docShell && (document.docShell.allowJavascript = %v))`, !params.Value),
			"returnByValue": true,
		})
		if err != nil {
			// Not all pages support docShell access — return success anyway
			return json.RawMessage(`{}`), nil
		}
		return json.RawMessage(`{}`), nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
