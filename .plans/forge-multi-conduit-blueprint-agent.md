# Plan: Forge Multi-Conduit Blueprint and Agent Integration

## Objective

Transform Forge from a single-conduit code generator into a multi-conduit blueprint system. Introduce an `agent/` orchestration package and a standardized `conduit.Conduit` interface so that Forge-generated applications can host multiple frontends (HTTP, TUI, and future external conduits) against a shared `session.Manager`. Extend the YAML manifest format to a multi-conduit blueprint, rename the configuration concept from "manifest" to "blueprint" per #95, and teach the build pipeline to resolve external conduit modules via `go get`.

## Context

### Current State

- **Forge CLI** (`cmd/forge/`) reads a YAML file with a single `conduit.type` field (`http` or `tui`) and generates a hardcoded `main.go` with explicit `{{if eq .ConduitType "http"}}` branches.
- The generated `main.go` directly wires a `session.Manager` into either `http.NewHandler(mgr)` or `tui.New(sess)` — there is no shared orchestration layer.
- **No `agent` package exists** in the repository.
- **No `conduit.Conduit` interface exists** — HTTP handler and TUI have incompatible signatures (`NewHandler(mgr, ...)` vs `New(sess)`).
- The YAML "manifest" concept has no "recipe" terminology anywhere in code, but issue #95 decrees the smithing metaphor: configurations shall be called **blueprints**.
- The build pipeline (`cmd/forge/build.go`) runs `go mod tidy` and `go build` inside a temp module that only replaces the local `ore` module; it has no explicit handling for external conduit modules.

### Relevant Files Discovered

| File | Role |
|------|------|
| `cmd/forge/templates/main.go.tmpl` | Hardcoded single-conduit template |
| `cmd/forge/templates/go.mod.tmpl` | Generated go.mod with local replace directive |
| `cmd/forge/manifest.go` | `Manifest` struct and `ParseManifest` |
| `cmd/forge/generate.go` | `GenerateMainGo`, `GenerateGoMod`, `Generate` |
| `cmd/forge/build.go` | `Build` — temp module, tidy, compile |
| `cmd/forge/main.go` | Cobra CLI commands |
| `cmd/forge/generate_test.go` | Template generation tests |
| `cmd/forge/manifest_test.go` | Manifest parsing tests |
| `cmd/forge/build_test.go` | Build integration tests |
| `cmd/forge/forge_test.go` | Smoke tests against testdata + examples |
| `cmd/forge/cmd_generate_test.go` | CLI-level generate tests |
| `cmd/forge/testdata/http-forge.yaml` | Single-conduit HTTP test manifest |
| `cmd/forge/testdata/tui-forge.yaml` | Single-conduit TUI test manifest |
| `examples/forge/http/forge.yaml` | Example HTTP manifest |
| `examples/forge/tui/forge.yaml` | Example TUI manifest |
| `conduit/conduit.go` | `Capability`, `Descriptor` — no `Conduit` interface |
| `conduit/event.go` | `Event`, `UserMessageEvent`, `InterruptEvent` |
| `conduit/http/handler.go` | `NewHandler(mgr, opts...) *Handler` |
| `conduit/tui/tui.go` | `New(sess session.Session) *TUI` |
| `session/manager.go` | `Manager`, `Create`, `Attach`, `Process` |
| `loop/loop.go` | `Step`, `Turn`, `Submit`, `FanOut` |
| `examples/http-chat/main.go` | Manual HTTP conduit composition example |
| `examples/tui-chat/main.go` | Manual TUI conduit composition example |

## Architectural Blueprint

### Selected Architecture

1. **`conduit.Conduit` interface** (`conduit/conduit.go`)  
   A minimal runnable contract: `Run(ctx context.Context) error`. This is intentionally simple so that any frontend — HTTP server, Bubble Tea TUI, Kafka consumer, email sender — can implement it without importing `session/` or `agent/`.

2. **`agent.Agent` package** (new `agent/`)  
   An orchestrator that holds a `*session.Manager` and a slice of `conduit.Conduit` implementations. It exposes `New(mgr)`, `Add(c)`, and `Run(ctx)` which starts all conduits concurrently and blocks until context cancellation or any conduit exits.

