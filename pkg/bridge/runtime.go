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

		// Intercept Puppeteer's cssQuerySelector pattern and replace with direct querySelector.
		// Puppeteer sends: (element, selector, {cssQuerySelector}) => cssQuerySelector(element, selector)
		// The helper objectId becomes stale after navigation. Replace with native querySelector.
		if strings.Contains(funcDecl, "cssQuerySelector") && params.Arguments != nil {
			var args []json.RawMessage
			if json.Unmarshal(params.Arguments, &args) == nil && len(args) >= 2 {
				var selectorArg struct {
					Value string `json:"value"`
				}
				json.Unmarshal(args[1], &selectorArg)
				if selectorArg.Value != "" {
					log.Printf("[runtime] intercepting cssQuerySelector for %q", selectorArg.Value)
					if strings.Contains(funcDecl, "cssQuerySelectorAll") {
						funcDecl = fmt.Sprintf(`function(element) { return element.querySelectorAll(%q); }`, selectorArg.Value)
					} else {
						funcDecl = fmt.Sprintf(`function(element) { return element.querySelector(%q); }`, selectorArg.Value)
					}
					// Only keep the element arg, drop selector and stale helper
					params.Arguments = mustMarshal([]interface{}{json.RawMessage(args[0])})
					params.AwaitPromise = false // direct querySelector doesn't return a promise
				}
			}
		}

		// If awaitPromise, wrap the function to await its result
		if params.AwaitPromise {
			funcDecl = fmt.Sprintf(`async function(...args) { return await (%s).apply(this, args) }`, funcDecl)
		}

		// Juggler callFunction accepts: executionContextId, functionDeclaration, returnByValue, args, objectId
		jugglerParams := map[string]interface{}{
			"functionDeclaration": funcDecl,
			"returnByValue":      params.ReturnByValue,
		}

		// Juggler ALWAYS requires executionContextId (unlike Chrome which infers from objectId).
		// Use the latest main world context — Juggler doesn't have real isolated worlds.
		latest := b.latestContextForSession(msg.SessionID)
		if latest != "" {
			jugglerParams["executionContextId"] = latest
		} else if execCtxID != "" {
			jugglerParams["executionContextId"] = execCtxID
		}

		// Also set objectId as executionContextId hint — if the function is called ON an object,
		// Juggler needs the context that owns it. Since we map everything to the main world,
		// the latest context is correct. But we need to make sure args with objectIds also
		// reference objects in this same context.
		// Rewrite any argument objectIds that look like they're from a stale/isolated context
		// by NOT changing them — Juggler objects are valid as long as the context exists.
		if params.ObjectID != "" {
			// Juggler's callFunction doesn't support objectId as `this` binding.
			// Prepend objectId as the first argument and wrap the function to use it as `this`.
			var existingArgs []json.RawMessage
			if params.Arguments != nil {
				json.Unmarshal(params.Arguments, &existingArgs)
			}
			newArgs := make([]json.RawMessage, 0, len(existingArgs)+1)
			objArg, _ := json.Marshal(map[string]string{"objectId": params.ObjectID})
			newArgs = append(newArgs, objArg)
			newArgs = append(newArgs, existingArgs...)
			jugglerParams["args"] = newArgs

			// Wrap: pass the object as first arg. Use .call for regular functions, direct arg for arrow functions.
			funcDecl = fmt.Sprintf(`function(__this__, ...args) { const fn = %s; if (fn.prototype) { return fn.call(__this__, ...args); } else { return fn(__this__, ...args); } }`, funcDecl)
		} else if params.Arguments != nil {
			jugglerParams["args"] = params.Arguments
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
