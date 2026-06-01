# agentfleet

A Go library for running multiple AI agent CLI sessions in parallel with a unified TUI dashboard, automatic step injection (via example managers), session recording, and remote attach capabilities.

## Module

```
github.com/hoaitan/agentfleet
```

## Package Layout

| Package | Responsibility |
|---------|-----------------|
| `github.com/hoaitan/agentfleet` | Core library: `Task`, `Fleet`, `Runner`, `Manager`, `Agent`, `Hook` interfaces + implementations; `Config`, `DefaultConfig`, `AgentConfigFromTerminal` |
| `agentfleet/tui` | Bubbletea TUI dashboard: `tui.Run(ctx, fleet, cfg, onAttach)` — takes `*Fleet` and `TUIConfig` |
| `agentfleet/source` | Task loaders: `FileSource` (JSON/YAML), `MarkdownSource`, `HTTPSource`, `GenerateSource` (Claude API); `StepTask` and `Step` types |
| `agentfleet/hook` | Byte processing pipeline: `Hook` interface, `Dir`/`DirIn`/`DirOut`, `Chain`, `FileLogger`, `Logger` |
| `internal/proxy` | PTY proxying: `Proxy` struct, bidirectional I/O with hook chains, resize signal handling (SIGWINCH) |
| `examples/file-manager` | Example binary: loads tasks from JSON/YAML/Markdown, runs Fleet + TUI with step injection |
| `examples/http-manager` | Example binary: fetches tasks from HTTP endpoint, runs Fleet + TUI with step injection; includes `taskserver/` helper |
| `examples/generate-manager` | Example binary: generates tasks via Claude API, confirm-before-run, Fleet + TUI |

## Key Interfaces

### Task

```go
// Task is the minimum contract for any runnable unit.
type Task interface {
    ID()      string
    Name()    string
    Command() string
}

// BasicTask is the default Task implementation.
type BasicTask struct {
    TaskID   string `json:"id"      yaml:"id"`
    TaskName string `json:"name"    yaml:"name"`
    Cmd      string `json:"command" yaml:"command"`
}
```

Step injection is **not** part of the core `Task` interface. Use `source.StepTask` for tasks that need timed step injection, and call `injectSteps()` in your manager or example code.

### Fleet

```go
type Fleet struct { ... }

func NewFleet(cfg FleetConfig) *Fleet
func (f *Fleet) Add(ctx context.Context, r *Runner) error  // blocks at MaxConcurrent
func (f *Fleet) Runners() []*Runner                         // snapshot for TUI
func (f *Fleet) Wait(ctx context.Context) error             // blocks until all done
```

### Runner

```go
func NewRunner(task Task, ag Agent, cfg FleetConfig) *Runner
func (r *Runner) Start()
func (r *Runner) Stop() error
func (r *Runner) Status() Status
func (r *Runner) Done() <-chan struct{}
func (r *Runner) StdinWriter() io.Writer   // write timed steps here
func (r *Runner) Lines() []string          // ring buffer for TUI preview
func (r *Runner) Task() Task
func (r *Runner) StartedAt() time.Time
func (r *Runner) FinishedAt() time.Time
```

### Manager

```go
// Manager is the orchestration strategy extension point.
type Manager interface {
    Run(ctx context.Context, fleet *Fleet) error
}
```

### Source (in agentfleet/source)

```go
type Source interface {
    Load() ([]agentfleet.Task, error)
}

// StepTask is a Task with timed injection steps — used by all built-in sources.
type StepTask struct {
    TaskID    string `json:"id"      yaml:"id"`
    TaskName  string `json:"name"    yaml:"name"`
    Cmd       string `json:"command" yaml:"command"`
    TaskSteps []Step `json:"steps"   yaml:"steps"`
}

type Step struct {
    Delay   float64 `json:"delay"   yaml:"delay"`
    Command string  `json:"command" yaml:"command"`
}
```

### Hook (in agentfleet/hook)

```go
type Dir uint8
const (DirIn Dir = iota; DirOut)

type Hook interface {
    Process(data []byte, dir Dir) ([]byte, error)
}

type Chain []Hook
func (c Chain) Process(data []byte, dir Dir) ([]byte, error)
```

### Agent

```go
type Agent interface {
    Start(rows, cols int) error
    Write(p []byte) (int, error)
    Read(p []byte) (int, error)
    Resize(rows, cols int) error
    Stop() error
    Done() <-chan struct{}
}
```

