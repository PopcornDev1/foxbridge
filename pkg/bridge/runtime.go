package bridge

import (
	"encoding/json"
	"fmt"
	"log"
	"strings"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func mustMarshal(v interface{}) json.RawMessage {
	data, _ := json.Marshal(v)
	return data
}

func (b *Bridge) handleRuntime(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Runtime.enable":
		// After returning success, emit executionContextCreated for existing contexts
		// (Puppeteer expects these after Runtime.enable, like Chrome does)
		go func() {
			// Get frame ID from the session
			jugglerSessionID := b.resolveSession(msg.SessionID)
			frameID := ""
			if info, ok := b.sessions.Get(msg.SessionID); ok {
				frameID = info.FrameID
			}
			if frameID == "" && jugglerSessionID != "" {
				if info, ok := b.sessions.GetByJugglerSession(jugglerSessionID); ok {
					frameID = info.FrameID
				}
			}
			if frameID != "" {
				// Emit a default execution context for this frame
				b.emitEvent("Runtime.executionContextCreated", map[string]interface{}{
					"context": map[string]interface{}{
						"id":       100, // distinct from earlier contexts
						"origin":   "https://example.com",
						"name":     "",
						"uniqueId": fmt.Sprintf("ctx-%s-main", jugglerSessionID),
						"auxData": map[string]interface{}{
							"isDefault": true,
							"type":      "default",
							"frameId":   frameID,
						},
					},
				}, msg.SessionID)
			}
		}()
		return json.RawMessage(`{}`), nil

	case "Runtime.evaluate":
		log.Printf("[runtime] evaluate on session=%s params=%s", msg.SessionID, string(msg.Params)[:min(len(msg.Params), 200)])
		var params struct {
			Expression            string `json:"expression"`
			ReturnByValue         bool   `json:"returnByValue"`
			AwaitPromise          bool   `json:"awaitPromise"`
			UniqueContextID       string `json:"uniqueContextId"`
			ContextID             int    `json:"contextId"`
			GeneratePreview       bool   `json:"generatePreview"`
			UserGesture           bool   `json:"userGesture"`
			IncludeCommandLineAPI bool   `json:"includeCommandLineAPI"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Map CDP contextId (numeric) to Juggler executionContextId (string)
		execCtxID := params.UniqueContextID
		if execCtxID == "" && params.ContextID > 0 {
			b.ctxMapMu.RLock()
			if mapped, ok := b.ctxMap[params.ContextID]; ok {
				execCtxID = mapped
			}
			b.ctxMapMu.RUnlock()
		}

		// Always prefer the latest context to avoid stale context errors after navigation
		latest := b.latestContextForSession(msg.SessionID)
		if latest != "" {
			execCtxID = latest
		}

		// If awaitPromise is requested, wrap the expression so the promise is resolved
		// before returning. Juggler's Runtime.evaluate doesn't support awaitPromise natively.
		expression := params.Expression
		if params.AwaitPromise {
			// Wrap in an async IIFE that awaits the result. Juggler will evaluate
			// this synchronously, but the await inside handles promise resolution.
			// We use a special wrapper that Juggler can handle.
			expression = fmt.Sprintf(`(async () => { return await (%s) })()`, expression)
		}

		// Juggler only accepts: executionContextId, expression, returnByValue
		jugglerParams := map[string]interface{}{
			"expression":    expression,
			"returnByValue": params.ReturnByValue,
		}
		if execCtxID != "" {
			jugglerParams["executionContextId"] = execCtxID
		}

		log.Printf("[runtime] calling Juggler Runtime.evaluate with %v", jugglerParams)
		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", jugglerParams)
		if err != nil {
			log.Printf("[runtime] evaluate error: %v", err)
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		log.Printf("[runtime] evaluate result: %s", string(result)[:min(len(result), 300)])
		return result, nil

	case "Runtime.callFunctionOn":
		log.Printf("[runtime] callFunctionOn params: %s", string(msg.Params)[:min(len(msg.Params), 500)])
		var params struct {
			FunctionDeclaration string          `json:"functionDeclaration"`
			ObjectID            string          `json:"objectId"`
			Arguments           json.RawMessage `json:"arguments"`
			ReturnByValue       bool            `json:"returnByValue"`
			AwaitPromise        bool            `json:"awaitPromise"`
			ExecutionContextID  int             `json:"executionContextId"`
			UniqueContextID     string          `json:"uniqueContextId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Map CDP contextId to Juggler executionContextId
		execCtxID := params.UniqueContextID
		if execCtxID == "" && params.ExecutionContextID > 0 {
			b.ctxMapMu.RLock()
			if mapped, ok := b.ctxMap[params.ExecutionContextID]; ok {
				execCtxID = mapped
			}
			b.ctxMapMu.RUnlock()
		}

		// If we have a context ID but no objectId, always use the LATEST context
		// to avoid stale context errors after navigation
		if params.ObjectID == "" {
			latest := b.latestContextForSession(msg.SessionID)
			if latest != "" {
				execCtxID = latest
			}
		}

		funcDecl := params.FunctionDeclaration

		// Strip //# sourceURL= comments that Puppeteer appends to function declarations.
		// These break SpiderMonkey's async wrapper when the comment is inside grouping parens.
		if idx := strings.Index(funcDecl, "\n//# sourceURL="); idx >= 0 {
			funcDecl = funcDecl[:idx]
		}
		if idx := strings.Index(funcDecl, "\n//@ sourceURL="); idx >= 0 {
			funcDecl = funcDecl[:idx]
		}

		// Check if there's a pending $eval selector — if so, combine querySelector + userFn
		// into a single evaluate to avoid Juggler's object lifecycle issues.
		b.lastQueryMu.RLock()
		pendingSelector := b.lastQuery[msg.SessionID]
		pendingAll := b.lastQueryAll[msg.SessionID]
		b.lastQueryMu.RUnlock()

		if pendingSelector != "" && !strings.Contains(funcDecl, "cssQuerySelector") {
			// For $$eval, Puppeteer sends 2 internal plumbing calls (iterator + collector)
			// before the user function. Skip them by counting.
			b.lastQueryMu.RLock()
			skipsRemaining := b.lastQuerySkips[msg.SessionID]
			b.lastQueryMu.RUnlock()

			if skipsRemaining > 0 {
				b.lastQueryMu.Lock()
				b.lastQuerySkips[msg.SessionID]--
				b.lastQueryMu.Unlock()
				log.Printf("[runtime] skipping $$eval plumbing call (%d remaining)", skipsRemaining-1)
				return marshalResult(map[string]interface{}{
					"result": map[string]interface{}{
						"type": "object",
					},
				})
			}

			// This is the user's function — combine with the stored selector
			b.lastQueryMu.Lock()
			delete(b.lastQuery, msg.SessionID)
			delete(b.lastQueryAll, msg.SessionID)
			b.lastQueryMu.Unlock()

			log.Printf("[runtime] combining $eval: selector=%q all=%v fn=%s", pendingSelector, pendingAll, funcDecl[:min(len(funcDecl), 60)])

			var expr string
			if pendingAll {
				expr = fmt.Sprintf(`(function() { const els = document.querySelectorAll(%q); return (%s)(Array.from(els)); })()`, pendingSelector, funcDecl)
			} else {
				expr = fmt.Sprintf(`(function() { const el = document.querySelector(%q); return (%s)(el); })()`, pendingSelector, funcDecl)
			}

			evalParams := map[string]interface{}{
				"expression":    expr,
				"returnByValue": params.ReturnByValue,
			}
			if latest := b.latestContextForSession(msg.SessionID); latest != "" {
				evalParams["executionContextId"] = latest
			}
			result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", evalParams)
			if err != nil {
				return nil, &cdp.Error{Code: -32000, Message: err.Error()}
			}
			return result, nil
		}

		// Intercept Puppeteer's cssQuerySelector pattern — store the selector for the
		// NEXT callFunctionOn which will be the user's function ($eval pattern).
		if strings.Contains(funcDecl, "cssQuerySelector") && params.Arguments != nil {
			var args []json.RawMessage
			if json.Unmarshal(params.Arguments, &args) == nil && len(args) >= 2 {
				var selectorArg struct {
					Value string `json:"value"`
				}
				json.Unmarshal(args[1], &selectorArg)
				if selectorArg.Value != "" {
					isAll := strings.Contains(funcDecl, "cssQuerySelectorAll")
					log.Printf("[runtime] storing $eval selector %q (all=%v) for next callFunctionOn", selectorArg.Value, isAll)

					b.lastQueryMu.Lock()
					b.lastQuery[msg.SessionID] = selectorArg.Value
					b.lastQueryAll[msg.SessionID] = isAll
					b.lastQueryMu.Unlock()

					// Set skip count: $$eval has 3 plumbing calls to skip
					// (iterator + collector + element mapper), $eval has 0
					skips := 0
					if isAll {
						skips = 3
					}
					b.lastQueryMu.Lock()
					b.lastQuerySkips[msg.SessionID] = skips
					b.lastQueryMu.Unlock()

					return marshalResult(map[string]interface{}{
						"result": map[string]interface{}{
							"type":    "object",
							"subtype": "node",
						},
					})
				}
			}
		}

		// Handle objectId-as-this: Juggler doesn't support objectId for `this` binding.
		// Prepend objectId as first argument and wrap function to use it as `this` or first arg.
		finalArgs := params.Arguments
		if params.ObjectID != "" {
			var existingArgs []json.RawMessage
			if params.Arguments != nil {
				json.Unmarshal(params.Arguments, &existingArgs)
			}
			newArgs := make([]json.RawMessage, 0, len(existingArgs)+1)
			objArg, _ := json.Marshal(map[string]string{"objectId": params.ObjectID})
			newArgs = append(newArgs, objArg)
			newArgs = append(newArgs, existingArgs...)
			finalArgs = mustMarshal(newArgs)

			// Wrap: pass the object as first arg
			funcDecl = fmt.Sprintf(`function(__this__, ...args) { const fn = %s; if (fn.prototype) { return fn.call(__this__, ...args); } else { return fn(__this__, ...args); } }`, funcDecl)
		}

		// Build Juggler params AFTER all funcDecl transformations
		jugglerParams := map[string]interface{}{
			"functionDeclaration": funcDecl,
			"returnByValue":      params.ReturnByValue,
		}

		// Juggler ALWAYS requires executionContextId
		latest := b.latestContextForSession(msg.SessionID)
		if latest != "" {
			jugglerParams["executionContextId"] = latest
		} else if execCtxID != "" {
			jugglerParams["executionContextId"] = execCtxID
		}

		if finalArgs != nil {
			jugglerParams["args"] = json.RawMessage(finalArgs)
		}

		result, err := b.callJuggler(msg.SessionID, "Runtime.callFunction", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		return result, nil

	case "Runtime.releaseObject":
		var params struct {
			ObjectID string `json:"objectId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		disposeParams := map[string]interface{}{
			"objectId": params.ObjectID,
		}
		if latest := b.latestContextForSession(msg.SessionID); latest != "" {
			disposeParams["executionContextId"] = latest
		}
		_, err := b.callJuggler(msg.SessionID, "Runtime.disposeObject", disposeParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Runtime.getProperties":
		// If there's a pending $$eval, return empty properties — the real query
		// will be done in the combined evaluate when the user function arrives.
		b.lastQueryMu.RLock()
		hasPending := b.lastQuery[msg.SessionID] != ""
		b.lastQueryMu.RUnlock()
		if hasPending {
			return marshalResult(map[string]interface{}{
				"result": []interface{}{},
			})
		}

		var params struct {
			ObjectID                 string `json:"objectId"`
			OwnProperties            bool   `json:"ownProperties"`
			GeneratePreview          bool   `json:"generatePreview"`
			AccessorPropertiesOnly   bool   `json:"accessorPropertiesOnly"`
			NonIndexedPropertiesOnly bool   `json:"nonIndexedPropertiesOnly"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		getPropsParams := map[string]interface{}{
			"objectId": params.ObjectID,
		}
		latest := b.latestContextForSession(msg.SessionID)
		if latest != "" {
			getPropsParams["executionContextId"] = latest
		}
		result, err := b.callJuggler(msg.SessionID, "Runtime.getObjectProperties", getPropsParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		return result, nil

	case "Runtime.releaseObjectGroup":
		return json.RawMessage(`{}`), nil

	case "Runtime.runIfWaitingForDebugger":
		return json.RawMessage(`{}`), nil

	case "Runtime.addBinding":
		return json.RawMessage(`{}`), nil

	case "Runtime.discardConsoleEntries":
		return json.RawMessage(`{}`), nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}
