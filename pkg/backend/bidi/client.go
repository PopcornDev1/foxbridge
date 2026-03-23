package bidi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"log"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/PopcornDev1/foxbridge/pkg/backend"
)

// Compile-time check that Client implements backend.Backend.
var _ backend.Backend = (*Client)(nil)

// DefaultCallTimeout is the default timeout for Call().
const DefaultCallTimeout = 30 * time.Second

// Client is a BiDi protocol client that implements backend.Backend.
// It translates Juggler-style method calls into BiDi protocol calls,
// so the existing bridge layer works unchanged.
type Client struct {
	transport *WSTransport
	nextID    atomic.Int64
	pending   map[int]chan *Message
	pendingMu sync.Mutex
	handlers  map[string][]backend.EventHandler // keyed by Juggler event names
	handlerMu sync.RWMutex
	done      chan struct{}
	closeOnce sync.Once

	// BiDi state
	contextMap map[string]string // juggler sessionID → BiDi browsing context ID
	realmMap   map[string]string // BiDi realm ID → BiDi browsing context ID
	contextMu  sync.RWMutex
	subscribed bool
}

// NewClient creates a BiDi client over the given WebSocket transport.
func NewClient(transport *WSTransport) *Client {
	c := &Client{
		transport:  transport,
		pending:    make(map[int]chan *Message),
		handlers:   make(map[string][]backend.EventHandler),
		done:       make(chan struct{}),
		contextMap: make(map[string]string),
		realmMap:   make(map[string]string),
	}
	go c.readLoop()
	return c
}

// Call sends a Juggler-style RPC call, translating it to BiDi protocol internally.
func (c *Client) Call(sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCallTimeout)
	defer cancel()
	return c.callWithContext(ctx, sessionID, method, params)
}

// Subscribe registers a handler for a Juggler event name.
// The readLoop translates BiDi events to Juggler event names before dispatch.
func (c *Client) Subscribe(event string, handler backend.EventHandler) {
	c.handlerMu.Lock()
	defer c.handlerMu.Unlock()
	c.handlers[event] = append(c.handlers[event], handler)
}

// Close shuts down the client and transport.
func (c *Client) Close() error {
	c.closeOnce.Do(func() { close(c.done) })
	return c.transport.Close()
}

// callWithContext translates a Juggler method+params into BiDi and sends it.
func (c *Client) callWithContext(ctx context.Context, sessionID, method string, params json.RawMessage) (json.RawMessage, error) {
	switch method {
	// ── Browser domain ──
	case "Browser.enable":
		return c.handleBrowserEnable()
	case "Browser.newPage":
		return c.handleBrowserNewPage(params)
	case "Browser.close":
		return c.sendBiDi(ctx, "browser.close", nil)
	case "Browser.getInfo":
		return c.handleBrowserGetInfo()
	case "Browser.createBrowserContext":
		return c.handleCreateBrowserContext(ctx)
	case "Browser.removeBrowserContext":
		return c.handleRemoveBrowserContext(ctx, params)
	case "Browser.setCookies":
		return c.handleSetCookies(ctx, params)
	case "Browser.getCookies":
		return c.handleGetCookies(ctx, params)
	case "Browser.clearCookies":
		return c.handleClearCookies(ctx, params)
	case "Browser.setTimezoneOverride":
		// BiDi doesn't have a native timezone override — return success silently
		return json.RawMessage(`{}`), nil
	case "Browser.setExtraHTTPHeaders":
		return c.handleSetExtraHTTPHeaders(ctx, sessionID, params)
	case "Browser.setRequestInterception":
		return c.handleSetRequestInterception(ctx, sessionID, params)

	// ── Page domain ──
	case "Page.navigate":
		return c.handlePageNavigate(ctx, sessionID, params)
	case "Page.reload":
		return c.handlePageReload(ctx, sessionID)
	case "Page.close":
		return c.handlePageClose(ctx, sessionID)
	case "Page.screenshot":
		return c.handlePageScreenshot(ctx, sessionID, params)
	case "Page.printToPDF":
		return c.handlePagePrintToPDF(ctx, sessionID, params)
	case "Page.handleDialog":
		return c.handlePageDialog(ctx, sessionID, params)
	case "Page.addScriptToEvaluateOnNewDocument":
		return c.handleAddPreloadScript(ctx, params)
	case "Page.setEmulatedMedia":
		return json.RawMessage(`{}`), nil // no-op in BiDi
	case "Page.dispatchMouseEvent":
		return c.handleDispatchMouseEvent(ctx, sessionID, params)
	case "Page.dispatchKeyEvent":
		return c.handleDispatchKeyEvent(ctx, sessionID, params)
	case "Page.insertText":
		return c.handleInsertText(ctx, sessionID, params)

	// ── Runtime domain ──
	case "Runtime.evaluate":
		return c.handleRuntimeEvaluate(ctx, sessionID, params)
	case "Runtime.callFunction":
		return c.handleRuntimeCallFunction(ctx, sessionID, params)
	case "Runtime.disposeObject":
		return c.handleRuntimeDisposeObject(ctx, sessionID, params)
	case "Runtime.getObjectProperties":
		return c.handleRuntimeGetObjectProperties(ctx, sessionID, params)

	// ── Accessibility domain ──
	case "Accessibility.getFullAXTree":
		return c.handleGetFullAXTree(ctx, sessionID)

	default:
		return nil, fmt.Errorf("bidi: unsupported method %s", method)
	}
}

// ──────────────────────────────────────────────
// Browser domain handlers
// ──────────────────────────────────────────────

