# agentfleet Library Refactor — Design Spec

**Date:** 2026-05-31
**Branch:** feat/initial-agentfleet
**Status:** Approved for implementation

---

## Overview

Refactor agentfleet from a single CLI tool into a reusable Go library. The library provides core abstractions (`Task`, `Fleet`, `Runner`, `Manager`, `Agent`, `Hook`) that consumers import to build their own agent orchestration tools. Concrete implementations (managers, step injection, the TUI binary) move to `examples/`. The module path `github.com/hoaitan/agentfleet` and the names `Fleet` / `agentfleet` are kept.

---

## 1. Package Layout

```
github.com/hoaitan/agentfleet/         ← root package "agentfleet"
  task.go                               Task interface (no Steps)
  fleet.go                              Fleet struct — dynamic runner registry
  runner.go                             Runner struct — lifecycle wrapper, no step logic
  manager.go                            Manager interface
  agent.go                              Agent interface + PtyAgent + MockAgent
  config.go                             Config, DefaultConfig(), AgentConfigFromTerminal()

github.com/hoaitan/agentfleet/tui/     ← "tui" package
  tui.go                                Bubbletea program, takes *Fleet + TUIConfig

github.com/hoaitan/agentfleet/source/  ← "source" package
  source.go                             Source interface
  file.go                               FileSource (JSON/YAML)
  markdown.go                           MarkdownSource
  http.go                               HTTPSource
  generate.go                           GenerateSource (Claude API)

github.com/hoaitan/agentfleet/hook/    ← "hook" package
  hook.go                               Hook interface, Dir constants, Chain
  file_logger.go                        FileLogger implementation

internal/proxy/                        ← unchanged, stays internal

examples/
  file-manager/main.go                  FileManager: JSON/YAML/Markdown → StepTask → Fleet + TUI
  http-manager/main.go                  HTTPManager: HTTP endpoint → StepTask → Fleet + TUI
  generate-manager/main.go             GenerateManager: Claude → tasks → Fleet + TUI
  attach/main.go                        attach binary (moved from cmd/attach)
  taskserver/main.go                    example HTTP task server (unchanged)
```

### What moves where

| Current location | New location |
|------------------|--------------|
| `internal/fleet/` | root package |
| `internal/agent/` | root package |
| `internal/hook/` | `hook/` sub-package |
| `internal/source/` | `source/` sub-package |
| `internal/proxy/` | `internal/proxy/` (unchanged) |
| `cmd/agentfleet/` | split across `examples/file-manager/`, `examples/http-manager/`, `examples/generate-manager/` |
| `cmd/attach/` | `examples/attach/` |
| `Task.Steps()` | removed from interface; lives as `StepTask` in examples |
| `Runner.runSteps()` | removed from library; example helper `injectSteps()` |

---

## 2. Core Interfaces

### Task

```go
// Task is the minimum contract for any runnable unit.
// No step injection — scheduling is the caller's responsibility.
type Task interface {
    ID()      string
    Name()    string
    Command() string // CLI binary to spawn, e.g. "claude --no-update"
}
```

### Fleet

```go
// Fleet is a thread-safe, dynamic registry of runners.
// Managers add runners as tasks arrive; the TUI reads from it.
type Fleet struct { ... }

func NewFleet(cfg FleetConfig) *Fleet
func (f *Fleet) Add(r *Runner)                       // blocks when MaxConcurrent is reached
func (f *Fleet) Runners() []*Runner                  // snapshot for TUI
func (f *Fleet) Wait(ctx context.Context) error      // returns on all done or ctx cancel
```

`Add()` enforces `FleetConfig.MaxConcurrent`: when the count of running runners hits the limit it blocks until a slot opens (a runner transitions to Done or Failed). Managers call `Add()` freely without tracking concurrency themselves.

### Runner

