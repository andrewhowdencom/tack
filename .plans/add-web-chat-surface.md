# Plan: add-web-chat-surface

## Objective

Add a web-based chat surface to the `ore` framework that demonstrates parity with the existing TUI chat example. A new `surface/web/` package will implement the `surface.Surface` interface over HTTP using Server-Sent Events (SSE) for streaming, with composable `http.Handler`s that an application mounts on its own router. An `examples/web-chat/` example will wire it together with the same `cognitive.ReAct` + `state.Memory` + `loop.Step` pattern used by the TUI. The package supports API-only usage via a `WithUI()` functional option, and includes an OpenAPI 3.0 specification documenting the HTTP contract.

## Context

The `ore` framework defines a minimal `surface.Surface` interface (`Events() <-chan Event`, `SetStatus(ctx, string) error`) that abstracts I/O frontends from the core inference loop. The existing `surface/tui/` package implements this interface using Bubble Tea for a terminal UI, and `examples/tui-chat/main.go` demonstrates how to compose it with `loop.Step`, `cognitive.ReAct`, `provider/openai`, and `state.Memory`.

Key observed patterns:

- `surface/tui/tui.go`: `New(eventsCh <-chan loop.OutputEvent) *TUI` constructor starts an internal goroutine that reads `loop.OutputEvent` values (artifacts as `deltaMsg`, turn completions as `turnMsg`, errors as silent drops) and forwards them into the Bubble Tea message loop. `Events()` returns a user-event channel fed by keyboard input. `SetStatus()` sends a `statusMsg` into Bubble Tea. `Run()` blocks until the user quits.
- `loop/loop.go`: `Step.Subscribe(kinds ...string) <-chan loop.OutputEvent` filters `FanOut` events by kind. Relevant kinds for the TUI are `"text_delta"`, `"reasoning_delta"`, `"tool_call_delta"`, `"turn_complete"`.
- `loop/fanout.go`: Events are delivered non-blocking; slow subscribers drop events.
- `cognitive/react.go`: `ReAct.Run()` repeatedly calls `Step.Turn()` until the last turn is from the assistant.
- `artifact/artifact.go`: Concrete types (`TextDelta`, `ReasoningDelta`, `ToolCallDelta`, `Text`, `ToolCall`, etc.) implement `artifact.Artifact` with `Kind() string`.
- `examples/tui-chat/main.go`: Creates `loop.Step`, subscribes to delta/turn events, instantiates `tui.New(ch)`, spawns an event-processing goroutine that reads `surface.Event` from the TUI, appends user messages to `state.Memory`, calls `react.Run()`, and handles `InterruptEvent` via mutex-protected context cancellation.
- `go.mod`: Module `github.com/andrewhowdencom/ore`, Go 1.26.2. The TUI depends on Bubble Tea packages. The framework has no web-specific dependencies.
- `AGENTS.md`: Core packages (`artifact/`, `state/`, `provider/`, `core/` or `loop/` here) live at root level. Concrete provider adapters under `provider/<name>/`. Examples under `examples/<name>/`. Standard library preferred. Functional options pattern. Table-driven tests with `-race`. `httptest.Server` for mocking HTTP.

## Architectural Blueprint

### Selected Architecture

**SSE + vanilla JS, composable handlers, optional UI via `WithUI()`**

The web surface is an HTTP adapter for the `surface.Surface` contract. It uses Server-Sent Events (`text/event-stream`) for server→client streaming of deltas, turn completions, status updates, and errors. Client→server input (user messages and interrupts) travels over simple HTTP POST endpoints. The package exposes individual `http.Handler` methods so the caller composes routing, consistent with the user's request for "handlers that the example composes."

A functional option `WithUI()` embeds a minimal HTML/JS chat client (via `//go:embed`) into the binary. Without this option, the surface exposes only the API handlers, enabling API-only reuse. This aligns with the requirement that "users [can] reuse the API implementation without necessarily opting into the UI implementation itself."

The HTML/JS client is intentionally minimal: a scrollable chat area, a textarea input, Send and Interrupt buttons, and `marked.js` (loaded from CDN) for client-side Markdown→HTML rendering. This mirrors the TUI's use of glamour for Markdown→ANSI rendering, but adapted for the web medium.

### Evaluated Alternatives

