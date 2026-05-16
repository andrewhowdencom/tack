# Plan: Forge Multi-Conduit Blueprint and Agent Integration

## Objective

Complete the transformation of Forge from a single-conduit code generator into a multi-conduit blueprint system. An `agent/` orchestration package and a standardized `conduit.Conduit` interface (already partially landed) will allow Forge-generated applications to host multiple frontends (HTTP, TUI, and future external conduits) against a shared `session.Manager`. This plan covers the remaining implementation work after the conduit interface and blueprint format rename have already been merged to the branch.

## Context

### Already Implemented on This Branch

- **`conduit.Conduit` interface** (`conduit/conduit.go`) — added in commit `a4acf60`. Defines `Run(ctx context.Context) error`.
- **`Blueprint` struct and `ParseBlueprint`** (`cmd/forge/blueprint.go`) — added in commit `c6fd960`. Replaced the old `Manifest` struct. YAML format now uses `conduits:` list with `module` and `options` per entry.
- **Testdata YAMLs** (`cmd/forge/testdata/http-forge.yaml`, `cmd/forge/testdata/tui-forge.yaml`) already use `conduits:` format.
- **CLI** (`cmd/forge/main.go`) uses `blueprint.yaml` as default config and "blueprint" terminology throughout.

### Still Pending

- **No `agent/` package exists** — needed to orchestrate multiple conduits against one `session.Manager`.
- **`conduit/http.Handler` does not implement `Conduit`** — still exports `NewHandler(mgr, opts...)` and `ServeMux()`. Needs `New(mgr, opts...)` rename, `WithPort(port string)` option, and `Run(ctx)` method.
- **`conduit/tui.TUI` does not implement `Conduit`** — still exports `New(sess)` and `Run()`. Needs `New(mgr, opts...)` and `Run(ctx)` with context-aware graceful shutdown.
- **`main.go.tmpl` is still single-conduit** — uses hardcoded `{{if eq .ConduitType "http"}}` branches via `deriveConduitType`. Needs dynamic imports, agent construction, and a conduit loop.
- **Build pipeline lacks external module resolution** — `go mod tidy` handles standard imports, but explicit `go get` is needed for non-local conduit modules.
- **Example applications don't demonstrate agent pattern** — `examples/http-chat` and `examples/tui-chat` manually wire conduits instead of using `agent.Agent`.

### Relevant Files

| File | Role |
|------|------|
| `cmd/forge/templates/main.go.tmpl` | Hardcoded single-conduit template (needs rewrite) |
| `cmd/forge/templates/go.mod.tmpl` | Generated go.mod with local replace directive |
| `cmd/forge/generate.go` | `GenerateMainGo`, `GenerateGoMod`, `Generate` — needs template data overhaul |
| `cmd/forge/generate_test.go` | Tests old template output (needs update) |
| `cmd/forge/build.go` | `Build` — needs `go get` for external modules |
| `cmd/forge/blueprint.go` | `Blueprint` struct, `ParseBlueprint` (already correct) |
| `conduit/conduit.go` | `Conduit` interface (already correct) |
| `conduit/http/handler.go` | HTTP handler — needs `Run(ctx)`, `New` rename, `WithPort` |
| `conduit/http/handler_test.go` | HTTP handler tests |
| `conduit/tui/tui.go` | TUI — needs `New(mgr)`, `Run(ctx)`, `WithThreadID` |
| `conduit/tui/tui_test.go` | TUI tests |
| `session/manager.go` | `Manager` — agent will hold a reference |
| `loop/loop.go` | `Step` — used by session manager |
| `examples/http-chat/main.go` | Manual HTTP composition example |
| `examples/tui-chat/main.go` | Manual TUI composition example |
| `examples/forge/http/forge.yaml` | HTTP blueprint for smoke tests |
| `examples/forge/tui/forge.yaml` | TUI blueprint for smoke tests |

