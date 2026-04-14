<p align="center">
  <img src="docs/public/FoxbridgeBanner.jpg" alt="Foxbridge" width="600">
</p>

<p align="center">
  <h1 align="center">foxbridge</h1>
  <h4 align="center">CDP-to-Firefox Protocol Proxy — via Juggler and WebDriver BiDi</h4>
</p>

<p align="center">
  <a href="https://github.com/VulpineOS/foxbridge/actions/workflows/ci.yml"><img src="https://github.com/VulpineOS/foxbridge/actions/workflows/ci.yml/badge.svg" alt="CI"></a>
</p>

<p align="center">
  <a href="https://foxbridge.vulpineos.com">Documentation</a> ·
  <a href="https://github.com/VulpineOS/VulpineOS">VulpineOS</a> ·
  <a href="https://github.com/VulpineOS/foxbridge/issues">Issues</a>
</p>

---

## What is foxbridge?

foxbridge translates Chrome DevTools Protocol (CDP) into Firefox's Juggler and WebDriver BiDi protocols. Any tool built for Chrome — Puppeteer, OpenClaw, browser-use, Playwright CDP mode — can control Firefox-based browsers without modification.

```
CDP Client (Puppeteer, OpenClaw, etc.)
  │
  ▼ WebSocket (ws://localhost:9222)
┌────────────────────────────────────┐
│ foxbridge                          │
│  CDP Server + HTTP Discovery       │
│  Bridge Layer (method translation) │
│  ├── Juggler Backend (pipe FD 3/4) │
│  └── BiDi Backend (WebSocket)      │
└────────────────────────────────────┘
  │
  ▼
Firefox / Camoufox
```

## Why?