Built-in: `PtyAgent` (uses `creack/pty`), `MockAgent` (in-memory, for tests).

## Configuration

```go
cfg := agentfleet.DefaultConfig()
// cfg.Fleet.MaxConcurrent  = 9      (default)
// cfg.Fleet.RingBufferSize = 200    (default)
// cfg.Fleet.SocketDir      = "/tmp" (default; empty = no socket server)
// cfg.Fleet.LogDir         = "/tmp" (default; empty = no log file)
// cfg.TUI.Columns          = 3
// cfg.TUI.RefreshRate      = 500ms
// cfg.Agent.PTYRows        = 24
// cfg.Agent.PTYCols        = 220

cfg.Agent = agentfleet.AgentConfigFromTerminal() // read from actual terminal
```

## Naming Conventions

### Socket Paths

```
/tmp/agentfleet-{task-id}.sock
```

Configured via `FleetConfig.SocketDir`. Runner creates the socket; any Unix socket client can connect to it.

### Log Paths

```
/tmp/agentfleet-{task-id}.log
```

Configured via `FleetConfig.LogDir`. Set to `""` to disable logging.

## Build and Test

```bash
# Build main example binary
go build -o agentfleet ./examples/file-manager/

# Test
go test ./...

# Verbose test output
go test -v ./...

# Test specific packages
go test ./hook/
go test ./source/
go test .

# Run with race detector
go test -race ./...
```

## Task Source Formats

### Markdown

```markdown
## Task: Get today's date
command: claude

- delay: 2, inject: "What is today's date?"
- delay: 8, inject: "/exit"
```

Run: `./agentfleet --source tasks.md`

### JSON

```json
[
  {
    "id": "task-1",
    "name": "Get today's date",
    "command": "claude",
    "steps": [
      {"delay": 2, "command": "What is today's date?"},
      {"delay": 8, "command": "/exit"}
    ]
  }
]
```

Run: `./agentfleet --source tasks.json`

### YAML

```yaml
- id: task-1
  name: Get today's date
  command: claude
  steps:
    - delay: 2
      command: What is today's date?
    - delay: 8
      command: /exit
```

Run: `./agentfleet --source tasks.yaml`

### HTTP

```bash
# Terminal 1
go run ./examples/http-manager/taskserver/
# Terminal 2
go run ./examples/http-manager/ --source http://localhost:8080/tasks
```

### LLM-generated

```bash
ANTHROPIC_API_KEY=sk-... go run ./examples/generate-manager/ --generate "Run 5 coding challenges"
```

## Implementation Notes

- **Runner**: Manages Task + Agent lifecycle. Starts a Unix socket server (if `SocketDir != ""`). No step logic.
- **Fleet**: Dynamic runner registry with semaphore-based `MaxConcurrent` enforcement and `WaitGroup`-based `Wait()`.
- **Step injection**: Example-layer only. Example managers type-assert tasks to `*source.StepTask` and call `go injectSteps(r, st.Steps())`.
- **PtyAgent**: Platform-specific PTY handling (darwin/linux). Uses `creack/pty` and `syscall.SysProcAttr` for session isolation.
- **Proxy**: Sits between agent PTY and socket clients. Multiplexes agent output to attached clients, multiplexes client input to agent stdin, applies hooks.
- **TUI**: Bubbletea app reading from `fleet.Runners()` on each tick. Handles ctx cancellation via `ctxDoneCmd`.

## Common Tasks

### Add a new task source

1. Create a new file in `source/` or your own package
2. Implement `source.Source` interface: `Load() ([]agentfleet.Task, error)`
3. Use in your Manager or example

### Add a new hook

1. Implement `hook.Hook` interface: `Process(data []byte, dir hook.Dir) ([]byte, error)`
2. Add to a `hook.Chain` passed to `proxy.New` — or add to the runner's proxy via a custom Runner setup

### Implement a custom Manager

1. Implement `agentfleet.Manager`: `Run(ctx context.Context, fleet *agentfleet.Fleet) error`
2. Inside `Run`: load/stream tasks → create `Agent` + `Runner` → call `fleet.Add(ctx, r)` → `r.Start()`
3. Call `fleet.Wait(ctx)` at the end to block until all tasks complete

### Log a session

Set `FleetConfig.LogDir` to a directory path (e.g., `cfg.Fleet.LogDir = "/var/log/agentfleet"`). The Runner writes to `{LogDir}/agentfleet-{task-id}.log`. Set to `""` to disable.