3. **Conduit standardization**  
   - `conduit/http`: rename `NewHandler` → `New`, add `WithPort(port string) Option`, implement `Run(ctx)` which creates the `http.Server` and listens.  
   - `conduit/tui`: change `New(sess)` → `New(mgr, opts...)`, add `WithThreadID(id string) Option`, implement `Run(ctx)` which creates/attaches a session and starts the Bubble Tea program. Thread ID moves from a CLI flag into a conduit option (with `ORE_THREAD_ID` env fallback).

4. **Blueprint format**  
   - Rename `Manifest` → `Blueprint`, `ParseManifest` → `ParseBlueprint`.  
   - Replace the single `conduit:` key with a `conduits:` list. Each entry has `module` (Go import path) and `options` (YAML map).  
   - Default config file name becomes `blueprint.yaml`.

5. **Template generation**  
   - `main.go.tmpl` generates dynamic import aliases (derived from the last path segment of each module) to avoid collisions.  
   - It constructs `agent.New(mgr)`, loops over `conduits`, calls each package's `New(mgr, opts...)`, adds the result to the agent, and finally calls `agent.Run(ctx)`.
   - For built-in ore conduits (`conduit/http`, `conduit/tui`) the template hard-codes the translation of well-known YAML options into typed functional-option calls. External conduits receive an empty option slice in the first iteration; a future spike can introduce a generic `WithOptions(map[string]any)` convention.

6. **Build pipeline**  
   - After generating `go.mod` and `main.go`, the `Build` function runs `go mod tidy` (which naturally resolves new imports). For explicit external modules, it additionally runs `go get <module>` so that private or non-proxy modules are fetched with clear error messages.

### Evaluated Alternatives

| Path | Why Rejected |
|------|-------------|
| **Template-only change, no agent package** | Would require the generated `main.go` to inline all concurrency wiring, making the template unmaintainable as conduit count grows. |
| **Keep `NewHandler` / `New(sess)` signatures** | Prevents generic looping in the template; every conduit would need a custom `if` branch, defeating multi-conduit support. |
| **Place `Conduit` interface in `agent/`** | Violates ore's package graph; `conduit/` is the natural home for the contract because concrete conduits import it, not the other way around. |
| **Support arbitrary option translation for external conduits in the template** | Requires runtime reflection or string-to-function mapping that is brittle; deferred to a future spike after the typed built-in path is validated. |

## Requirements

1. A `conduit.Conduit` interface exists with `Run(ctx context.Context) error`.
2. An `agent.Agent` type exists that can register multiple `Conduit` values and run them concurrently against one `session.Manager`.
3. `conduit/http.Handler` conforms to `Conduit` via `New(mgr, opts...)` and `Run(ctx)`.
4. `conduit/tui.TUI` conforms to `Conduit` via `New(mgr, opts...)` and `Run(ctx)`.
5. The YAML format supports a `conduits:` list with `module` and `options` per entry.
6. The Forge parser, generator, and CLI use the term **blueprint** instead of **manifest**.
7. `cmd/forge/templates/main.go.tmpl` generates code that imports each conduit module dynamically, constructs an `agent.Agent`, loops over conduits, and calls `agent.Run(ctx)`.
8. The build pipeline resolves external conduit modules (runs `go get` when needed).
9. All existing tests and smoke tests are updated and continue to pass.
10. Example applications (`examples/http-chat`, `examples/tui-chat`) are updated to demonstrate the agent pattern.

## Task Breakdown

### Task 1: Define `conduit.Conduit` Interface
- **Goal**: Add a `Conduit` interface to `conduit/conduit.go` so all frontends share a runnable contract.
- **Dependencies**: None.
- **Files Affected**: `conduit/conduit.go`, `conduit/conduit_test.go`
- **New Files**: None.
- **Interfaces**: 
  ```go
  type Conduit interface {
      Run(ctx context.Context) error
  }
  ```
- **Validation**: `go test ./conduit/...` passes.
- **Details**: Add the interface and a compile-time assertion in the test file that both `*http.Handler` and `*tui.TUI` satisfy it (they will not yet compile, which is expected until Tasks 3 and 4 land; the assertion can be added in those tasks).

