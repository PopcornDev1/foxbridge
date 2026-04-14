package bridge

import (
	"encoding/json"
	"log"
	"strings"
	"sync"

	"github.com/VulpineOS/foxbridge/pkg/backend"
	"github.com/VulpineOS/foxbridge/pkg/cdp"
)

// Bridge translates CDP messages to Juggler protocol calls.
type Bridge struct {
	backend    backend.Backend
	isBiDi     bool // true when using BiDi backend (disables $eval combine pattern)
	sessions   *cdp.SessionManager
	server     *cdp.Server
	autoAttach *autoAttachState
	// ctxMap maps numeric CDP execution context IDs to Juggler execution context ID strings
	ctxMapMu   sync.RWMutex
	ctxMap     map[int]string // cdpContextID → jugglerContextID
	ctxCounter int            // monotonic counter for execution context IDs
	// loaderMap tracks the last loaderId per CDP session for lifecycle event consistency
	loaderMapMu sync.RWMutex
	loaderMap   map[string]string // cdpSessionID → last loaderId
	// latestCtx tracks the most recent Juggler execution context per session
	latestCtxMu sync.RWMutex
	latestCtx   map[string]string // jugglerSessionID → latest executionContextId
	// isolatedWorlds tracks isolated world names per CDP session for re-emission after navigation
	isolatedWorldsMu sync.RWMutex
	isolatedWorlds   map[string][]isolatedWorldInfo // cdpSessionID → list of isolated worlds
	// nodeObjects maps backendNodeId → objectId for DOM.describeNode/resolveNode round-trips
	nodeObjectsMu sync.RWMutex
	nodeObjects   map[int]string // backendNodeId → objectId
	// lastQuerySelector tracks the last intercepted CSS selector per session
	// so we can combine querySelector + userFn into a single evaluate for $eval
	lastQueryMu    sync.RWMutex
	lastQuery      map[string]string // cdpSessionID → CSS selector
	lastQueryAll   map[string]bool   // cdpSessionID → true if querySelectorAll
	lastQuerySkips map[string]int    // cdpSessionID → remaining calls to skip before user fn
	// lastDialogID tracks the last dialog ID per CDP session for handleDialog
	lastDialogMu sync.Mutex
	lastDialog   map[string]string // cdpSessionID → dialogId
	// pdfStreams stores PDF data for IO.read streaming
	pdfStreamsMu sync.Mutex
	pdfStreams   map[string]string // streamHandle → base64 data
	// pendingContextClear tracks sessions that had executionContextsCleared.
	// The next executionContextCreated should trigger isolated world re-emission.
	pendingContextClearMu sync.Mutex
	pendingContextClear   map[string]bool // cdpSessionID → true
}

// New creates a new Bridge. Set isBiDi to true when using the BiDi backend.
func New(b backend.Backend, sessions *cdp.SessionManager, server *cdp.Server, isBiDi ...bool) *Bridge {
	bidi := len(isBiDi) > 0 && isBiDi[0]
	_ = bidi
	return &Bridge{
		backend:             b,
		isBiDi:              bidi,
		sessions:            sessions,
		server:              server,
		autoAttach:          newAutoAttachState(),
		ctxMap:              make(map[int]string),
		ctxCounter:          100,
		loaderMap:           make(map[string]string),
		latestCtx:           make(map[string]string),
		isolatedWorlds:      make(map[string][]isolatedWorldInfo),
		nodeObjects:         make(map[int]string),
		lastQuery:           make(map[string]string),
		lastQueryAll:        make(map[string]bool),
		lastQuerySkips:      make(map[string]int),
		lastDialog:          make(map[string]string),
		pdfStreams:          make(map[string]string),
		pendingContextClear: make(map[string]bool),
	}
}

