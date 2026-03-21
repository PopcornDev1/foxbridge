package bridge

import (
	"encoding/json"
	"fmt"
	"log"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

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

		// If awaitPromise, wrap the function to await its result
		funcDecl := params.FunctionDeclaration
		if params.AwaitPromise {
			funcDecl = fmt.Sprintf(`async function(...args) { return await (%s).apply(this, args) }`, funcDecl)
		}

		// Juggler callFunction accepts: executionContextId, functionDeclaration, returnByValue, args, objectId
		jugglerParams := map[string]interface{}{
			"functionDeclaration": funcDecl,
			"returnByValue":      params.ReturnByValue,
		}
		if execCtxID != "" {
			jugglerParams["executionContextId"] = execCtxID
		}
		if params.ObjectID != "" {
			jugglerParams["objectId"] = params.ObjectID
		}
		if params.Arguments != nil {
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

		_, err := b.callJuggler(msg.SessionID, "Runtime.disposeObject", map[string]string{
			"objectId": params.ObjectID,
		})
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

		result, err := b.callJuggler(msg.SessionID, "Runtime.getObjectProperties", map[string]string{
			"objectId": params.ObjectID,
		})
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