## Architectural Blueprint

### Selected Architecture

1. **`agent.Agent` package** (new `agent/`)  
   An orchestrator that holds a `*session.Manager` and a slice of `conduit.Conduit` implementations. It exposes `New(mgr)`, `Add(c)`, and `Run(ctx)` which starts all conduits concurrently and blocks until context cancellation or any conduit exits.

2. **Conduit standardization**  
   - `conduit/http`: rename `NewHandler` → `New`, add `WithPort(port string) Option`, implement `Run(ctx)` which creates the `http.Server`, starts `ListenAndServe` in a goroutine, and shuts down gracefully on `ctx.Done()`.  
   - `conduit/tui`: change `New(sess)` → `New(mgr, opts...)`, add `WithThreadID(id string) Option`, implement `Run(ctx)` which creates/attaches a session and starts the Bubble Tea program. A goroutine watches `ctx.Done()` and sends `tea.Quit()` for graceful shutdown.

3. **Template generation**  
   `main.go.tmpl` generates dynamic import aliases (derived from the last path segment of each module) to avoid collisions. It constructs `agent.New(mgr)`, loops over `conduits`, calls each package's `New(mgr, opts...)`, adds the result to the agent, and finally calls `agent.Run(ctx)`. For built-in ore conduits, well-known YAML options are translated into typed functional-option calls in the Go generation code (not the template itself). External conduits receive an empty option slice in the first iteration.

4. **Build pipeline**  
   After `go mod tidy`, `Build` iterates over `blueprint.Conduits` and for each module path that does **not** belong to the local `ore` module, runs `go get <modulePath>` inside the temp directory. This provides explicit, early failure with clear error messages when an external conduit is unavailable.

### Evaluated Alternatives

| Path | Why Rejected |
|------|-------------|
| **Template-only change, no agent package** | Would require the generated `main.go` to inline all concurrency wiring, making the template unmaintainable as conduit count grows. |
| **Keep `NewHandler` / `New(sess)` signatures** | Prevents generic looping in the template; every conduit would need a custom `if` branch, defeating multi-conduit support. |
| **Place `Conduit` interface in `agent/`** | Violates ore's package graph; `conduit/` is the natural home for the contract because concrete conduits import it, not the other way around. (Already placed correctly.) |
| **Support arbitrary option translation for external conduits in the template** | Requires runtime reflection or string-to-function mapping that is brittle; deferred to a future spike after the typed built-in path is validated. |

## Requirements

1. An `agent.Agent` type exists that can register multiple `conduit.Conduit` values and run them concurrently against one `session.Manager`.
2. `conduit/http.Handler` conforms to `Conduit` via `New(mgr, opts...)` and `Run(ctx)`.
3. `conduit/tui.TUI` conforms to `Conduit` via `New(mgr, opts...)` and `Run(ctx)`.
4. `cmd/forge/templates/main.go.tmpl` generates code that imports each conduit module dynamically, constructs an `agent.Agent`, loops over conduits, and calls `agent.Run(ctx)`.
5. The build pipeline resolves external conduit modules (runs `go get` when needed).
6. All existing tests and smoke tests are updated and continue to pass.
7. Example applications (`examples/http-chat`, `examples/tui-chat`) are updated to demonstrate the agent pattern.

## Task Breakdown

### Task 1: Refactor HTTP Handler for Conduit Interface
- **Goal**: Rename constructor, add port option, and implement `Run(ctx context.Context) error` so `*Handler` satisfies `conduit.Conduit`.
- **Dependencies**: None (conduit interface already exists).
- **Files Affected**: `conduit/http/handler.go`, `conduit/http/handler_test.go`, `examples/http-chat/main.go`
- **New Files**: None.
- **Interfaces**:
  ```go
  func New(mgr *session.Manager, opts ...Option) *Handler
  func WithPort(port string) Option
  func (h *Handler) Run(ctx context.Context) error
  ```