- **Anti-detect browsers** like [Camoufox](https://github.com/daijro/camoufox) are Firefox-based, but most AI agent tools only speak CDP
- **No existing tool** bridges CDP to Firefox's automation protocols
- **Dual backend**: Juggler (Playwright's protocol, production-ready) and WebDriver BiDi (W3C standard, future-proof)
- **Part of [VulpineOS](https://github.com/VulpineOS/VulpineOS)** — integrates automatically so OpenClaw agents use Camoufox instead of Chrome

## Quick Start

```bash
go install github.com/PopcornDev1/foxbridge/cmd/foxbridge@latest

# Juggler backend (default)
foxbridge --binary /path/to/camoufox --port 9222

# BiDi backend
foxbridge --backend bidi --binary /path/to/firefox --port 9222

# Connect to existing BiDi endpoint
foxbridge --backend bidi --bidi-url ws://localhost:9223/session

# Now connect any CDP tool:
# Puppeteer: puppeteer.connect({ browserWSEndpoint: 'ws://localhost:9222' })
```

## Docs

Full documentation is available at **[foxbridge.vulpineos.com](https://foxbridge.vulpineos.com)** — covering setup, CDP domain coverage, backend configuration, and VulpineOS integration.

## CDP Domain Coverage

| Domain | Methods | Status |
|--------|---------|--------|
| **Target** | setAutoAttach, createTarget, closeTarget, createBrowserContext, disposeBrowserContext, getTargets, attachToTarget, activateTarget, getBrowserContexts, getTargetInfo, setDiscoverTargets | Full |
| **Page** | navigate, reload, close, captureScreenshot, printToPDF, getFrameTree, getLayoutMetrics, setContent, handleJavaScriptDialog, addScriptToEvaluateOnNewDocument, removeScriptToEvaluateOnNewDocument, createIsolatedWorld, setBypassCSP, bringToFront, stopLoading, getNavigationHistory, getResourceTree, setExtraHTTPHeaders | Full |
| **Runtime** | evaluate, callFunctionOn, releaseObject, getProperties, releaseObjectGroup, addBinding, discardConsoleEntries (+ awaitPromise wrapping) | Full |
| **Input** | dispatchMouseEvent (incl. wheel deltaX/Y), dispatchKeyEvent, insertText, dispatchTouchEvent | Full |
| **Network** | setCookies, getCookies, clearBrowserCookies, setExtraHTTPHeaders, setRequestInterception, getResponseBody, emulateNetworkConditions, setUserAgentOverride | Full |
| **Fetch** | enable, disable, continueRequest, fulfillRequest, failRequest, getResponseBody | Full |
| **Emulation** | setGeolocationOverride, setUserAgentOverride, setTimezoneOverride, setLocaleOverride, setDeviceMetricsOverride, setTouchEmulationEnabled, setEmulatedMedia, setScrollbarsHidden | Full |
| **DOM** | getDocument, querySelector, querySelectorAll, describeNode, resolveNode, getBoxModel, getContentQuads, getOuterHTML, scrollIntoViewIfNeeded, focus, setFileInputFiles, getAttributes | Full |
| **Accessibility** | getFullAXTree | Full |
| **Console** | enable, disable | Full |
| **Browser** | getVersion, close, getWindowForTarget, setWindowBounds | Full |
| **Performance** | getMetrics (real timing data from page) | Full |
| **IO** | read, close (PDF streaming for Puppeteer v24+) | Full |
| **Stubs** | Debugger, Profiler, HeapProfiler, Memory, ServiceWorker, CSS, Overlay, DOMStorage, WebAuthn, Media, Audits, Inspector, + 8 more (20 total) | No-op |

## Event Translation

| Firefox Event | CDP Event |
|---|---|
| `Browser.attachedToTarget` | `Target.attachedToTarget` (tab + page dual session) |
| `Browser.detachedFromTarget` | `Target.targetDestroyed` |
| `Page.navigationCommitted` | `Page.frameNavigated` + lifecycle events |
| `Page.eventFired(load)` | `Page.loadEventFired` + `Page.frameStoppedLoading` |
| `Page.eventFired(DOMContentLoaded)` | `Page.domContentEventFired` |
| `Runtime.executionContextCreated` | `Runtime.executionContextCreated` |
| `Runtime.console` | `Runtime.consoleAPICalled` |
| `Page.dialogOpened` | `Page.javascriptDialogOpening` |
| `Network.requestWillBeSent` | `Network.requestWillBeSent` |
| `Network.responseReceived` | `Network.responseReceived` |
| `Browser.requestIntercepted` | `Fetch.requestPaused` |

## HTTP Discovery

```bash
# Browser info
curl http://localhost:9222/json/version

# Active page targets
curl http://localhost:9222/json/list
```

## Dual Backend Architecture

foxbridge implements the `Backend` interface — swap between Juggler and BiDi without changing the bridge layer:

```go
type Backend interface {
    Call(sessionID, method string, params json.RawMessage) (json.RawMessage, error)
    Subscribe(event string, handler func(sessionID string, params json.RawMessage))
    Close() error
}
```

- **Juggler** (`--backend juggler`): Pipe FD 3/4 transport, null-byte JSON framing. Direct protocol match with Playwright. Default and most battle-tested.
- **BiDi** (`--backend bidi`): WebSocket transport, W3C standard. The BiDi client internally translates Juggler-style calls to BiDi equivalents so the bridge layer works unchanged.

## Embedded Mode (VulpineOS)

When used inside [VulpineOS](https://github.com/VulpineOS/VulpineOS), foxbridge runs as an embedded CDP server sharing a single Camoufox instance:

```
Kernel -> Camoufox (single process) -> Juggler pipe -> juggler.Client
  ├── TUI (direct Juggler calls)
  ├── MCP server (Juggler calls for browser tools)
  └── Embedded foxbridge CDP server on :9222
       └── OpenClaw connects via cdpUrl -> same Camoufox contexts
```

No separate process needed — VulpineOS imports foxbridge as a Go library and starts the CDP server in-process. OpenClaw agents automatically connect to Camoufox through the embedded bridge. Graceful fallback: if the foxbridge binary is not found, OpenClaw uses its built-in Chrome.

## CLI Flags

| Flag | Default | Description |
|------|---------|-------------|
| `--port` | 9222 | CDP WebSocket port |
| `--binary` | auto-detect | Firefox/Camoufox binary path |
| `--headless` | false | Run headless |
| `--profile` | — | Firefox profile directory |
| `--backend` | juggler | Backend: `juggler` or `bidi` |
| `--bidi-url` | — | Connect to existing BiDi endpoint |
| `--bidi-port` | 9223 | BiDi port when auto-launching Firefox |

## Testing

foxbridge has comprehensive test coverage across three layers:

- **227 Go unit tests** — all passing with race detector (`go test -race ./...`)
- **74/74 Puppeteer Juggler tests** — full Puppeteer test suite against the Juggler backend
- **62/62 Puppeteer BiDi tests** — full Puppeteer test suite against the BiDi backend

```bash
go test -race ./...
```

CI runs on every push via GitHub Actions (build + vet + race-detected tests).

## License

MIT