```go
// Runner manages one CLI agent session. It has no knowledge of steps.
type Runner struct { ... }

func NewRunner(task Task, ag Agent) *Runner
func (r *Runner) Start()
func (r *Runner) Stop() error
func (r *Runner) Status() Status
func (r *Runner) Done() <-chan struct{}
func (r *Runner) StdinWriter() io.Writer  // example step injectors write here
func (r *Runner) Lines() []string         // ring buffer snapshot for TUI preview
func (r *Runner) Task() Task
func (r *Runner) StartedAt() time.Time
func (r *Runner) FinishedAt() time.Time
```

### Manager

```go
// Manager is the single extension point for orchestration strategy.
// It receives a Fleet and drives tasks into it — loading, streaming, or generating.
type Manager interface {
    Run(ctx context.Context, fleet *Fleet) error
}
```

### Agent

```go
// Agent is any interactive CLI process running in a PTY.
type Agent interface {
    Start(rows, cols int) error
    Write(p []byte) (int, error)
    Read(p []byte) (int, error)
    Resize(rows, cols int) error
    Stop() error
    Done() <-chan struct{}
}
```

Built-in implementations: `PtyAgent` (wraps `creack/pty`), `MockAgent` (in-memory, for tests).

### Hook

All hook types live in the `hook/` sub-package. Root package and `internal/proxy` import it.

```go
// package hook
type Dir uint8
const (DirIn Dir = iota; DirOut)

type Hook interface {
    Process(data []byte, dir Dir) ([]byte, error)
}

// Chain applies hooks in order; returning nil bytes suppresses data; error skips that hook.
type Chain []Hook
func (c Chain) Process(data []byte, dir Dir) ([]byte, error)

// FileLogger logs all bytes to a file.
type FileLogger struct { Path string }
```

Consumers import `github.com/hoaitan/agentfleet/hook` to implement custom hooks or use `FileLogger`.

### Source

```go
// in package source
type Source interface {
    Load() ([]agentfleet.Task, error)
}
```

Built-in: `FileSource` (JSON/YAML), `MarkdownSource`, `HTTPSource`, `GenerateSource` (Claude API).

---

## 3. Configuration

```go
type Config struct {
    Fleet FleetConfig
    TUI   TUIConfig
    Agent AgentConfig
}

type FleetConfig struct {
    MaxConcurrent  int    // max tasks running in parallel — default: 9
    RingBufferSize int    // output lines kept per runner   — default: 200
    SocketDir      string // Unix socket dir               — default: /tmp
    LogDir         string // session log dir               — default: /tmp
}

type TUIConfig struct {
    Columns      int           // grid columns               — default: 3
    PreviewLines int           // output lines shown in card — default: 3
    CardWidth    int           // card width in chars        — default: 64
    RefreshRate  time.Duration // TUI tick interval          — default: 500ms
}

type AgentConfig struct {
    PTYRows int // default: 24
    PTYCols int // default: 220
}

func DefaultConfig() Config { ... }

// AgentConfigFromTerminal reads actual terminal dimensions.
// Falls back to DefaultConfig().Agent when stdout is not a TTY.
func AgentConfigFromTerminal() AgentConfig { ... }
```

Callers override only what they need:
```go
cfg := agentfleet.DefaultConfig()
cfg.Fleet.MaxConcurrent = 4
cfg.Agent = agentfleet.AgentConfigFromTerminal()
fleet := agentfleet.NewFleet(cfg.Fleet)
```

---

## 4. Data Flow

```
main()
  │
  ├─ cfg  := agentfleet.DefaultConfig()         (override as needed)
  ├─ fleet := agentfleet.NewFleet(cfg.Fleet)
  ├─ mgr  := NewFileManager(source, cfg)        (example-layer type)
  │
  ├─ go mgr.Run(ctx, fleet)
  │     │
  │     ├─ load / stream tasks from source
  │     ├─ for each task:
  │     │     ag := agentfleet.NewPtyAgent(fields, cfg.Agent)
  │     │     r  := agentfleet.NewRunner(task, ag)
  │     │     fleet.Add(r)          ← blocks if MaxConcurrent reached
  │     │     r.Start()
  │     │     go injectSteps(r, task.Steps)   ← example-layer only
  │     │
  │     └─ fleet.Wait(ctx)          ← blocks until all done or ctx cancelled
  │
  └─ tui.Run(ctx, fleet, cfg.TUI)  ← reads fleet.Runners(), refreshes on tick
        └─ on Enter: open attach tab for selected runner
```