- **Validation**: `go test ./conduit/http/...` passes; `go build ./examples/http-chat` succeeds.
- **Details**:
  - Rename `NewHandler` → `New`.
  - Add `port` field to `Handler`. `WithPort` sets it; default `"8080"`.
  - `Run(ctx)` creates `http.Server` with `h.ServeMux()`, starts `ListenAndServe` in a goroutine, then blocks on `<-ctx.Done()`. On cancellation, call `server.Shutdown(context.Background())` (or a short timeout) and return `ctx.Err()`.
  - Update `examples/http-chat/main.go` to use `http.New(mgr, http.WithPort(port))` and `agent.New/Add/Run` instead of manual server wiring. Remove the `httpc` import alias.
  - Add a compile-time assertion in `handler_test.go` that `*Handler` satisfies `conduit.Conduit`.

### Task 2: Refactor TUI for Conduit Interface
- **Goal**: Change constructor to accept `*session.Manager`, add `WithThreadID` option, and implement `Run(ctx context.Context) error` so `*TUI` satisfies `conduit.Conduit`.
- **Dependencies**: None (conduit interface already exists).
- **Files Affected**: `conduit/tui/tui.go`, `conduit/tui/tui_test.go`, `examples/tui-chat/main.go`
- **New Files**: None.
- **Interfaces**:
  ```go
  func New(mgr *session.Manager, opts ...Option) *TUI
  func WithThreadID(id string) Option
  func (t *TUI) Run(ctx context.Context) error
  ```
- **Validation**: `go test ./conduit/tui/...` passes; `go build ./examples/tui-chat` succeeds.
- **Details**:
  - Add `Option` type and `WithThreadID` to the TUI package.
  - `New(mgr, opts...)` stores `mgr` and any options on `TUI`. It does **not** create/attach a session yet (session binding is deferred to `Run`).
  - `Run(ctx)` creates a session: if `WithThreadID` was set, calls `mgr.Attach(threadID)`; otherwise calls `mgr.Create()`. Then starts the Bubble Tea program.
  - Add a goroutine that watches `ctx.Done()` and sends `tea.Quit()` into the program for graceful shutdown.
  - Update `examples/tui-chat/main.go` to use `tui.New(mgr, tui.WithThreadID(os.Getenv("ORE_THREAD_ID")))` and the agent pattern. Remove the `--thread` CLI flag from the example.
  - Add a compile-time assertion in `tui_test.go` that `*TUI` satisfies `conduit.Conduit`.

### Task 3: Create `agent/` Orchestration Package
- **Goal**: Implement `Agent` that starts multiple `Conduit` values concurrently and blocks until shutdown.
- **Dependencies**: None (conduit interface already exists).
- **Files Affected**: None.
- **New Files**: `agent/agent.go`, `agent/agent_test.go`
- **Interfaces**:
  ```go
  func New(mgr *session.Manager) *Agent
  func (a *Agent) Add(c conduit.Conduit)
  func (a *Agent) Run(ctx context.Context) error
  ```
- **Validation**: `go test ./agent/...` passes. Use local mock structs in the test file to satisfy `conduit.Conduit`.
- **Details**:
  - `Agent` holds `*session.Manager` and `[]conduit.Conduit`.
  - `Run(ctx)` starts each conduit in its own goroutine, collecting errors via an errgroup or a dedicated channel.
  - On `ctx.Done()`, return `ctx.Err()` after waiting for all conduits to finish.
  - If any conduit returns a non-nil error, propagate it and initiate shutdown of siblings via `ctx.Cancel()`.
  - The package must import `conduit/` and `session/` only.

