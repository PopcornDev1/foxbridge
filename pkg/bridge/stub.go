package bridge

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

// stubDomains are domains that return success no-ops.
var stubDomains = map[string]bool{
	"Debugger":          true,
	"Profiler":          true,
	"HeapProfiler":      true,
	"Memory":            true,
	"ServiceWorker":     true,
	"CacheStorage":      true,
	"IndexedDB":         true,
	"Log":               true,
	"Security":          true,
	"Overlay":           true,
	"WebAuthn":          true,
	"Media":             true,
	"Audits":            true,
	"Inspector":         true,
	"Database":          true,
	"BackgroundService": true,
	"Cast":              true,
	"DeviceAccess":      true,
}

func (b *Bridge) handleStub(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	method := msg.Method

	// Browser.getVersion → Browser.getInfo
	if method == "Browser.getVersion" {
		result, err := b.callJuggler("", "Browser.getInfo", nil)
		if err != nil {
			// Fallback with static info.
			return marshalResult(map[string]string{
				"protocolVersion": "1.3",
				"product":         "foxbridge (Firefox/Camoufox)",
				"revision":        "",
				"userAgent":       "",
				"jsVersion":       "",
			})
		}
		// Translate Juggler's Browser.getInfo to CDP's Browser.getVersion format
		var info struct {
			Version   string `json:"version"`
			UserAgent string `json:"userAgent"`
		}
		json.Unmarshal(result, &info)
		return marshalResult(map[string]string{
			"protocolVersion": "1.3",
			"product":         fmt.Sprintf("foxbridge (%s)", info.Version),
			"revision":        "",
			"userAgent":       info.UserAgent,
			"jsVersion":       "",
		})
	}

	// Browser.grantPermissions
	if method == "Browser.grantPermissions" {
		// Forward to Juggler if supported, otherwise no-op
		_, _ = b.callJuggler("", "Browser.grantPermissions", msg.Params)
		return mustMarshal(map[string]interface{}{}), nil
	}

	// Browser.close
	if method == "Browser.close" {
		_, err := b.callJuggler("", "Browser.close", nil)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil
	}

	// Browser.getWindowForTarget — Puppeteer calls this for window management
	if method == "Browser.getWindowForTarget" {
		return marshalResult(map[string]interface{}{
			"windowId": 1,
			"bounds": map[string]interface{}{
				"left":        0,
				"top":         0,
				"width":       1280,
				"height":      720,
				"windowState": "normal",
			},
		})
	}

	// Browser.setWindowBounds
	if method == "Browser.setWindowBounds" {
		return json.RawMessage(`{}`), nil
	}

	// SystemInfo.getProcessInfo — some tools query this
	if method == "SystemInfo.getProcessInfo" {
		return marshalResult(map[string]interface{}{
			"processInfo": []interface{}{},
		})
	}

	// Check if the domain is a known stub domain.
	parts := strings.SplitN(method, ".", 2)
	if len(parts) == 2 && stubDomains[parts[0]] {
		return json.RawMessage(`{}`), nil
	}

	// .enable / .disable methods are generally safe to no-op.
	if strings.HasSuffix(method, ".enable") || strings.HasSuffix(method, ".disable") {
		return json.RawMessage(`{}`), nil
	}

	// Specific method stubs needed for Puppeteer compatibility.
	switch method {
	case "Runtime.runIfWaitingForDebugger":
		return json.RawMessage(`{}`), nil
	}

	return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", method)}
}