### Task 2: Create `agent/` Orchestration Package
- **Goal**: Implement `Agent` that starts multiple `Conduit` values concurrently and blocks until shutdown.
- **Dependencies**: Task 1.
- **Files Affected**: None.
- **New Files**: `agent/agent.go`, `agent/agent_test.go`
- **Interfaces**:
  ```go
  func New(mgr *session.Manager) *Agent
  func (a *Agent) Add(c conduit.Conduit)
  func (a *Agent) Run(ctx context.Context) error
  ```
- **Validation**: `go test ./agent/...` passes. Use local mock structs in the test file to satisfy `conduit.Conduit`.
- **Details**: Start each conduit in a goroutine. On `ctx.Done()`, return `ctx.Err()`. If any conduit returns a non-nil error, propagate it and wait for siblings to finish. The package must import `conduit/` and `session/` only.

### Task 3: Refactor HTTP Handler for `Conduit` Interface
- **Goal**: Rename constructor, add port option, and implement `Run(ctx context.Context) error`.
- **Dependencies**: Task 1.
- **Files Affected**: `conduit/http/handler.go`, `conduit/http/handler_test.go`, `examples/http-chat/main.go`
- **New Files**: None.
- **Interfaces**:
  ```go
  func New(mgr *session.Manager, opts ...Option) *Handler
  func WithPort(port string) Option
  func (h *Handler) Run(ctx context.Context) error
  ```
- **Validation**: `go test ./conduit/http/...` passes; `go build ./examples/http-chat` succeeds.
- **Details**: Move `http.Server` creation and `ListenAndServe` from application code into `Handler.Run`. Store the port on `Handler`. Update `examples/http-chat/main.go` to use `http.New(mgr, http.WithPort(port))` and `agent.New/Add/Run` instead of manual server wiring. Remove the `httpc` import alias from the example in favour of the plain package name if it no longer collides with `net/http`.

### Task 4: Refactor TUI for `Conduit` Interface
- **Goal**: Change constructor to accept `*session.Manager`, add `WithThreadID` option, and implement `Run(ctx context.Context) error`.
- **Dependencies**: Task 1.
- **Files Affected**: `conduit/tui/tui.go`, `conduit/tui/tui_test.go`, `examples/tui-chat/main.go`
- **New Files**: None.
- **Interfaces**:
  ```go
  func New(mgr *session.Manager, opts ...Option) *TUI
  func WithThreadID(id string) Option
  func (t *TUI) Run(ctx context.Context) error
  ```
- **Validation**: `go test ./conduit/tui/...` passes; `go build ./examples/tui-chat` succeeds.
- **Details**: `Run` creates a new session (or attaches if `WithThreadID` is set) and then starts the Bubble Tea program. Add goroutine that watches `ctx.Done()` and sends `tea.Quit()` for graceful shutdown. Update `examples/tui-chat/main.go` to use `tui.New(mgr)` and the agent pattern. The `--thread` CLI flag moves from the example's `main.go` into a `WithThreadID(os.Getenv("ORE_THREAD_ID"))` option or similar.

### Task 5: Rename Manifest to Blueprint and Extend for Multi-Conduit
- **Goal**: Replace the single-conduit `Manifest` with a multi-conduit `Blueprint`, rename parser and validation, and update all Forge code.
- **Dependencies**: None (rename can proceed independently; it touches the same files as Task 6 but is structurally orthogonal).
- **Files Affected**:
  - `cmd/forge/manifest.go` → `cmd/forge/blueprint.go`
  - `cmd/forge/manifest_test.go` → `cmd/forge/blueprint_test.go`
  - `cmd/forge/generate.go`, `cmd/forge/build.go`, `cmd/forge/main.go`
  - All test files in `cmd/forge/`
- **New Files**: `cmd/forge/blueprint.go`, `cmd/forge/blueprint_test.go`.
- **Interfaces**:
  ```go
  type Blueprint struct {
      Dist     Dist            `yaml:"dist"`
      Conduits []ConduitConfig `yaml:"conduits"`
  }
  type ConduitConfig struct {
      Module  string         `yaml:"module"`
      Options map[string]any `yaml:"options"`
  }
  func ParseBlueprint(r io.Reader) (*Blueprint, error)
  ```
