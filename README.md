<p align="center">
  <h1 align="center">foxbridge</h1>
  <h4 align="center">CDP-to-Firefox protocol proxy — use Puppeteer, OpenClaw, or any CDP tool with Firefox</h4>
</p>

<p align="center">
  <a href="https://foxbridge.vulpineos.com">Documentation</a> ·
  <a href="https://github.com/PopcornDev1/VulpineOS">VulpineOS</a> ·
  <a href="https://github.com/PopcornDev1/foxbridge/issues">Issues</a>
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
- **Part of [VulpineOS](https://github.com/PopcornDev1/VulpineOS)** — integrates automatically so OpenClaw agents use Camoufox instead of Chrome

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
| **Stubs** | Debugger, Profiler, Performance, Memory, ServiceWorker, CSS, Overlay, DOMStorage, WebAuthn, Media, Audits, Inspector, + 20 more | No-op |

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

```bash
go test -race ./...
```

CI runs on every push via GitHub Actions (build + vet + race-detected tests).

## License

MIT
