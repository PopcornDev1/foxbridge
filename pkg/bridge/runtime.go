package bridge

import (
	"encoding/json"
	"fmt"

	"github.com/PopcornDev1/foxbridge/pkg/cdp"
)

func (b *Bridge) handleRuntime(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Runtime.enable":
		return json.RawMessage(`{}`), nil

	case "Runtime.evaluate":
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

		// Juggler requires executionContextId — use uniqueContextId from CDP
		// (which is the Juggler execution context ID we sent in Runtime.executionContextCreated)
		execCtxID := params.UniqueContextID
		if execCtxID == "" {
			// Try to find it from the session's stored frame info
			jugglerSessionID := b.resolveSession(msg.SessionID)
			if info, ok := b.sessions.GetByJugglerSession(jugglerSessionID); ok && info.FrameID != "" {
				// Use the frame ID as context lookup — but Juggler just needs any valid executionContextId
				// The simplest approach: find it by trying the evaluate without one first
			}
		}

		jugglerParams := map[string]interface{}{
			"expression":    params.Expression,
			"returnByValue": params.ReturnByValue,
		}
		if execCtxID != "" {
			jugglerParams["executionContextId"] = execCtxID
		}
		if params.AwaitPromise {
			jugglerParams["awaitPromise"] = true
		}

		result, err := b.callJuggler(msg.SessionID, "Runtime.evaluate", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		// Translate Juggler result format to CDP format
		// Juggler: {result: {type, value, ...}, exceptionDetails: {...}}
		// CDP:     {result: {type, value, description, ...}, exceptionDetails: {...}}
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

		jugglerParams := map[string]interface{}{
			"functionDeclaration": params.FunctionDeclaration,
			"returnByValue":      params.ReturnByValue,
		}
		if params.ObjectID != "" {
			jugglerParams["objectId"] = params.ObjectID
		}
		if params.Arguments != nil {
			jugglerParams["args"] = params.Arguments
		}
		if params.AwaitPromise {
			jugglerParams["awaitPromise"] = true
		}
		// Map execution context
		if params.UniqueContextID != "" {
			jugglerParams["executionContextId"] = params.UniqueContextID
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