- **Validation**: `go test ./cmd/forge/...` passes after updating all test expectations.
- **Details**: 
  - Rename `Manifest` → `Blueprint`, `ParseManifest` → `ParseBlueprint`.
  - Remove the old `Conduit struct` and `Conduit.Type` validation.
  - Require `dist.name`, `dist.output_path`, and at least one entry in `conduits`.
  - Update CLI default `--config` from `forge.yaml` to `blueprint.yaml`.
  - Update `cmd/forge/generate.go` function signatures to accept `*Blueprint`.
  - Update `cmd/forge/build.go` function signatures to accept `*Blueprint`.

### Task 6: Rewrite `main.go.tmpl` for Multi-Conduit Agent Pattern
- **Goal**: Generate code that imports each conduit dynamically, constructs an agent, and loops over conduits.
- **Dependencies**: Task 2 (agent package exists), Task 3 (HTTP `New` signature), Task 4 (TUI `New` signature), Task 5 (blueprint format).
- **Files Affected**: `cmd/forge/templates/main.go.tmpl`, `cmd/forge/generate.go`, `cmd/forge/generate_test.go`
- **New Files**: None.
- **Interfaces**:
  ```go
  // Template data structure
  type mainGoData struct {
      ModuleName    string
      OreModulePath string
      Conduits      []conduitData
      HasExternal   bool
  }
  type conduitData struct {
      ModulePath  string
      ImportAlias string
      Options     map[string]any
      IsBuiltin   bool
  }
  func GenerateMainGo(blueprint *Blueprint, oreModulePath string) ([]byte, error)
  ```
- **Validation**: `go test ./cmd/forge/...` passes; updated `TestGenerateMainGo` asserts dynamic imports and agent loop are present.
- **Details**:
  - Derive a unique import alias from the last path segment of each `module` (e.g. `http` from `.../conduit/http`, `tui` from `.../conduit/tui`). Collisions can be resolved by appending a counter.
  - For built-in ore conduits, translate known options into typed functional options in the template:
    - `port: "8080"` → `http.WithPort("8080")`
    - `ui: true` → `http.WithUI()`
    - `thread: "<id>"` → `tui.WithThreadID("<id>")`
  - External conduits are constructed with `alias.New(mgr)` (no options in the first iteration).
  - The generated `main.go` must still be valid Go: run `parser.ParseFile` in `GenerateMainGo` to verify.

### Task 7: Support `go get` Resolution for External Conduit Modules
- **Goal**: Ensure external modules listed in `conduits:` can be fetched during the build.
- **Dependencies**: Task 5 (blueprint has module list).
- **Files Affected**: `cmd/forge/build.go`, `cmd/forge/templates/go.mod.tmpl`
- **New Files**: None.
- **Interfaces**: No new exported interfaces; `Build(blueprint *Blueprint, oreModulePath string, outputPath string) error` is updated.
- **Validation**: `go test ./cmd/forge/...` passes; smoke tests still pass.
- **Details**: After writing `go.mod` and `main.go` to the temp directory, iterate over `blueprint.Conduits` and for each module path that does **not** belong to the local `ore` module, run `go get <modulePath>` inside the temp directory before `go mod tidy`. This provides explicit, early failure with clear error messages when an external conduit is unavailable. `go mod tidy` already resolves standard imports; the explicit `go get` is a safeguard for external/private modules.

### Task 8: Update Testdata and Example Manifests to New Blueprint Format
- **Goal**: Convert all YAML manifests into multi-conduit blueprints so tests and examples compile.
- **Dependencies**: Task 5 (blueprint format), Task 3 (HTTP options), Task 4 (TUI options).
- **Files Affected**:
  - `cmd/forge/testdata/http-forge.yaml`
  - `cmd/forge/testdata/tui-forge.yaml`
  - `examples/forge/http/forge.yaml`
  - `examples/forge/tui/forge.yaml`
- **New Files**: Optionally `examples/forge/multi/blueprint.yaml` (multi-conduit smoke example).
- **Validation**: `go test ./cmd/forge/...` passes; smoke tests in `TestForgeSmoke` pass.
- **Details**: Update each YAML to use `conduits:` with `module` and `options`. For example:
  ```yaml
  dist:
    name: http-chat
    output_path: ./http-chat
  conduits:
    - module: github.com/andrewhowdencom/ore/conduit/http
      options:
        port: "8080"
  ```
  Smoke tests in `forge_test.go` that reference manifest paths must be updated to `blueprint.yaml` if filenames change.