func (c *Client) handleBrowserEnable() (json.RawMessage, error) {
	if c.subscribed {
		return json.RawMessage(`{}`), nil
	}
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCallTimeout)
	defer cancel()

	// Step 1: Create a BiDi session (required before any other commands)
	sessionParams, _ := json.Marshal(map[string]interface{}{
		"capabilities": map[string]interface{}{},
	})
	_, err := c.sendBiDi(ctx, "session.new", sessionParams)
	if err != nil {
		log.Printf("[bidi] session.new failed (may already exist): %v", err)
		// Continue anyway — session might already be active
	}

	// Step 2: Subscribe to events
	events := []string{
		"browsingContext.contextCreated",
		"browsingContext.contextDestroyed",
		"browsingContext.navigationStarted",
		"browsingContext.load",
		"browsingContext.domContentLoaded",
		"browsingContext.userPromptOpened",
		"browsingContext.userPromptClosed",
		"script.realmCreated",
		"script.realmDestroyed",
		"script.message",
		"network.beforeRequestSent",
		"network.responseCompleted",
	}
	subscribeParams, _ := json.Marshal(map[string]interface{}{
		"events": events,
	})
	if _, err := c.sendBiDi(ctx, "session.subscribe", subscribeParams); err != nil {
		return nil, fmt.Errorf("bidi subscribe: %w", err)
	}
	c.subscribed = true
	return json.RawMessage(`{}`), nil
}

func (c *Client) handleBrowserNewPage(params json.RawMessage) (json.RawMessage, error) {
	ctx, cancel := context.WithTimeout(context.Background(), DefaultCallTimeout)
	defer cancel()

	var p struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if params != nil {
		json.Unmarshal(params, &p)
	}

	bidiParams := map[string]interface{}{
		"type": "tab",
	}
	if p.BrowserContextID != "" {
		bidiParams["userContext"] = p.BrowserContextID
	}
	raw, _ := json.Marshal(bidiParams)
	result, err := c.sendBiDi(ctx, "browsingContext.create", raw)
	if err != nil {
		return nil, err
	}

	var created struct {
		Context string `json:"context"`
	}
	json.Unmarshal(result, &created)

	// Use the context ID as both the sessionID and targetID for the bridge
	c.contextMu.Lock()
	c.contextMap[created.Context] = created.Context
	c.contextMu.Unlock()

	resp, _ := json.Marshal(map[string]interface{}{
		"sessionId": created.Context,
		"targetId":  created.Context,
	})
	return resp, nil
}

func (c *Client) handleBrowserGetInfo() (json.RawMessage, error) {
	resp, _ := json.Marshal(map[string]interface{}{
		"userAgent":       "Mozilla/5.0 (WebDriver BiDi)",
		"protocolVersion": "1.4",
	})
	return resp, nil
}

func (c *Client) handleCreateBrowserContext(ctx context.Context) (json.RawMessage, error) {
	result, err := c.sendBiDi(ctx, "browser.createUserContext", json.RawMessage(`{}`))
	if err != nil {
		// Fallback: return a synthetic context ID
		log.Printf("[bidi] browser.createUserContext failed: %v, using fallback", err)
		resp, _ := json.Marshal(map[string]interface{}{
			"browserContextId": fmt.Sprintf("ctx-%d", time.Now().UnixNano()),
		})
		return resp, nil
	}
	var created struct {
		UserContext string `json:"userContext"`
	}
	json.Unmarshal(result, &created)

	resp, _ := json.Marshal(map[string]interface{}{
		"browserContextId": created.UserContext,
	})
	return resp, nil
}

func (c *Client) handleRemoveBrowserContext(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}
	bidiParams, _ := json.Marshal(map[string]string{
		"userContext": p.BrowserContextID,
	})
	return c.sendBiDi(ctx, "browser.removeUserContext", bidiParams)
}