### Task 4: Rewrite `main.go.tmpl` for Multi-Conduit Agent Pattern
- **Goal**: Generate code that imports each conduit dynamically, constructs an agent, and loops over conduits.
- **Dependencies**: Task 1 (HTTP `New` signature), Task 2 (TUI `New` signature), Task 3 (agent package).
- **Files Affected**: `cmd/forge/templates/main.go.tmpl`, `cmd/forge/generate.go`, `cmd/forge/generate_test.go`
- **New Files**: None.
- **Interfaces**:
  ```go
  // In generate.go
  type conduitData struct {
      ModulePath  string
      ImportAlias string
      Options     []string  // e.g. `http.WithPort("8080")`
      IsBuiltin   bool
  }
  func GenerateMainGo(blueprint *Blueprint, oreModulePath string) ([]byte, error)
  ```
- **Validation**: `go test ./cmd/forge/...` passes; updated `TestGenerateMainGo` asserts dynamic imports and agent loop are present.
- **Details**:
  - Derive a unique import alias from the last path segment of each `module` (e.g. `http` from `.../conduit/http`, `tui` from `.../conduit/tui`). Collisions resolved by appending a counter.
  - For built-in ore conduits, translate known YAML options into typed Go option strings:
    - `port: "8080"` (or `8080`) → `"8080"` passed to `http.WithPort`
    - `ui: true` → `http.WithUI()`
    - `thread: "<id>"` → `tui.WithThreadID("<id>")`
  - External conduits are constructed with `alias.New(mgr)` (no options in the first iteration).
  - The generated `main.go` must still be valid Go: run `parser.ParseFile` in `GenerateMainGo` to verify.
  - Remove `deriveConduitType` from `generate.go` entirely.
  - Template structure:
    - Import `github.com/andrewhowdencom/ore/agent`
    - Dynamic imports for each conduit module
    - `agent.New(mgr)`
    - Loop over conduits calling `agent.Add(alias.New(mgr, opts...))`
    - `agent.Run(ctx)`

### Task 5: Support `go get` Resolution for External Conduit Modules
- **Goal**: Ensure external modules listed in `conduits:` can be fetched during the build.
- **Dependencies**: None (blueprint format already correct).
- **Files Affected**: `cmd/forge/build.go`, `cmd/forge/build_test.go`
- **New Files**: None.
- **Interfaces**: No new exported interfaces; `Build(blueprint *Blueprint, oreModulePath string, outputPath string) error` is updated.
- **Validation**: `go test ./cmd/forge/...` passes; smoke tests still pass.
- **Details**:
  - After writing `go.mod` and `main.go` to the temp directory and before `go mod tidy`, parse the local ore module name from `tmpDir/go.mod` (it is already generated with the correct module name).
  - Iterate over `blueprint.Conduits`. For each module path that does **not** start with the local ore module name + `/`, run `go get <modulePath>` inside the temp directory.
  - Then run `go mod tidy` as before.
  - Wrap `go get` in explicit error handling: if it fails, return a clear error including the module path.
  - For built-in ore conduits (e.g. `github.com/andrewhowdencom/ore/conduit/http`), `go get` is skipped because the replace directive makes them resolvable.

### Task 6: Update Examples and Testdata for Agent Pattern
- **Goal**: Convert manual composition examples to use `agent.Agent`, and update smoke test expectations.
- **Dependencies**: Task 1 (HTTP `Run`), Task 2 (TUI `Run`), Task 4 (template generates agent code).
- **Files Affected**:
  - `examples/http-chat/main.go`
  - `examples/tui-chat/main.go`
  - `cmd/forge/testdata/http-forge.yaml`
  - `cmd/forge/testdata/tui-forge.yaml`
  - `examples/forge/http/forge.yaml`
  - `examples/forge/tui/forge.yaml`
  - `cmd/forge/forge_test.go`
