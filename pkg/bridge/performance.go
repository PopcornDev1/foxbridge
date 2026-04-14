package bridge

import (
	"encoding/json"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func (b *Bridge) handlePerformance(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Performance.enable", "Performance.disable":
		return json.RawMessage(`{}`), nil

	case "Performance.getMetrics":
		// Evaluate performance.timing in the page to get real metrics
		expr := `(function() {
			const t = performance.timing;
			const now = performance.now();
			const metrics = [];
			metrics.push({name: "Timestamp", value: Date.now() / 1000});
			metrics.push({name: "Documents", value: document.querySelectorAll('*').length});
			metrics.push({name: "Frames", value: window.frames.length});
			metrics.push({name: "JSEventListeners", value: 0});
			if (t.domContentLoadedEventEnd > 0)
				metrics.push({name: "DomContentLoaded", value: (t.domContentLoadedEventEnd - t.navigationStart) / 1000});
			if (t.loadEventEnd > 0)
				metrics.push({name: "NavigationStart", value: t.navigationStart / 1000});
			if (t.domInteractive > 0)
				metrics.push({name: "DomInteractive", value: (t.domInteractive - t.navigationStart) / 1000});
			if (t.responseEnd > 0)
				metrics.push({name: "FirstMeaningfulPaint", value: (t.responseEnd - t.navigationStart) / 1000});
			metrics.push({name: "TaskDuration", value: now / 1000});
			if (performance.memory) {
				metrics.push({name: "JSHeapUsedSize", value: performance.memory.usedJSHeapSize});
				metrics.push({name: "JSHeapTotalSize", value: performance.memory.totalJSHeapSize});
			}
			return JSON.stringify(metrics);
		})()`

		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", map[string]interface{}{
			"expression":    expr,
			"returnByValue": true,
		})
		if err != nil {
			// Fallback: return basic timestamp
			return marshalResult(map[string]interface{}{
				"metrics": []map[string]interface{}{
					{"name": "Timestamp", "value": 0},
				},
			})
		}

		// Parse evaluate result
		var evalResult struct {
			Result struct {
				Value json.RawMessage `json:"value"`
			} `json:"result"`
		}
		json.Unmarshal(result, &evalResult)

		var metrics []map[string]interface{}
		if evalResult.Result.Value != nil {
			var strVal string
			if json.Unmarshal(evalResult.Result.Value, &strVal) == nil {
				json.Unmarshal([]byte(strVal), &metrics)
			} else {
				json.Unmarshal(evalResult.Result.Value, &metrics)
			}
		}

		if metrics == nil {
			metrics = []map[string]interface{}{
				{"name": "Timestamp", "value": 0},
			}
		}

		return marshalResult(map[string]interface{}{
			"metrics": metrics,
		})

	default:
		return json.RawMessage(`{}`), nil
	}
}
