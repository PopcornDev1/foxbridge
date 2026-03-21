package bridge

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

// stubDomains are domains that return success no-ops.
var stubDomains = map[string]bool{
	"Debugger":      true,
	"Profiler":      true,
	"Performance":   true,
	"HeapProfiler":  true,
	"Memory":        true,
	"ServiceWorker": true,
	"CacheStorage":  true,
	"IndexedDB":     true,
	"Log":           true,
	"Security":      true,
	"Fetch":         true,
	"CSS":           true,
	"Overlay":       true,
	"DOMStorage":    true,
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
		return result, nil
	}

	// Browser.close
	if method == "Browser.close" {
		_, err := b.callJuggler("", "Browser.close", nil)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil
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

	return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", method)}
}