func (c *Client) handleSetCookies(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// Juggler sends: {cookies: [{name, value, domain, path, ...}], browserContextId}
	// BiDi expects: {cookie: {name, value, domain, path, ...}, partition: {type, ...}}
	var p struct {
		Cookies []struct {
			Name     string `json:"name"`
			Value    string `json:"value"`
			Domain   string `json:"domain"`
			Path     string `json:"path"`
			Secure   bool   `json:"secure"`
			HttpOnly bool   `json:"httpOnly"`
			SameSite string `json:"sameSite"`
		} `json:"cookies"`
		BrowserContextID string `json:"browserContextId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	for _, cookie := range p.Cookies {
		bidiCookie := map[string]interface{}{
			"name": cookie.Name,
			"value": map[string]interface{}{
				"type":  "string",
				"value": cookie.Value,
			},
			"domain":   cookie.Domain,
			"path":     cookie.Path,
			"secure":   cookie.Secure,
			"httpOnly": cookie.HttpOnly,
		}
		if cookie.SameSite != "" {
			bidiCookie["sameSite"] = cookie.SameSite
		}

		bidiParams, _ := json.Marshal(map[string]interface{}{
			"cookie": bidiCookie,
		})
		if _, err := c.sendBiDi(ctx, "storage.setCookie", bidiParams); err != nil {
			return nil, err
		}
	}
	return json.RawMessage(`{}`), nil
}

func (c *Client) handleGetCookies(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	// BiDi storage.getCookies returns: {cookies: [{name, value: {type, value}, domain, ...}]}
	result, err := c.sendBiDi(ctx, "storage.getCookies", json.RawMessage(`{}`))
	if err != nil {
		return nil, err
	}

	var bidiResp struct {
		Cookies []struct {
			Name     string `json:"name"`
			Value    struct {
				Type  string `json:"type"`
				Value string `json:"value"`
			} `json:"value"`
			Domain   string `json:"domain"`
			Path     string `json:"path"`
			Size     int    `json:"size"`
			Secure   bool   `json:"secure"`
			HttpOnly bool   `json:"httpOnly"`
			SameSite string `json:"sameSite"`
		} `json:"cookies"`
	}
	if err := json.Unmarshal(result, &bidiResp); err != nil {
		return nil, err
	}

	// Convert to Juggler format
	cookies := make([]map[string]interface{}, 0, len(bidiResp.Cookies))
	for _, c := range bidiResp.Cookies {
		cookies = append(cookies, map[string]interface{}{
			"name":     c.Name,
			"value":    c.Value.Value,
			"domain":   c.Domain,
			"path":     c.Path,
			"size":     c.Size,
			"secure":   c.Secure,
			"httpOnly": c.HttpOnly,
			"sameSite": c.SameSite,
		})
	}

	resp, _ := json.Marshal(map[string]interface{}{
		"cookies": cookies,
	})
	return resp, nil
}

func (c *Client) handleClearCookies(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	return c.sendBiDi(ctx, "storage.deleteCookies", json.RawMessage(`{}`))
}

func (c *Client) handleSetExtraHTTPHeaders(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		Headers []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"headers"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	bidiHeaders := make([]map[string]interface{}, len(p.Headers))
	for i, h := range p.Headers {
		bidiHeaders[i] = map[string]interface{}{
			"name":  h.Name,
			"value": map[string]string{"type": "string", "value": h.Value},
		}
	}

	bidiParams, _ := json.Marshal(map[string]interface{}{
		"context": bidiCtx,
		"headers": bidiHeaders,
	})
	return c.sendBiDi(ctx, "network.setExtraHeaders", bidiParams)
}

func (c *Client) handleSetRequestInterception(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		Enabled bool `json:"enabled"`
	}
	if params != nil {
		json.Unmarshal(params, &p)
	}

	if p.Enabled {
		bidiParams, _ := json.Marshal(map[string]interface{}{
			"phases":  []string{"beforeRequestSent"},
			"urlPatterns": []map[string]string{{"type": "pattern", "pattern": "*"}},
		})
		return c.sendBiDi(ctx, "network.addIntercept", bidiParams)
	}
	// To disable, we'd need the intercept ID — return empty for now
	return json.RawMessage(`{}`), nil
}

// ──────────────────────────────────────────────
// Page domain handlers
// ──────────────────────────────────────────────

func (c *Client) handlePageNavigate(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		URL string `json:"url"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	bidiParams, _ := json.Marshal(map[string]interface{}{
		"context": bidiCtx,
		"url":     p.URL,
		"wait":    "complete",
	})
	result, err := c.sendBiDi(ctx, "browsingContext.navigate", bidiParams)
	if err != nil {
		return nil, err
	}

	var nav struct {
		Navigation string `json:"navigation"`
		URL        string `json:"url"`
	}
	json.Unmarshal(result, &nav)

	resp, _ := json.Marshal(map[string]interface{}{
		"navigationId": nav.Navigation,
		"url":          nav.URL,
	})
	return resp, nil
}

func (c *Client) handlePageReload(ctx context.Context, sessionID string) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	bidiParams, _ := json.Marshal(map[string]interface{}{
		"context": bidiCtx,
		"wait":    "complete",
	})
	return c.sendBiDi(ctx, "browsingContext.reload", bidiParams)
}

func (c *Client) handlePageClose(ctx context.Context, sessionID string) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	bidiParams, _ := json.Marshal(map[string]string{
		"context": bidiCtx,
	})
	return c.sendBiDi(ctx, "browsingContext.close", bidiParams)
}

func (c *Client) handlePageScreenshot(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		Format string `json:"format"`
	}
	if params != nil {
		json.Unmarshal(params, &p)
	}

	bidiParams := map[string]interface{}{
		"context": bidiCtx,
	}
	if p.Format != "" {
		bidiParams["format"] = map[string]string{"type": p.Format}
	}
	raw, _ := json.Marshal(bidiParams)
	result, err := c.sendBiDi(ctx, "browsingContext.captureScreenshot", raw)
	if err != nil {
		return nil, err
	}

	var ss struct {
		Data string `json:"data"`
	}
	json.Unmarshal(result, &ss)

	resp, _ := json.Marshal(map[string]string{
		"data": ss.Data,
	})
	return resp, nil
}

func (c *Client) handlePagePrintToPDF(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	bidiParams := map[string]interface{}{
		"context": bidiCtx,
	}
	if params != nil {
		var extra map[string]interface{}
		json.Unmarshal(params, &extra)
		for k, v := range extra {
			bidiParams[k] = v
		}
	}
	raw, _ := json.Marshal(bidiParams)
	result, err := c.sendBiDi(ctx, "browsingContext.print", raw)
	if err != nil {
		return nil, err
	}

	var pdf struct {
		Data string `json:"data"`
	}
	json.Unmarshal(result, &pdf)

	resp, _ := json.Marshal(map[string]string{
		"data": pdf.Data,
	})
	return resp, nil
}

func (c *Client) handlePageDialog(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		Accept   bool   `json:"accept"`
		UserText string `json:"promptText"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	bidiParams := map[string]interface{}{
		"context": bidiCtx,
		"accept":  p.Accept,
	}
	if p.UserText != "" {
		bidiParams["userText"] = p.UserText
	}
	raw, _ := json.Marshal(bidiParams)
	return c.sendBiDi(ctx, "browsingContext.handleUserPrompt", raw)
}

func (c *Client) handleAddPreloadScript(ctx context.Context, params json.RawMessage) (json.RawMessage, error) {
	var p struct {
		Script string `json:"script"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	bidiParams, _ := json.Marshal(map[string]interface{}{
		"functionDeclaration": p.Script,
	})
	return c.sendBiDi(ctx, "script.addPreloadScript", bidiParams)
}

