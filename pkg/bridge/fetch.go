package bridge

import (
	"encoding/base64"
	"encoding/json"
	"fmt"

	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

func (b *Bridge) handleFetch(conn *cdp.Connection, msg *cdp.Message) (json.RawMessage, *cdp.Error) {
	switch msg.Method {
	case "Fetch.enable":
		var params struct {
			Patterns           []json.RawMessage `json:"patterns"`
			HandleAuthRequests bool              `json:"handleAuthRequests"`
		}
		if msg.Params != nil {
			json.Unmarshal(msg.Params, &params)
		}

		jugglerParams := map[string]interface{}{
			"enabled": true,
		}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}

		_, err := b.callJuggler("", "Browser.setRequestInterception", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Fetch.disable":
		jugglerParams := map[string]interface{}{
			"enabled": false,
		}
		if msg.SessionID != "" {
			if info, ok := b.sessions.Get(msg.SessionID); ok && info.BrowserContextID != "" {
				jugglerParams["browserContextId"] = info.BrowserContextID
			}
		}

		_, err := b.callJuggler("", "Browser.setRequestInterception", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Fetch.continueRequest":
		var params struct {
			RequestID         string        `json:"requestId"`
			URL               string        `json:"url"`
			Method            string        `json:"method"`
			PostData          string        `json:"postData"`
			Headers           []headerEntry `json:"headers"`
			InterceptResponse bool          `json:"interceptResponse"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"requestId": params.RequestID,
		}
		if params.URL != "" {
			jugglerParams["url"] = params.URL
		}
		if params.Method != "" {
			jugglerParams["method"] = params.Method
		}
		if len(params.Headers) > 0 {
			// Juggler expects headers as [{name, value}] array, not a map
			headers := make([]map[string]string, len(params.Headers))
			for i, h := range params.Headers {
				headers[i] = map[string]string{"name": h.Name, "value": h.Value}
			}
			jugglerParams["headers"] = headers
		}

		_, err := b.callJuggler("", "Browser.continueInterceptedRequest", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Fetch.fulfillRequest":
		var params struct {
			RequestID             string        `json:"requestId"`
			ResponseCode          int           `json:"responseCode"`
			ResponseHeaders       []headerEntry `json:"responseHeaders"`
			BinaryResponseHeaders string        `json:"binaryResponseHeaders"`
			Body                  string        `json:"body"`
			ResponsePhrase        string        `json:"responsePhrase"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		statusText := params.ResponsePhrase
		if statusText == "" {
			statusText = httpStatusText(params.ResponseCode)
		}

		// Convert CDP header array to Juggler header array of {name, value} objects
		var headers []map[string]string
		for _, h := range params.ResponseHeaders {
			headers = append(headers, map[string]string{
				"name":  h.Name,
				"value": h.Value,
			})
		}

		// CDP sends body as base64-encoded string; Juggler expects the same
		jugglerParams := map[string]interface{}{
			"requestId":  params.RequestID,
			"status":     params.ResponseCode,
			"statusText": statusText,
			"headers":    headers,
			"body":       params.Body,
		}

		_, err := b.callJuggler("", "Browser.fulfillInterceptedRequest", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Fetch.failRequest":
		var params struct {
			RequestID   string `json:"requestId"`
			ErrorReason string `json:"errorReason"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		// Map CDP Network.ErrorReason to Juggler error code
		errorCode := mapErrorReason(params.ErrorReason)

		jugglerParams := map[string]interface{}{
			"requestId": params.RequestID,
			"errorCode": errorCode,
		}

		_, err := b.callJuggler("", "Browser.abortInterceptedRequest", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Fetch.continueWithAuth":
		var params struct {
			RequestID             string `json:"requestId"`
			AuthChallengeResponse struct {
				Response string `json:"response"` // "Default", "CancelAuth", "ProvideCredentials"
				Username string `json:"username"`
				Password string `json:"password"`
			} `json:"authChallengeResponse"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"requestId": params.RequestID,
		}

		switch params.AuthChallengeResponse.Response {
		case "ProvideCredentials":
			jugglerParams["action"] = "provideCredentials"
			jugglerParams["username"] = params.AuthChallengeResponse.Username
			jugglerParams["password"] = params.AuthChallengeResponse.Password
		case "CancelAuth":
			jugglerParams["action"] = "cancel"
		default:
			jugglerParams["action"] = "default"
		}

		_, err := b.callJuggler("", "Browser.handleAuthRequest", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	case "Fetch.getResponseBody":
		var params struct {
			RequestID string `json:"requestId"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		result, err := b.callJuggler("", "Browser.getResponseBody", map[string]interface{}{
			"requestId": params.RequestID,
		})
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}

		// Juggler returns {base64body: "..."}, CDP expects {body: "...", base64Encoded: bool}
		var jugglerResult struct {
			Base64Body string `json:"base64body"`
		}
		if err := json.Unmarshal(result, &jugglerResult); err != nil {
			return nil, &cdp.Error{Code: -32000, Message: "failed to parse response body"}
		}

		// Check if the body is valid UTF-8 text; if so, decode and return as plain text
		decoded, decodeErr := base64.StdEncoding.DecodeString(jugglerResult.Base64Body)
		if decodeErr == nil && isUTF8Text(decoded) {
			resp, _ := json.Marshal(map[string]interface{}{
				"body":          string(decoded),
				"base64Encoded": false,
			})
			return resp, nil
		}

		// Return as base64
		resp, _ := json.Marshal(map[string]interface{}{
			"body":          jugglerResult.Base64Body,
			"base64Encoded": true,
		})
		return resp, nil

	case "Fetch.continueResponse":
		// Continue with modified response — allows response body interception.
		// The response is forwarded to the page with optional modifications.
		var params struct {
			RequestID             string        `json:"requestId"`
			ResponseCode          int           `json:"responseCode"`
			ResponseHeaders       []headerEntry `json:"responseHeaders"`
			BinaryResponseHeaders string        `json:"binaryResponseHeaders"`
		}
		if err := json.Unmarshal(msg.Params, &params); err != nil {
			return nil, &cdp.Error{Code: -32602, Message: "invalid params"}
		}

		jugglerParams := map[string]interface{}{
			"requestId": params.RequestID,
		}
		if params.ResponseCode > 0 {
			jugglerParams["status"] = params.ResponseCode
		}
		if len(params.ResponseHeaders) > 0 {
			headers := make([]map[string]string, len(params.ResponseHeaders))
			for i, h := range params.ResponseHeaders {
				headers[i] = map[string]string{"name": h.Name, "value": h.Value}
			}
			jugglerParams["headers"] = headers
		}

		_, err := b.callJuggler("", "Browser.continueInterceptedRequest", jugglerParams)
		if err != nil {
			return nil, &cdp.Error{Code: -32000, Message: err.Error()}
		}
		return json.RawMessage(`{}`), nil

	default:
		return nil, &cdp.Error{Code: -32601, Message: fmt.Sprintf("method not found: %s", msg.Method)}
	}
}

// headerEntry represents a CDP header entry with name/value pair.
type headerEntry struct {
	Name  string `json:"name"`
	Value string `json:"value"`
}

// isUTF8Text checks if the byte slice looks like valid UTF-8 text (no null bytes).
func isUTF8Text(data []byte) bool {
	for _, b := range data {
		if b == 0 {
			return false
		}
	}
	return true
}

// mapErrorReason converts a CDP Network.ErrorReason to a Juggler error code string.
func mapErrorReason(reason string) string {
	switch reason {
	case "Failed":
		return "failed"
	case "Aborted":
		return "aborted"
	case "TimedOut":
		return "timedout"
	case "AccessDenied":
		return "accessdenied"
	case "ConnectionClosed":
		return "connectionclosed"
	case "ConnectionReset":
		return "connectionreset"
	case "ConnectionRefused":
		return "connectionrefused"
	case "ConnectionAborted":
		return "connectionaborted"
	case "ConnectionFailed":
		return "connectionfailed"
	case "NameNotResolved":
		return "namenotresolved"
	case "InternetDisconnected":
		return "internetdisconnected"
	case "AddressUnreachable":
		return "addressunreachable"
	case "BlockedByClient":
		return "blockedbyclient"
	case "BlockedByResponse":
		return "blockedbyresponse"
	default:
		return "failed"
	}
}

// httpStatusText returns the standard status text for an HTTP status code.
func httpStatusText(code int) string {
	switch code {
	case 200:
		return "OK"
	case 201:
		return "Created"
	case 204:
		return "No Content"
	case 301:
		return "Moved Permanently"
	case 302:
		return "Found"
	case 304:
		return "Not Modified"
	case 400:
		return "Bad Request"
	case 401:
		return "Unauthorized"
	case 403:
		return "Forbidden"
	case 404:
		return "Not Found"
	case 405:
		return "Method Not Allowed"
	case 500:
		return "Internal Server Error"
	case 502:
		return "Bad Gateway"
	case 503:
		return "Service Unavailable"
	default:
		return "OK"
	}
}