// HandleMessage dispatches an incoming CDP message to the appropriate domain handler.
func (b *Bridge) HandleMessage(conn *cdp.Connection, msg *cdp.Message) {
	method := msg.Method

	var result json.RawMessage
	var cdpErr *cdp.Error

	switch {
	case strings.HasPrefix(method, "Target."):
		result, cdpErr = b.handleTarget(conn, msg)
	case strings.HasPrefix(method, "Page."):
		result, cdpErr = b.handlePage(conn, msg)
	case strings.HasPrefix(method, "Runtime."):
		result, cdpErr = b.handleRuntime(conn, msg)
	case strings.HasPrefix(method, "Input."):
		result, cdpErr = b.handleInput(conn, msg)
	case strings.HasPrefix(method, "Network."):
		result, cdpErr = b.handleNetwork(conn, msg)
	case strings.HasPrefix(method, "Emulation."):
		result, cdpErr = b.handleEmulation(conn, msg)
	case strings.HasPrefix(method, "DOM."):
		result, cdpErr = b.handleDOM(conn, msg)
	case strings.HasPrefix(method, "Accessibility."):
		result, cdpErr = b.handleAccessibility(conn, msg)
	case strings.HasPrefix(method, "Console."):
		result, cdpErr = b.handleConsole(conn, msg)
	case strings.HasPrefix(method, "Fetch."):
		result, cdpErr = b.handleFetch(conn, msg)
	case strings.HasPrefix(method, "Performance."):
		result, cdpErr = b.handlePerformance(conn, msg)
	case strings.HasPrefix(method, "IO."):
		result, cdpErr = b.handleIO(conn, msg)
	case strings.HasPrefix(method, "CSS."):
		result, cdpErr = b.handleCSS(conn, msg)
	case strings.HasPrefix(method, "DOMStorage."):
		result, cdpErr = b.handleDOMStorage(conn, msg)
	default:
		result, cdpErr = b.handleStub(conn, msg)
	}

	resp := &cdp.Message{
		ID:        msg.ID,
		SessionID: msg.SessionID,
	}
	if cdpErr != nil {
		resp.Error = cdpErr
	} else {
		if result == nil {
			result = json.RawMessage(`{}`)
		}
		resp.Result = result
	}

	if err := conn.Send(resp); err != nil {
		log.Printf("failed to send CDP response for %s: %v", method, err)
	}
}

// resolveSession maps a CDP sessionID to a Juggler sessionID.
func (b *Bridge) resolveSession(cdpSessionID string) string {
	if cdpSessionID == "" {
		return ""
	}
	if info, ok := b.sessions.Get(cdpSessionID); ok {
		return info.JugglerSessionID
	}
	return cdpSessionID
}

// callJuggler is a convenience wrapper for backend.Call with session resolution.
func (b *Bridge) callJuggler(cdpSessionID, method string, params interface{}) (json.RawMessage, error) {
	sessionID := b.resolveSession(cdpSessionID)
	var raw json.RawMessage
	if params != nil {
		var err error
		raw, err = json.Marshal(params)
		if err != nil {
			return nil, err
		}
	}
	return b.backend.Call(sessionID, method, raw)
}

// nextCtxID allocates a new unique execution context ID.
func (b *Bridge) nextCtxID() int {
	b.ctxMapMu.Lock()
	b.ctxCounter++
	id := b.ctxCounter
	b.ctxMapMu.Unlock()
	return id
}

// isolatedWorldInfo tracks an isolated world for re-emission after navigation.
type isolatedWorldInfo struct {
	WorldName string
	FrameID   string
}

// latestContextForSession returns the most recent Juggler execution context for a CDP session.
func (b *Bridge) latestContextForSession(cdpSessionID string) string {
	jugglerSessionID := b.resolveSession(cdpSessionID)
	b.latestCtxMu.RLock()
	defer b.latestCtxMu.RUnlock()
	return b.latestCtx[jugglerSessionID]
}

// emitEvent sends a CDP event to all connected clients.
func (b *Bridge) emitEvent(method string, params interface{}, sessionID string) {
	var raw json.RawMessage
	if params != nil {
		raw, _ = json.Marshal(params)
	}
	b.server.Broadcast(&cdp.Message{
		Method:    method,
		Params:    raw,
		SessionID: sessionID,
	})
}