func (c *Client) handleDispatchMouseEvent(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		Type       string          `json:"type"`
		X          float64         `json:"x"`
		Y          float64         `json:"y"`
		Button     json.RawMessage `json:"button"`
		ClickCount int             `json:"clickCount"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	// Map Juggler mouse event types to BiDi input action sequences
	actions := []interface{}{}

	// Button can be int (Juggler: 0=left, 1=middle, 2=right) or string (CDP: "left", "middle", "right")
	buttonID := 0
	if p.Button != nil {
		var btnInt int
		if json.Unmarshal(p.Button, &btnInt) == nil {
			buttonID = btnInt
		} else {
			var btnStr string
			json.Unmarshal(p.Button, &btnStr)
			switch btnStr {
			case "left":
				buttonID = 0
			case "middle":
				buttonID = 1
			case "right":
				buttonID = 2
			}
		}
	}

	switch p.Type {
	case "mouseMoved":
		actions = append(actions, map[string]interface{}{
			"type": "pointerMove",
			"x":    int(p.X),
			"y":    int(p.Y),
		})
	case "mousePressed":
		actions = append(actions, map[string]interface{}{
			"type": "pointerMove",
			"x":    int(p.X),
			"y":    int(p.Y),
		})
		actions = append(actions, map[string]interface{}{
			"type":   "pointerDown",
			"button": buttonID,
		})
	case "mouseReleased":
		actions = append(actions, map[string]interface{}{
			"type": "pointerMove",
			"x":    int(p.X),
			"y":    int(p.Y),
		})
		actions = append(actions, map[string]interface{}{
			"type":   "pointerUp",
			"button": buttonID,
		})
	}

	bidiParams, _ := json.Marshal(map[string]interface{}{
		"context": bidiCtx,
		"actions": []map[string]interface{}{
			{
				"type": "pointer",
				"id":   "mouse",
				"parameters": map[string]string{
					"pointerType": "mouse",
				},
				"actions": actions,
			},
		},
	})
	return c.sendBiDi(ctx, "input.performActions", bidiParams)
}

func (c *Client) handleDispatchKeyEvent(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		Type string `json:"type"`
		Key  string `json:"key"`
		Code string `json:"code"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	keyValue := p.Key
	if keyValue == "" {
		keyValue = p.Code
	}

	actions := []interface{}{}
	switch p.Type {
	case "keyDown":
		actions = append(actions, map[string]interface{}{
			"type":  "keyDown",
			"value": keyValue,
		})
	case "keyUp":
		actions = append(actions, map[string]interface{}{
			"type":  "keyUp",
			"value": keyValue,
		})
	case "keyPress":
		actions = append(actions, map[string]interface{}{
			"type":  "keyDown",
			"value": keyValue,
		})
		actions = append(actions, map[string]interface{}{
			"type":  "keyUp",
			"value": keyValue,
		})
	}

	bidiParams, _ := json.Marshal(map[string]interface{}{
		"context": bidiCtx,
		"actions": []map[string]interface{}{
			{
				"type":    "key",
				"id":      "keyboard",
				"actions": actions,
			},
		},
	})
	return c.sendBiDi(ctx, "input.performActions", bidiParams)
}

func (c *Client) handleInsertText(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		Text string `json:"text"`
	}
	if params != nil {
		json.Unmarshal(params, &p)
	}
	actions := make([]interface{}, 0, len(p.Text)*2)
	for _, ch := range p.Text {
		s := string(ch)
		actions = append(actions, map[string]interface{}{"type": "keyDown", "value": s})
		actions = append(actions, map[string]interface{}{"type": "keyUp", "value": s})
	}
	bidiParams, _ := json.Marshal(map[string]interface{}{
		"context": bidiCtx,
		"actions": []map[string]interface{}{{
			"type": "key", "id": "keyboard", "actions": actions,
		}},
	})
	return c.sendBiDi(ctx, "input.performActions", bidiParams)
}

// ───────────────────────���──────────────────────
// Runtime domain handlers
// ──────────────────────────────────────────────

func (c *Client) handleRuntimeEvaluate(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		Expression     string `json:"expression"`
		AwaitPromise   bool   `json:"awaitPromise"`
		ReturnByValue  bool   `json:"returnByValue"`
		ExecutionWorld string `json:"executionWorld"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	ownership := "none"
	if !p.ReturnByValue {
		ownership = "root"
	}

	// The bridge wraps async expressions in an IIFE that returns a Promise.
	// BiDi natively supports awaitPromise, so always await if the expression
	// looks like an async IIFE (from the bridge's awaitPromise wrapping).
	awaitPromise := p.AwaitPromise
	if strings.Contains(p.Expression, "(async () =>") {
		awaitPromise = true
	}

	bidiParams, _ := json.Marshal(map[string]interface{}{
		"expression": p.Expression,
		"target": map[string]interface{}{
			"context": bidiCtx,
		},
		"awaitPromise":    awaitPromise,
		"resultOwnership": ownership,
	})
	result, err := c.sendBiDi(ctx, "script.evaluate", bidiParams)
	if err != nil {
		return nil, err
	}
	return c.translateScriptResult(result), nil
}

func (c *Client) handleRuntimeCallFunction(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		FunctionDeclaration string            `json:"functionDeclaration"`
		Args                []json.RawMessage `json:"args"`
		AwaitPromise        bool              `json:"awaitPromise"`
		ReturnByValue       bool              `json:"returnByValue"`
		ObjectID            string            `json:"objectId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	ownership := "none"
	if !p.ReturnByValue {
		ownership = "root"
	}

	// Convert args to BiDi serialized values
	bidiArgs := make([]map[string]interface{}, 0, len(p.Args))
	for _, arg := range p.Args {
		bidiArgs = append(bidiArgs, c.toBiDiArg(arg))
	}

	bidiParam := map[string]interface{}{
		"functionDeclaration": p.FunctionDeclaration,
		"target": map[string]interface{}{
			"context": bidiCtx,
		},
		"awaitPromise":    p.AwaitPromise,
		"resultOwnership": ownership,
		"arguments":       bidiArgs,
	}
	if p.ObjectID != "" {
		bidiParam["this"] = map[string]interface{}{
			"handle": p.ObjectID,
		}
	}

	raw, _ := json.Marshal(bidiParam)
	log.Printf("[bidi] script.callFunction: %s", string(raw)[:min(len(raw), 300)])
	result, err := c.sendBiDi(ctx, "script.callFunction", raw)
	if err != nil {
		log.Printf("[bidi] script.callFunction error: %v", err)
		return nil, err
	}
	log.Printf("[bidi] script.callFunction result: %s", string(result)[:min(len(result), 300)])
	return c.translateScriptResult(result), nil
}