| Approach | Why Not Selected |
|---|---|
| **WebSocket** | More connection management, harder to proxy, bidirectional not needed (server streams, client posts). SSE is simpler and sufficient. |
| **htmx + SSE** | Adds a frontend dependency (htmx) that isn't needed; vanilla `EventSource` and `fetch` are sufficient for a chat interface. |
| **Self-contained `http.Server` in `surface/web/`** | Violates composability principle; the caller should own the server lifecycle and routing. |
| **Server-side Markdown rendering** | Would require an HTML template engine or external library. Client-side `marked.js` is simpler and appropriate for a web frontend. |

### Components

1. **`surface/web/web.go`** — `Web` struct implementing `surface.Surface`, internal SSE broadcaster goroutine, constructor with `WithUI()` functional option.
2. **`surface/web/handlers.go`** — `SSEHandler()`, `MessageHandler()`, `InterruptHandler()`, `StaticHandler()` (conditional on `WithUI()`).
3. **`surface/web/options.go`** — `Option` functional options type and `WithUI()`.
4. **`surface/web/static/index.html`** — Minimal chat UI.
5. **`surface/web/static/chat.js`** — SSE client, DOM updates, `fetch` calls, `marked.js` integration.
6. **`surface/web/openapi.yaml`** — OpenAPI 3.0 spec documenting `/events`, `/message`, `/interrupt`, and static endpoints.
7. **`examples/web-chat/main.go`** — Reference application composing handlers into `http.ServeMux`, starting `http.Server`, wiring `ReAct` loop.

### Session Model

Ephemeral, single-session-per-process (TUI parity). The `Web` struct holds one `eventsCh` and one SSE broadcaster. Multiple browser tabs connecting to the same server share the same conversation state and event stream. Multi-tenancy (per-tab or per-user sessions) is explicitly out of scope and deferred to future work.

### Event Serialization Over SSE

The SSE protocol uses `event:` and `data:` lines:

```
event: text_delta
data: {"content":"hello"}

event: turn_complete
data: {"role":"assistant","artifacts":[...]}

event: status
data: {"status":"thinking..."}

event: error
data: {"error":"turn failed"}
```

A helper function in the web package type-switches on `artifact.Artifact` and `loop.OutputEvent` to produce JSON payloads. `artifact.Artifact` values are serialized by type-asserting to concrete structs (`TextDelta`, `ReasoningDelta`, etc.) since the interface itself carries no JSON metadata.

## Requirements

1. `surface/web/` package shall implement `surface.Surface` (`Events()`, `SetStatus()`).
2. The package shall expose composable `http.Handler` methods for SSE streaming, message receipt, interrupt receipt, and optional static file serving.
3. The constructor shall accept a `<-chan loop.OutputEvent` (same signature as `tui.New`) and functional options.
4. `WithUI()` option shall embed `index.html` and `chat.js` via `//go:embed` and enable a `StaticHandler()`.
5. Without `WithUI()`, the package shall expose only API handlers, suitable for headless API reuse.
6. The SSE stream shall emit events for: `text_delta`, `reasoning_delta`, `tool_call_delta`, `turn_complete`, `status`, `error`.
7. The HTML/JS client shall: connect to SSE, send messages via POST, send interrupts via POST, render Markdown client-side with `marked.js`, display a scrollable conversation history, show transient status updates.
8. `examples/web-chat/main.go` shall mirror `examples/tui-chat/main.go`: `loop.Step`, `cognitive.ReAct`, `state.Memory`, event-processing goroutine, interrupt handling, graceful shutdown.
9. An `openapi.yaml` shall document all HTTP endpoints, request/response schemas, and SSE event types.
10. No new Go module dependencies shall be introduced (standard library only for the `surface/web/` package).

## Task Breakdown

### Task 1: Create Core Web Surface Package
- **Goal**: Implement `surface/web/web.go` and `surface/web/options.go` with `surface.Surface` contract, internal SSE broadcaster, and `WithUI()` functional option.
- **Dependencies**: None.
- **Files Affected**: New: `surface/web/web.go`, `surface/web/options.go`, `surface/web/web_test.go`.
- **New Files**:
  - `surface/web/web.go`
  - `surface/web/options.go`
  - `surface/web/web_test.go`
- **Interfaces**:
  ```go
  type Web struct { /* eventsCh, statusCh, sseClients, mu, withUI, embedFS */ }
  type Option func(*Web)
  func WithUI() Option
  func New(eventsCh <-chan loop.OutputEvent, opts ...Option) *Web
  func (w *Web) Events() <-chan surface.Event
  func (w *Web) SetStatus(ctx context.Context, status string) error
  func (w *Web) Close() error
  ```
  - Internal: `type sseEvent struct { event string; data []byte }`, broadcaster goroutine reading `loop.OutputEvent` and `statusCh`, registering/deregistering SSE client channels.
