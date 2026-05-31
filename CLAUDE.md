# agentfleet

A Go tool for running multiple AI agent CLI sessions in parallel with a unified TUI dashboard, automatic step injection, session recording, and remote attach capabilities.

## Module

```
github.com/tan/agentfleet
```

## Package Layout

| Package | Responsibility |
|---------|-----------------|
| `cmd/agentfleet` | Main TUI program: Bubbletea dashboard, task loader dispatch, Unix socket server, iTerm2 tab auto-opener, model state |
| `cmd/attach` | Terminal client: connects to `/tmp/agentfleet-<task-id>.sock`, bidirectional I/O multiplexing, handles raw terminal mode |
| `internal/fleet` | Core abstractions: Task interface, BasicTask struct, Runner state machine (Pending → Running → Done/Failed), RingBuffer for output history |
| `internal/agent` | PTY layer: Agent interface, PtyAgent (wraps github.com/creack/pty), MockAgent for testing, Start/Stop/Read/Write/Resize/Done semantics |
| `internal/source` | Task loaders: MarkdownSource (## Task: format), FileSource (JSON/YAML), HTTPSource (JSON endpoint), GenerateSource (Claude API via Anthropic SDK) |
| `internal/hook` | Byte processing: Hook interface, Chain (ordered sequence), FileLogger implementation, DirIn/DirOut constants for byte flow direction |
| `internal/proxy` | PTY proxying: Proxy struct, socket server setup, bidirectional I/O with optional hooks, resize signal handling (SIGWINCH), keepalive |

## Key Interfaces

### Task

All task definitions implement this interface:

```go
// Task is the core interface all task definitions must implement.
type Task interface {
    ID()      string   // Unique task identifier
    Name()    string   // Human-readable task name
    Command() string   // CLI binary to spawn, e.g. "claude" or "codex"
    Steps()   []Step   // Slice of timed injections
}
```

**BasicTask** is the default implementation:

```go
// BasicTask is used by all built-in sources
type BasicTask struct {
    TaskID    string `json:"id"      yaml:"id"`
    TaskName  string `json:"name"    yaml:"name"`
    Cmd       string `json:"command" yaml:"command"`
    TaskSteps []Step `json:"steps"   yaml:"steps"`
}

// Step is one timed injection: wait Delay seconds, then send Command
type Step struct {
    Delay   float64 `json:"delay"   yaml:"delay"`
    Command string  `json:"command" yaml:"command"` // empty = stop agent
}
```

### Source

All task loaders implement this interface:

```go
// Source loads a list of tasks from some external system
type Source interface {
    Load() ([]fleet.Task, error)
}
```

Built-in implementations:
- **MarkdownSource** — parses `## Task: name` + `command: <cli>` + `- delay: N, inject: "text"` format
- **FileSource** — auto-detects JSON or YAML via file extension
- **HTTPSource** — fetches JSON array from HTTP endpoint
- **GenerateSource** — calls Claude API to generate tasks from natural-language goal

### Hook

All byte processors implement this interface:

```go
// Direction constants for byte flow
type Dir uint8
const (
    DirIn  Dir = iota // user / wrapper → agent
    DirOut            // agent → display
)

// Hook processes a byte slice in a given direction
// Returning nil bytes suppresses the data
// Returning error causes the chain to skip this hook (fail-open)
type Hook interface {
    Process(data []byte, dir Dir) ([]byte, error)
}

// Chain applies hooks in sequence
type Chain []Hook
func (c Chain) Process(data []byte, dir Dir) ([]byte, error)
```

Built-in implementation:
- **FileLogger** — logs bytes to a file, one direction per line

### Agent

All PTY processes implement this interface:

```go
// Agent is any interactive CLI process running in a PTY
type Agent interface {
    Start(rows, cols int) error
    Write(p []byte) (int, error) // send to stdin
    Read(p []byte) (int, error)   // read from stdout
    Resize(rows, cols int) error
    Stop() error
    Done() <-chan struct{} // closed when process exits
}
```

Built-in implementations:
- **PtyAgent** — uses github.com/creack/pty, platform-specific (darwin/linux)
- **MockAgent** — in-memory, for tests

## Naming Conventions

### Socket Paths

Per-agent Unix socket for `attach`:

```
/tmp/agentfleet-{task-id}.sock
```

The socket server is created by the Runner's proxy and listened on by the TUI main loop.

### Log Paths

Session recording (if enabled via FileLogger hook):

```
/tmp/agentfleet-{task-id}.log
```

Or custom path via FileLogger configuration.

### Package Names

- Internal packages: `internal/{pkg}` — not importable by external code
- Public: none — this is a tool, not a library. Consumers extend via custom Source/Hook/Task implementations in their own code.

## Build and Test

```bash
# Build
go build -o agentfleet ./cmd/agentfleet/
go build -o attach ./cmd/attach/

# Test
go test ./...

# Verbose test output
go test -v ./...

# Test a specific package
go test ./internal/fleet/
go test ./internal/source/

# Run with race detector
go test -race ./...
```

## Task Source Formats

### Markdown

File: `tasks.md`

```markdown
## Task: Get today's date
command: claude

- delay: 2, inject: "What is today's date?"
- delay: 8, inject: "/exit"

## Task: Another task
command: codex

- delay: 3, inject: "Write a function"
```

Launch: `agentfleet --source tasks.md`

### JSON

File: `tasks.json`

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

Launch: `agentfleet --source tasks.json`

### YAML

File: `tasks.yaml`

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

Launch: `agentfleet --source tasks.yaml`

### HTTP

JSON endpoint returning array of tasks (same structure as JSON file):

```bash
agentfleet --source http://localhost:8080/tasks
```

Example server: `examples/taskserver/main.go`

### LLM-generated

Calls Claude API to generate tasks from natural-language goal:

```bash
ANTHROPIC_API_KEY=sk-... agentfleet --generate "Run 5 coding challenges"
```

Requires `ANTHROPIC_API_KEY` environment variable. GenerateSource parses the response and creates BasicTask objects.

## Implementation Notes

- **Runner**: State machine that manages Task + Agent lifecycle. Injects steps at specified delays using a time.Ticker.
- **PtyAgent**: Platform-specific PTY handling (darwin/linux). Uses syscall.SetWinsize for SIGWINCH resize signals.
- **Proxy**: Sits between agent PTY and socket clients. Multiplexes agent output to all attached clients, multiplexes client input to agent stdin, applies hooks.
- **TUI**: Bubbletea app with responsive grid layout. Updates RingBuffer with agent output for live preview. Handles arrow keys and Enter for attach.
- **Attach**: Simple TCP client on Unix socket. Reads from stdin, writes to socket; reads from socket, writes to stdout. Raw terminal mode via golang.org/x/term.

## Common Tasks

### Add a new task source

1. Create new file in `internal/source/`
2. Implement `Source` interface: `Load() ([]fleet.Task, error)`
3. In `cmd/agentfleet/main.go`, add case to `loadTasks()` function
4. Write tests in `internal/source/*_test.go`

### Add a new hook

1. Create new file in `internal/hook/` or embed in your application
2. Implement `Hook` interface: `Process(data []byte, dir Dir) ([]byte, error)`
3. Add to hook.Chain in main or Runner setup
4. Test with MockAgent

### Add a custom task type

1. Create struct, implement `Task` interface or embed `BasicTask`
2. Create custom Source that parses and returns your type (cast to `[]fleet.Task`)
3. No code changes in agentfleet needed — completely external

### Log a session

Use `FileLogger` hook:

```go
runner.AddHook(&hook.FileLogger{Path: "/tmp/task-" + task.ID() + ".log"})
```

Or create custom hook that writes to any destination.