func min(a, b int) int {
	if a < b {
		return a
	}
	return b
}

func (c *Client) handleRuntimeDisposeObject(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	// Skip script.disown for BiDi — it can cause Firefox to close the WebSocket
	// when handles are already invalid or from destroyed realms. BiDi manages
	// object lifetimes via realm destruction, so explicit disown is not needed.
	return json.RawMessage(`{}`), nil
}

func (c *Client) handleRuntimeGetObjectProperties(ctx context.Context, sessionID string, params json.RawMessage) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	var p struct {
		ObjectID string `json:"objectId"`
	}
	if err := json.Unmarshal(params, &p); err != nil {
		return nil, err
	}

	if p.ObjectID == "" {
		resp, _ := json.Marshal(map[string]interface{}{"result": []interface{}{}})
		return resp, nil
	}

	// Get array length first
	lenExpr := `function() { return typeof this.length === 'number' ? this.length : Object.keys(this).length; }`
	lenParams, _ := json.Marshal(map[string]interface{}{
		"functionDeclaration": lenExpr,
		"target":             map[string]interface{}{"context": bidiCtx},
		"this":               map[string]interface{}{"handle": p.ObjectID},
		"awaitPromise":       false,
		"resultOwnership":    "none",
	})
	lenResult, err := c.sendBiDi(ctx, "script.callFunction", lenParams)
	if err != nil {
		resp, _ := json.Marshal(map[string]interface{}{"result": []interface{}{}})
		return resp, nil
	}

	var lenOuter struct {
		Result struct {
			Type  string          `json:"type"`
			Value json.RawMessage `json:"value"`
		} `json:"result"`
	}
	json.Unmarshal(lenResult, &lenOuter)
	var length int
	json.Unmarshal(lenOuter.Result.Value, &length)

	// Get each element as an individual handle
	properties := make([]map[string]interface{}, 0, length+1)
	for i := 0; i < length; i++ {
		elemExpr := fmt.Sprintf(`function() { return this[%d]; }`, i)
		elemParams, _ := json.Marshal(map[string]interface{}{
			"functionDeclaration": elemExpr,
			"target":             map[string]interface{}{"context": bidiCtx},
			"this":               map[string]interface{}{"handle": p.ObjectID},
			"awaitPromise":       false,
			"resultOwnership":    "root",
		})
		elemResult, err := c.sendBiDi(ctx, "script.callFunction", elemParams)
		if err != nil {
			continue
		}

		translated := c.translateScriptResult(elemResult)
		var juggler struct {
			Result json.RawMessage `json:"result"`
		}
		json.Unmarshal(translated, &juggler)

		properties = append(properties, map[string]interface{}{
			"name":         fmt.Sprintf("%d", i),
			"value":        juggler.Result,
			"configurable": true,
			"enumerable":   true,
			"writable":     true,
			"isOwn":        true,
		})
	}

	// Add length property
	properties = append(properties, map[string]interface{}{
		"name":         "length",
		"value":        map[string]interface{}{"type": "number", "value": length},
		"configurable": true,
		"enumerable":   false,
		"writable":     true,
		"isOwn":        true,
	})

	resp, _ := json.Marshal(map[string]interface{}{
		"result": properties,
	})
	return resp, nil
}

// ──────────────────────────────────────────────
// Accessibility domain handlers
// ──────────────────────────────────────────────

func (c *Client) handleGetFullAXTree(ctx context.Context, sessionID string) (json.RawMessage, error) {
	bidiCtx := c.resolveContext(sessionID)
	// BiDi doesn't have a native AX tree method — use script.evaluate with JS
	script := `function(){
		function walk(n,d){
			const r={role:n.computedRole||"",name:n.computedName||"",children:[]};
			if(n.children)for(const c of n.children)r.children.push(walk(c,d+1));
			return r;
		}
		try{
			const t=document.body;
			if(!t)return JSON.stringify({tree:{role:"document",name:document.title,children:[]}});
			return JSON.stringify({tree:walk(t,0)});
		}catch(e){return JSON.stringify({tree:{role:"document",name:"",children:[]},error:e.message});}
	}`

	bidiParams, _ := json.Marshal(map[string]interface{}{
		"functionDeclaration": script,
		"target": map[string]interface{}{
			"context": bidiCtx,
		},
		"awaitPromise":    false,
		"resultOwnership": "none",
	})
	result, err := c.sendBiDi(ctx, "script.callFunction", bidiParams)
	if err != nil {
		return nil, err
	}

	// Extract the string value from the BiDi result
	var bidiResult struct {
		Result struct {
			Type  string `json:"type"`
			Value string `json:"value"`
		} `json:"result"`
	}
	json.Unmarshal(result, &bidiResult)

	if bidiResult.Result.Type == "string" {
		return json.RawMessage(bidiResult.Result.Value), nil
	}
	return result, nil
}