### Context cancellation

- `main()` cancels ctx on `q` / Ctrl-C / OS signal
- `Manager.Run()` receives cancellation → stops accepting new tasks → returns
- `Fleet.Wait()` unblocks → main exits
- Remaining running runners get `Stop()` called in cleanup

### Step injection (example layer only)

```go
// injectSteps lives in examples, not in the library.
func injectSteps(r *agentfleet.Runner, steps []Step) {
    w := r.StdinWriter()
    for _, s := range steps {
        select {
        case <-r.Done():
            return
        case <-time.After(time.Duration(s.Delay * float64(time.Second))):
        }
        if s.Command == "" {
            r.Stop()
            return
        }
        fmt.Fprintf(w, "%s\r", s.Command)
    }
}
```

### gRPC streaming case

A `GRPCManager` (user-implemented, not in library) works naturally:

```go
func (m *GRPCManager) Run(ctx context.Context, fleet *agentfleet.Fleet) error {
    stream, _ := m.client.TaskStream(ctx)
    for {
        task, err := stream.Recv()
        if err != nil { return err }
        ag := agentfleet.NewPtyAgent(agentfleet.CommandFields(task), m.cfg.Agent)
        r  := agentfleet.NewRunner(task, ag)
        fleet.Add(r)
        r.Start()
        go func(r *agentfleet.Runner, id string) {
            <-r.Done()
            stream.Send(&Result{ID: id, Status: r.Status().String()})
        }(r, task.ID())
    }
}
```

---

## 5. Error Handling

| Layer | Behavior |
|-------|----------|
| `Manager.Run()` | Returns error on strategy failure (broken stream, API error, bad source). Fleet keeps running already-added runners. |
| Runner failure | Sets `Status = StatusFailed`, closes `Done()`. Does not propagate to Fleet or Manager. |
| `Fleet.Wait()` | Returns first ctx error or nil when all runners complete. Caller inspects `runner.Status()` for individual failures. |
| `AgentConfigFromTerminal()` | Silently falls back to defaults when stdout is not a TTY. No error returned. |
| `source.Load()` | Returns `([]Task, error)`. Manager decides retry/fatal policy. |
| `Fleet.Add()` | Blocks on `MaxConcurrent`; unblocks when a runner slot opens. No error. |

---

## 6. Testing

- **`MockAgent`** in root package — unit tests for Runner, Fleet, example managers without spawning PTYs
- **`source` package** — existing tests carry over unchanged
- **Fleet concurrency** — verify `MaxConcurrent` queuing with `MockAgent` instances
- **Example manager integration** — FileManager against a temp JSON file with `MockAgent` injected via config
- **TUI** — optional; Bubbletea supports rendering to a `strings.Builder` for snapshot tests

---

## 7. Examples

| Example | Source | Demonstrates |
|---------|--------|--------------|
| `examples/file-manager/` | JSON / YAML / Markdown file | `StepTask`, `FileSource`, step injection, Fleet + TUI |
| `examples/http-manager/` | HTTP JSON endpoint | `HTTPSource`, dynamic polling, Fleet + TUI |
| `examples/generate-manager/` | Claude API | `GenerateSource`, LLM-generated tasks, confirm-before-run |
| `examples/attach/` | n/a | `attach` binary — connects to agent Unix socket |
| `examples/taskserver/` | n/a | Example HTTP server that serves tasks (unchanged) |

Each example is a self-contained `main` package. `StepTask` is defined in `examples/file-manager/` (or a shared `examples/internal/steptask/` if multiple examples need it).

---

## 8. Documentation Changes

- **`README.md`** — rewrite to lead with library usage, show import paths, move CLI usage to examples section
- **`CLAUDE.md`** — update package layout table, remove `cmd/` references, update key interfaces section, update build/test commands