- **New Files**: None.
- **Validation**: `go test ./cmd/forge/...` passes; `go build ./examples/http-chat` and `go build ./examples/tui-chat` succeed.
- **Details**:
  - `examples/http-chat/main.go` and `examples/tui-chat/main.go` should construct a `session.Manager`, create `agent.New(mgr)`, add the respective conduit, and call `agent.Run(ctx)`. This serves as living documentation.
  - Update YAML comments in example blueprints to reference `agent.Agent` and multi-conduit composition.
  - `cmd/forge/forge_test.go` `TestForgeSmoke` references `forge.yaml` paths. If any filenames changed (they didn't — they were already updated to `conduits:`), ensure paths are correct. The test should pass because `Build` now compiles agent-based binaries.

### Task 7: Full Integration Validation
- **Goal**: Verify the entire repository is healthy after all changes.
- **Dependencies**: All preceding tasks.
- **Files Affected**: None (test-only).
- **Validation**:
  - `go test -race ./...` passes.
  - `go build ./...` passes.
  - `go vet ./...` is clean.
  - `cmd/forge` smoke tests (`TestForgeSmoke`, `TestForgeSmoke_RuntimeGuard`) pass with the new blueprints.
- **Details**:
  - Run the full test suite. If any race is detected in `agent.Agent` goroutine management, fix it before marking this task complete.
  - Verify the generated `main.go` for a single HTTP conduit no longer contains hardcoded `if eq .ConduitType` branches.
  - Verify the generated `main.go` for a single TUI conduit compiles and runs (runtime guard test exits with "ORE_API_KEY not set").

## Dependency Graph

- Task 1 || Task 2 || Task 3 (parallel after existing Conduit interface)
- Task 1 → Task 4
- Task 2 → Task 4
- Task 3 → Task 4
- Task 4 → Task 6
- Task 5 || Task 6 (parallel)
- Task 1 → Task 6
- Task 2 → Task 6
- Task 6 → Task 7
- Task 5 → Task 7

Critical path: **Task 1 → Task 4 → Task 6 → Task 7** (or Task 2 instead of Task 1)

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|---|---|---|---|
| **Bubble Tea context cancellation is non-trivial** | TUI `Run(ctx)` may not exit cleanly | Medium | In Task 2, implement `tea.Quit()` on `ctx.Done()` and verify with a manual test. The TUI test can use a short timeout context. |
| **External conduit module paths fail `go get`** | Build breaks for any non-local conduit | Medium | In Task 5, wrap `go get` in explicit error handling. Document that external modules must be reachable by the host Go toolchain. |
| **Template import alias collisions** | Generated code has duplicate import aliases | Low | In Task 4, deduplicate aliases by appending a numeric suffix (e.g. `http2`) when the last path segment repeats. |
| **YAML option type translation is brittle** | Built-in conduits evolve and template falls out of sync | Medium | Keep the option mapping minimal (port, ui, thread_id). Add a design note in the plan: future work should define `conduit.WithOptions(map[string]any)` for generic external conduits. |
| **Agent goroutine leak on error** | One conduit error may not cleanly stop others | Low | In Task 3, use `context.WithCancelCause` or errgroup to propagate cancellation. Test with a mock conduit that returns an error immediately. |

## Validation Criteria

- [ ] `go test -race ./conduit/...` passes.
- [ ] `go test -race ./agent/...` passes.
- [ ] `go test -race ./cmd/forge/...` passes.
- [ ] `go build ./examples/http-chat` succeeds.
- [ ] `go build ./examples/tui-chat` succeeds.
- [ ] A Forge smoke test using an HTTP blueprint compiles and produces a binary.
- [ ] A Forge smoke test using a TUI blueprint compiles and produces a binary.
- [ ] The generated `main.go` for a single HTTP conduit no longer contains hardcoded `if eq .ConduitType` branches.
- [ ] The generated `main.go` contains `agent.New(mgr)` and `agent.Run(ctx)`.
- [ ] The term "manifest" no longer appears in user-facing code (already done; verify no regressions).
- [ ] `conduit/http` and `conduit/tui` both have compile-time assertions that they implement `conduit.Conduit`.