// ──────────────────────────────────────────────
// BiDi transport layer
// ──────────────────────────────────────────────

// sendBiDi sends a raw BiDi call and waits for the response.
func (c *Client) sendBiDi(ctx context.Context, method string, params json.RawMessage) (json.RawMessage, error) {
	id := int(c.nextID.Add(1))

	msg := &Message{
		ID:     id,
		Method: method,
		Params: params,
	}

	ch := make(chan *Message, 1)
	c.pendingMu.Lock()
	c.pending[id] = ch
	c.pendingMu.Unlock()

	if err := c.transport.Send(msg); err != nil {
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, err
	}

	select {
	case resp := <-ch:
		if resp.Error != nil {
			return nil, resp.Error
		}
		return resp.Result, nil
	case <-ctx.Done():
		c.pendingMu.Lock()
		delete(c.pending, id)
		c.pendingMu.Unlock()
		return nil, fmt.Errorf("bidi call %s: %w", method, ctx.Err())
	case <-c.done:
		return nil, fmt.Errorf("bidi client closed")
	}
}

// readLoop reads messages from the transport and dispatches them.
func (c *Client) readLoop() {
	for {
		msg, err := c.transport.Receive()
		if err != nil {
			select {
			case <-c.done:
				return
			default:
				c.Close()
				return
			}
		}

		if msg.IsResponse() {
			c.pendingMu.Lock()
			ch, ok := c.pending[msg.ID]
			if ok {
				delete(c.pending, msg.ID)
			}
			c.pendingMu.Unlock()
			if ok {
				ch <- msg
			}
		} else if msg.IsEvent() {
			c.handleBiDiEvent(msg)
		}
	}
}

// ──────────────────────────────────────────────
// Event translation (BiDi → Juggler)
// ──────────────────────────────────────────────

func (c *Client) handleBiDiEvent(msg *Message) {
	var params map[string]interface{}
	if msg.Params != nil {
		json.Unmarshal(msg.Params, &params)
	}

	switch msg.Method {
	case "browsingContext.contextCreated":
		c.onContextCreated(params)
	case "browsingContext.contextDestroyed":
		c.onContextDestroyed(params)
	case "browsingContext.navigationStarted":
		// Translate BiDi navigation params to Juggler format
		navParams, _ := json.Marshal(map[string]interface{}{
			"frameId":      extractString(params, "context"),
			"url":          extractString(params, "url"),
			"navigationId": extractString(params, "navigation"),
		})
		c.emitJugglerEvent("Page.navigationCommitted", c.contextToSession(params), navParams)
	case "browsingContext.load":
		c.emitPageEventFired("load", params)
	case "browsingContext.domContentLoaded":
		c.emitPageEventFired("DOMContentLoaded", params)
	case "browsingContext.userPromptOpened":
		c.emitJugglerEvent("Page.dialogOpened", c.contextToSession(params), params)
	case "browsingContext.userPromptClosed":
		c.emitJugglerEvent("Page.dialogClosed", c.contextToSession(params), params)
	case "script.realmCreated":
		c.onRealmCreated(params)
	case "script.realmDestroyed":
		c.onRealmDestroyed(params)
	case "script.message":
		c.onScriptMessage(params)
	case "network.beforeRequestSent":
		c.emitJugglerEvent("Network.requestWillBeSent", c.contextToSession(params), params)
	case "network.responseCompleted":
		c.emitJugglerEvent("Network.requestFinished", c.contextToSession(params), params)
	default:
		log.Printf("[bidi] unhandled event: %s", msg.Method)
	}
}

func (c *Client) onContextCreated(params map[string]interface{}) {
	ctxID, _ := params["context"].(string)
	if ctxID == "" {
		return
	}

	c.contextMu.Lock()
	c.contextMap[ctxID] = ctxID
	c.contextMu.Unlock()

	jugglerParams, _ := json.Marshal(map[string]interface{}{
		"sessionId": ctxID,
		"targetInfo": map[string]interface{}{
			"type":      "page",
			"targetId":  ctxID,
			"browserContextId": extractString(params, "userContext"),
			"url":       extractString(params, "url"),
		},
	})
	c.emitJugglerEvent("Browser.attachedToTarget", "", jugglerParams)
}

func (c *Client) onContextDestroyed(params map[string]interface{}) {
	ctxID, _ := params["context"].(string)
	if ctxID == "" {
		return
	}

	c.contextMu.Lock()
	delete(c.contextMap, ctxID)
	c.contextMu.Unlock()

	jugglerParams, _ := json.Marshal(map[string]interface{}{
		"sessionId": ctxID,
		"targetId":  ctxID,
	})
	c.emitJugglerEvent("Browser.detachedFromTarget", "", jugglerParams)
}

func (c *Client) onRealmCreated(params map[string]interface{}) {
	realmID, _ := params["realm"].(string)
	ctxID := extractString(params, "context")

	if realmID != "" && ctxID != "" {
		c.contextMu.Lock()
		c.realmMap[realmID] = ctxID
		c.contextMu.Unlock()
	}

	jugglerParams, _ := json.Marshal(map[string]interface{}{
		"executionContextId": realmID,
		"auxData": map[string]interface{}{
			"frameId": ctxID,
		},
	})
	sessionID := ctxID
	c.emitJugglerEvent("Runtime.executionContextCreated", sessionID, jugglerParams)
}