- **Validation**: `go test ./surface/web/` passes. `go vet ./surface/web/` clean.
- **Details**: The constructor starts a goroutine that reads from `eventsCh` and `statusCh`, converts events to `sseEvent` structs, and broadcasts to all registered SSE client channels. `Close()` stops the goroutine and closes all client channels. Tests use local mock implementations of `loop.OutputEvent` (same pattern as TUI tests with mock `markdownRenderer`).

### Task 2: Add HTTP Handlers and OpenAPI Specification
- **Goal**: Implement `surface/web/handlers.go` with composable `http.Handler`s and create `openapi.yaml`.
- **Dependencies**: Task 1.
- **Files Affected**: New: `surface/web/handlers.go`, `surface/web/handlers_test.go`, `surface/web/openapi.yaml`.
- **New Files**:
  - `surface/web/handlers.go`
  - `surface/web/handlers_test.go`
  - `surface/web/openapi.yaml`
- **Interfaces**:
  ```go
  func (w *Web) SSEHandler() http.Handler
  func (w *Web) MessageHandler() http.Handler
  func (w *Web) InterruptHandler() http.Handler
  func (w *Web) StaticHandler() http.Handler  // no-op or 404 if !withUI
  ```
  - `SSEHandler` sets `Content-Type: text/event-stream`, registers a client channel, flushes `sseEvent` values as SSE protocol lines, and removes the client when the request context is cancelled.
  - `MessageHandler` expects `POST` with `Content-Type: application/json` body `{"content": "..."}`; sends `surface.UserMessageEvent` to `eventsCh`; returns `202 Accepted`.
  - `InterruptHandler` expects `POST`; sends `surface.InterruptEvent` to `eventsCh`; returns `202 Accepted`.
  - `StaticHandler` serves embedded `index.html` and `chat.js` (via `http.FileServer` over `embed.FS`) when `withUI` is true; otherwise returns `404`.
- **Validation**: `go test ./surface/web/` passes. Handler tests use `httptest.Server` and `httptest.ResponseRecorder` per `AGENTS.md`. OpenAPI spec is syntactically valid (reviewed; `swagger-codegen validate` if tool available).
- **Details**: The SSE writer must flush after each event (`http.Flusher`). Tests verify SSE stream receives a delta event after subscribing, message handler enqueues a `UserMessageEvent`, interrupt handler enqueues an `InterruptEvent`, and static handler serves expected files when `WithUI()` is used.

### Task 3: Create HTML/JS Chat Client
- **Goal**: Build a minimal, dependency-free (except CDN `marked.js`) chat UI embedded in the binary.
- **Dependencies**: Task 2 (for handler infrastructure, though files can be created independently).
- **Files Affected**: New: `surface/web/static/index.html`, `surface/web/static/chat.js`.
- **New Files**:
  - `surface/web/static/index.html`
  - `surface/web/static/chat.js`
- **Interfaces**: No Go interfaces; JS functions: `connectSSE()`, `sendMessage(content)`, `sendInterrupt()`, `appendUserMessage(content)`, `appendDelta(content)`, `finalizeTurn(turn)`, `setStatus(status)`.
- **Validation**: Manual review of HTML/JS for correctness. `go test ./surface/web/` passes (tests for `StaticHandler` verify files are served).
- **Details**: `index.html` uses a flex layout with a scrollable `#chat` div, a textarea + buttons in `#input-area`, and a `#status` span. `chat.js` creates an `EventSource('/events')`, listens for `text_delta`, `reasoning_delta`, `tool_call_delta`, `turn_complete`, `status`, and `error` events, updates the DOM, and calls `marked.parse()` on assistant text. The Send button POSTs JSON to `/message`; the Interrupt button POSTs to `/interrupt`.

### Task 4: Create `examples/web-chat/main.go`
- **Goal**: Reference application demonstrating composition of the web surface with the ore framework.
- **Dependencies**: Tasks 1–3.
- **Files Affected**: New: `examples/web-chat/main.go`.
- **New Files**:
  - `examples/web-chat/main.go`