### Task 9: Update Example Applications to Demonstrate Agent Pattern
- **Goal**: Rewrite `examples/http-chat` and `examples/tui-chat` to use `agent.Agent`.
- **Dependencies**: Task 2, Task 3, Task 4.
- **Files Affected**: `examples/http-chat/main.go`, `examples/tui-chat/main.go`
- **New Files**: None.
- **Validation**: `go build ./examples/http-chat` and `go build ./examples/tui-chat` succeed.
- **Details**: Both examples should construct a `session.Manager`, create `agent.New(mgr)`, add the respective conduit, and call `agent.Run(ctx)`. This serves as living documentation for the new composition pattern.

### Task 10: Full Integration Validation
- **Goal**: Verify the entire repository is healthy after all changes.
- **Dependencies**: All preceding tasks.
- **Files Affected**: None (test-only).
- **Validation**: 
  - `go test -race ./...` passes.
  - `go build ./...` passes.
  - `go vet ./...` is clean.
  - `cmd/forge` smoke tests (`TestForgeSmoke`, `TestForgeSmoke_RuntimeGuard`) pass with the new blueprints.
- **Details**: Run the full test suite. If any race is detected in `agent.Agent` goroutine management, fix it before marking this task complete.

## Dependency Graph

- Task 1 → Task 2
- Task 1 → Task 3
- Task 1 → Task 4
- Task 2 || Task 3 || Task 4 || Task 5 (Tasks 2–5 can run in parallel after Task 1, except Task 2/3/4 need Task 1)
- Task 3 → Task 9
- Task 4 → Task 9
- Task 5 → Task 6
- Task 5 → Task 7
- Task 5 → Task 8
- Task 2 → Task 6
- Task 3 → Task 6
- Task 4 → Task 6
- Task 6 → Task 10
- Task 7 → Task 10
- Task 8 → Task 10
- Task 9 → Task 10

Critical path: **Task 1 → Task 3/4 → Task 6 → Task 10**

## Risks & Mitigations

| Risk | Impact | Likelihood | Mitigation |
|------|--------|------------|------------|
| **Bubble Tea context cancellation is non-trivial** | TUI `Run(ctx)` may not exit cleanly | Medium | Spike a minimal Bubble Tea + context prototype in Task 4 before committing to the full refactor. Use `tea.Quit()` message on `ctx.Done()`. |
| **External conduit module paths fail `go get`** | Build breaks for any non-local conduit | Medium | In Task 7, wrap `go get` in explicit error handling and fall back to `go mod tidy` alone if `go get` is unnecessary. Document that external modules must be reachable by the host Go toolchain. |
| **Template import alias collisions** | Generated code has duplicate import aliases | Low | In Task 6, deduplicate aliases by appending a numeric suffix (e.g. `http2`) when the last path segment repeats. |
| **YAML option type translation is brittle** | Built-in conduits evolve and template falls out of sync | Medium | Keep the option mapping minimal (port, ui, thread_id). Add a design note in the plan: future work should define `conduit.WithOptions(map[string]any)` for generic external conduits. |
| **Manifest rename touches many tests** | Large merge conflict surface | Medium | Do the rename aggressively in Task 5 so downstream tasks (6, 7, 8) work on the new names from the start. |

## Validation Criteria

- [ ] `go test -race ./conduit/...` passes.
- [ ] `go test -race ./agent/...` passes.
- [ ] `go test -race ./cmd/forge/...` passes.
- [ ] `go build ./examples/http-chat` succeeds.
- [ ] `go build ./examples/tui-chat` succeeds.
- [ ] A Forge smoke test using a multi-conduit blueprint compiles and produces a binary.
- [ ] The generated `main.go` for a single HTTP conduit no longer contains hardcoded `if eq .ConduitType` branches.
- [ ] The term "blueprint" appears in `ParseBlueprint`, `Blueprint` struct, and CLI help text; the term "manifest" no longer appears in user-facing code.
- [ ] No "recipe" terminology remains in the repository (confirmed zero references at start of work).