func (c *Client) onRealmDestroyed(params map[string]interface{}) {
	realmID, _ := params["realm"].(string)

	c.contextMu.Lock()
	ctxID := c.realmMap[realmID]
	delete(c.realmMap, realmID)
	c.contextMu.Unlock()

	jugglerParams, _ := json.Marshal(map[string]interface{}{
		"executionContextId": realmID,
	})
	c.emitJugglerEvent("Runtime.executionContextDestroyed", ctxID, jugglerParams)
}

func (c *Client) onScriptMessage(params map[string]interface{}) {
	ctxID := extractString(params, "context")
	jugglerParams, _ := json.Marshal(params)
	c.emitJugglerEvent("Runtime.console", ctxID, jugglerParams)
}

func (c *Client) emitPageEventFired(name string, params map[string]interface{}) {
	sessionID := c.contextToSession(params)
	// Include frameId — the bridge's event handler needs it for lifecycle events
	contextID := extractString(params, "context")
	jugglerParams, _ := json.Marshal(map[string]interface{}{
		"name":    name,
		"frameId": contextID, // BiDi context ID serves as frame ID
	})
	c.emitJugglerEvent("Page.eventFired", sessionID, jugglerParams)
}

func (c *Client) emitJugglerEvent(jugglerEvent string, sessionID string, params interface{}) {
	var raw json.RawMessage
	switch v := params.(type) {
	case json.RawMessage:
		raw = v
	case []byte:
		raw = v
	case map[string]interface{}:
		raw, _ = json.Marshal(v)
	default:
		raw, _ = json.Marshal(v)
	}

	c.handlerMu.RLock()
	handlers := c.handlers[jugglerEvent]
	c.handlerMu.RUnlock()

	if len(handlers) == 0 {
		log.Printf("[bidi] no handler for translated event: %s (session=%s)", jugglerEvent, sessionID)
		return
	}
	for _, h := range handlers {
		h(sessionID, raw)
	}
}

// ──────────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────────

// resolveContext maps a Juggler sessionID to a BiDi browsing context ID.
func (c *Client) resolveContext(sessionID string) string {
	if sessionID == "" {
		return ""
	}
	c.contextMu.RLock()
	defer c.contextMu.RUnlock()
	if ctx, ok := c.contextMap[sessionID]; ok {
		return ctx
	}
	// sessionID might already be a BiDi context ID
	return sessionID
}

// contextToSession extracts the context field from BiDi event params and returns
// the corresponding Juggler sessionID.
func (c *Client) contextToSession(params map[string]interface{}) string {
	ctxID, _ := params["context"].(string)
	if ctxID == "" {
		return ""
	}
	// In our mapping, sessionID == contextID
	return ctxID
}

// translateScriptResult converts BiDi script result format to Juggler result format.
// BiDi returns: {realm: "...", result: {type: "number", value: 2}, type: "success"}
// We need to produce Juggler-style: {executionContextId: "...", result: {type: "number", value: 2}}
func (c *Client) translateScriptResult(result json.RawMessage) json.RawMessage {
	var bidiOuter struct {
		Realm  string          `json:"realm"`
		Result json.RawMessage `json:"result"`
		Type   string          `json:"type"`
	}
	if err := json.Unmarshal(result, &bidiOuter); err != nil {
		return result
	}

	// Parse the BiDi remote value (this is the actual {type, value, handle} object)
	var remoteValue struct {
		Type   string          `json:"type"`
		Value  json.RawMessage `json:"value"`
		Handle string          `json:"handle"`
	}
	if err := json.Unmarshal(bidiOuter.Result, &remoteValue); err != nil {
		return result
	}

	// Map to Juggler-style result
	jugglerResult := map[string]interface{}{
		"executionContextId": bidiOuter.Realm,
	}

	switch remoteValue.Type {
	case "undefined":
		jugglerResult["result"] = map[string]interface{}{"type": "undefined"}
	case "null":
		jugglerResult["result"] = map[string]interface{}{"type": "null"}
	case "string":
		var s string
		json.Unmarshal(remoteValue.Value, &s)
		jugglerResult["result"] = map[string]interface{}{"type": "string", "value": s}
	case "number":
		var n json.Number
		json.Unmarshal(remoteValue.Value, &n)
		jugglerResult["result"] = map[string]interface{}{"type": "number", "value": n}
	case "boolean":
		var b bool
		json.Unmarshal(remoteValue.Value, &b)
		jugglerResult["result"] = map[string]interface{}{"type": "boolean", "value": b}
	case "node", "htmlelement":
		// BiDi "node" maps to CDP "object" with subtype "node"
		obj := map[string]interface{}{
			"type":    "object",
			"subtype": "node",
		}
		if remoteValue.Handle != "" {
			obj["objectId"] = remoteValue.Handle
		}
		if remoteValue.Value != nil {
			obj["value"] = remoteValue.Value
		}
		jugglerResult["result"] = obj
	case "array", "nodelist", "htmlcollection":
		// BiDi arrays/collections map to CDP "object" with subtype "array"
		obj := map[string]interface{}{
			"type":    "object",
			"subtype": "array",
		}
		if remoteValue.Handle != "" {
			obj["objectId"] = remoteValue.Handle
		}
		// When returnByValue is true (no handle), convert BiDi serialized value to plain JSON
		if remoteValue.Handle == "" && remoteValue.Value != nil {
			obj["value"] = bidiValueToPlain(bidiOuter.Result)
		}
		jugglerResult["result"] = obj
	case "object", "map", "set", "window":
		obj := map[string]interface{}{"type": "object"}
		if remoteValue.Handle != "" {
			obj["objectId"] = remoteValue.Handle
		}
		// When returnByValue is true (no handle), convert BiDi serialized value to plain JSON
		if remoteValue.Handle == "" && remoteValue.Value != nil {
			obj["value"] = bidiValueToPlain(bidiOuter.Result)
		}
		jugglerResult["result"] = obj
	case "generator", "proxy", "promise", "typedarray", "arraybuffer", "regexp",
		"date", "error", "weakmap", "weakset", "iterator", "weakref":
		obj := map[string]interface{}{"type": "object"}
		if remoteValue.Handle != "" {
			obj["objectId"] = remoteValue.Handle
		}
		jugglerResult["result"] = obj
	default:
		obj := map[string]interface{}{"type": remoteValue.Type}
		if remoteValue.Handle != "" {
			obj["objectId"] = remoteValue.Handle
		}
		jugglerResult["result"] = obj
	}

	resp, _ := json.Marshal(jugglerResult)
	return resp
}