- **Interfaces**: No new interfaces; reuses `surface/web.Web`, `loop.Step`, `cognitive.ReAct`, `state.Memory`, `provider/openai.OpenAI`.
- **Validation**: `go build ./examples/web-chat/` succeeds.
- **Details**: The `main()` function mirrors `examples/tui-chat/main.go`:
  1. Read `ORE_API_KEY`, `ORE_MODEL`, `ORE_BASE_URL` from environment.
  2. Build `openai.Provider`.
  3. Create `loop.Step`, subscribe to `"text_delta"`, `"reasoning_delta"`, `"tool_call_delta"`, `"turn_complete"`.
  4. Create `web.New(ch, web.WithUI())`.
  5. Create `http.ServeMux`, mount handlers:
     - `mux.Handle("/events", w.SSEHandler())`
     - `mux.Handle("/message", w.MessageHandler())`
     - `mux.Handle("/interrupt", w.InterruptHandler())`
     - `mux.Handle("/", w.StaticHandler())`
  6. Start `http.Server` on `:8080` (or `ORE_PORT`).
  7. Spawn event-processing goroutine identical to tui-chat's: read `surface.Event`, append to `state.Memory`, call `react.Run()`, handle `InterruptEvent` with mutex-protected context cancellation.
  8. On `os.Interrupt` / `syscall.SIGTERM`, gracefully shut down: cancel context, shutdown HTTP server, `w.Close()`, `step.Close()`, wait for goroutine.

### Task 5: Integration Validation
- **Goal**: Ensure the repository remains healthy with the new package and example.
- **Dependencies**: Tasks 1–4.
- **Files Affected**: None (read-only validation).
- **New Files**: None.
- **Interfaces**: None.
- **Validation**:
  - `go test -race ./...` passes.
  - `go vet ./...` passes.
  - `go build ./examples/web-chat/` succeeds.
  - `go test ./surface/web/` passes.
  - OpenAPI spec reviewed for completeness.
- **Details**: Run all tests with race detector. Verify no new module dependencies were added to `go.mod` for `surface/web/` (only stdlib). Confirm the example builds and the web package tests pass independently.

## Dependency Graph

- Task 1 → Task 2 (handlers depend on core `Web` struct and broadcaster)
- Task 1 → Task 3 (static files are logically coupled to `WithUI()`, but can be authored in parallel since they have no Go compile-time dependency)
- Task 2 → Task 4 (example mounts handlers)
- Task 3 → Task 4 (example uses `WithUI()`)
- Task 1, Task 2, Task 3, Task 4 → Task 5 (integration validation)
- **Parallelizable**: Task 3 (HTML/JS client) can be written in parallel with Task 2 (HTTP handlers + OpenAPI) once Task 1 is complete.

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|---|---|---|---|
| **SSE client disconnect leaks goroutine** | Medium (resource exhaustion under many reconnects) | Medium | Mitigated by request `context.Done()` monitoring in `SSEHandler`; goroutine exits and client channel is removed from broadcaster. Verify in handler tests. |
| **Artifact JSON serialization is brittle** | Medium (API contract breakage) | Medium | Use explicit type-switch helper in `surface/web/`; do not modify `artifact/` package. Add table-driven tests for each artifact kind. |
| **Multiple tabs share one event stream** | Low (feature limitation, not bug) | High (by design) | Documented in requirements: ephemeral single-session model. Future multi-tenancy will replace this. |
| **`marked.js` CDN unavailable** | Low (example UI fails) | Low | Example is a reference, not a production product. Accept CDN dependency. Alternative: vendor a minified copy if required later. |
| **OpenAPI spec drifts from implementation** | Medium | Medium | Keep spec in same package (`surface/web/openapi.yaml`) and review as part of Task 2. Validate against actual handler behavior in tests. |
| **`//go:embed` requires static files at build time** | Low | Low | Static files are committed to repo. CI will catch missing files at build time. |

## Validation Criteria

- [ ] `go test -race ./...` passes with no new race conditions.
- [ ] `go vet ./...` is clean.
- [ ] `go build ./examples/web-chat/` produces a runnable binary.
- [ ] `go test ./surface/web/` passes (unit tests for broadcaster, handlers, and static serving).
- [ ] `surface/web/` package introduces zero new external Go module dependencies.
- [ ] OpenAPI specification (`surface/web/openapi.yaml`) documents all endpoints, request/response schemas, and SSE event types.
- [ ] `Web` struct satisfies `surface.Surface` interface at compile time.
- [ ] `examples/web-chat/main.go` is structurally analogous to `examples/tui-chat/main.go` (same ReAct + state + provider composition pattern).
- [ ] `WithUI()` functional option correctly toggles static file serving; without it, `StaticHandler()` returns 404.
- [ ] HTML/JS client renders streamed text deltas, shows status updates, supports interrupt, and renders Markdown via `marked.js`.