// toBiDiArg converts a Juggler/CDP argument to a BiDi serialized value.
// CDP args come as {value: X} or {objectId: "..."} — no explicit type field.
func (c *Client) toBiDiArg(raw json.RawMessage) map[string]interface{} {
	var arg struct {
		Type     string          `json:"type"`
		Value    json.RawMessage `json:"value"`
		ObjectID string          `json:"objectId"`
	}
	if err := json.Unmarshal(raw, &arg); err != nil {
		return map[string]interface{}{"type": "undefined"}
	}

	if arg.ObjectID != "" {
		return map[string]interface{}{"handle": arg.ObjectID}
	}

	// If we have an explicit type, use it
	if arg.Type != "" {
		switch arg.Type {
		case "string":
			var s string
			json.Unmarshal(arg.Value, &s)
			return map[string]interface{}{"type": "string", "value": s}
		case "number":
			var n float64
			json.Unmarshal(arg.Value, &n)
			return map[string]interface{}{"type": "number", "value": n}
		case "boolean":
			var b bool
			json.Unmarshal(arg.Value, &b)
			return map[string]interface{}{"type": "boolean", "value": b}
		case "null":
			return map[string]interface{}{"type": "null"}
		case "undefined":
			return map[string]interface{}{"type": "undefined"}
		case "bigint":
			var s string
			json.Unmarshal(arg.Value, &s)
			return map[string]interface{}{"type": "bigint", "value": s}
		}
	}

	// CDP-style args: {value: X} — infer type from the value
	if arg.Value != nil {
		var v interface{}
		json.Unmarshal(arg.Value, &v)
		switch tv := v.(type) {
		case string:
			return map[string]interface{}{"type": "string", "value": tv}
		case float64:
			return map[string]interface{}{"type": "number", "value": tv}
		case bool:
			return map[string]interface{}{"type": "boolean", "value": tv}
		case nil:
			return map[string]interface{}{"type": "null"}
		default:
			// Complex value — serialize as channel value
			return map[string]interface{}{"type": "object", "value": v}
		}
	}

	// Last resort: try to infer from the raw JSON itself
	var v interface{}
	json.Unmarshal(raw, &v)
	switch tv := v.(type) {
	case string:
		return map[string]interface{}{"type": "string", "value": tv}
	case float64:
		return map[string]interface{}{"type": "number", "value": tv}
	case bool:
		return map[string]interface{}{"type": "boolean", "value": tv}
	default:
		return map[string]interface{}{"type": "undefined"}
	}
}

// bidiValueToPlain recursively converts a BiDi serialized value to a plain Go value.
// BiDi: {type: "array", value: [{type: "number", value: 1}, ...]}
// Plain: [1, ...]
func bidiValueToPlain(raw json.RawMessage) interface{} {
	var entry struct {
		Type  string          `json:"type"`
		Value json.RawMessage `json:"value"`
	}
	if json.Unmarshal(raw, &entry) != nil {
		return nil
	}

	switch entry.Type {
	case "string":
		var s string
		json.Unmarshal(entry.Value, &s)
		return s
	case "number":
		var n float64
		json.Unmarshal(entry.Value, &n)
		return n
	case "boolean":
		var b bool
		json.Unmarshal(entry.Value, &b)
		return b
	case "null":
		return nil
	case "undefined":
		return nil
	case "array", "nodelist", "htmlcollection":
		var items []json.RawMessage
		json.Unmarshal(entry.Value, &items)
		result := make([]interface{}, len(items))
		for i, item := range items {
			result[i] = bidiValueToPlain(item)
		}
		return result
	case "object", "map":
		// BiDi objects serialize as [[key, value], [key, value], ...]
		var pairs []json.RawMessage
		if json.Unmarshal(entry.Value, &pairs) == nil {
			result := map[string]interface{}{}
			for _, pair := range pairs {
				var kv []json.RawMessage
				if json.Unmarshal(pair, &kv) == nil && len(kv) == 2 {
					var key string
					json.Unmarshal(kv[0], &key)
					if key == "" {
						// Try as {type: "string", value: "key"}
						var keyEntry struct {
							Value string `json:"value"`
						}
						json.Unmarshal(kv[0], &keyEntry)
						key = keyEntry.Value
					}
					result[key] = bidiValueToPlain(kv[1])
				}
			}
			return result
		}
		return nil
	default:
		return nil
	}
}

// extractString safely extracts a string field from a map.
func extractString(m map[string]interface{}, key string) string {
	v, _ := m[key].(string)
	return v
}

// Ensure base64 import is used (for screenshot data handling if needed).
var _ = base64.StdEncoding

// Ensure strings import is used.
var _ = strings.HasPrefix
