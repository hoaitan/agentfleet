# agentfleet

*Run your AI agent fleet from a single terminal dashboard*

agentfleet is a Go **library** for running multiple interactive CLI sessions (Claude Code, Codex, or any command) in parallel with a unified Bubbletea TUI dashboard. Each agent runs independently in a PTY with optional step injection, session recording, and remote attach capabilities via Unix sockets.

## Install

Use as a library:

```go
import agentfleet "github.com/hoaitan/agentfleet"
```

Or build the example binaries from source:

```bash
git clone https://github.com/hoaitan/agentfleet
cd agentfleet
go build -o agentfleet ./examples/file-manager/
go build -o attach ./examples/attach/
```

## Library Quick Start

```go
package main

import (
    "context"
    "os/signal"
    "syscall"

    agentfleet "github.com/hoaitan/agentfleet"
    "github.com/hoaitan/agentfleet/tui"
)

func main() {
    cfg := agentfleet.DefaultConfig()
    cfg.Agent = agentfleet.AgentConfigFromTerminal()

    ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
    defer stop()

    fleet := agentfleet.NewFleet(cfg.Fleet)

    // Implement Manager to drive tasks into the Fleet:
    // mgr := &MyManager{...}
    // go mgr.Run(ctx, fleet)

    tui.Run(ctx, fleet, cfg.TUI, nil)
}
```

## Example Binaries

### Tasks from Markdown file

Create `tasks.md`:

```markdown
## Task: Get today's date
command: claude

- delay: 2, inject: "What is today's date?"
- delay: 8, inject: "/exit"
```

Run:

```bash
./agentfleet --source tasks.md
```

### Tasks from JSON/YAML

```bash
./agentfleet --source tasks.json
./agentfleet --source tasks.yaml
```

### Tasks from HTTP

```bash
go run ./examples/taskserver/ &
./agentfleet --source http://localhost:8080/tasks
```

### LLM-generated tasks

```bash
ANTHROPIC_API_KEY=sk-... go run ./examples/generate-manager/ --generate "Run 5 coding challenges"
```

## Fleet Dashboard

```
┌─────────────────────────────────────────────────────────────────┐
│ agentfleet — 4 agents running                                   │
├─────────────────────────────────────────────────────────────────┤
│ ┌──────────────────┐  ┌──────────────────┐                      │
│ │ task-1: Running  │  │ task-2: Running  │                      │
│ │ claude           │  │ claude           │                      │
│ │                  │  │                  │                      │
│ │ $ What is today  │  │ $ Tell me a joke │                      │
│ │ > December 19... │  │ > Why did the... │                      │
│ └──────────────────┘  └──────────────────┘                      │
│ ┌──────────────────┐  ┌──────────────────┐                      │
│ │ task-3: Done     │  │ task-4: Failed   │                      │
│ │ codex            │  │ vim              │                      │
│ │                  │  │                  │                      │
│ └──────────────────┘  └──────────────────┘                      │
├─────────────────────────────────────────────────────────────────┤
│ (j/k) scroll | (↵) attach | (q) quit                            │
└─────────────────────────────────────────────────────────────────┘
```

### Key Bindings

| Key      | Action                                       |
|----------|----------------------------------------------|
| `j` / `k` | Navigate between agents (scroll down/up)   |
| `↵` Enter | Attach to selected agent's terminal        |
| `q`       | Quit (stops all running agents)            |

## Attaching to a Session

Build the attach binary:

```bash
go build -o attach ./examples/attach/
```

Open a new terminal and attach to a running agent:

```bash
./attach task-1
```

This connects to the agent's Unix socket at `/tmp/agentfleet-task-1.sock`, giving you full interactive control. Your terminal stays in sync with the dashboard.

## Extending

### Custom Manager

Implement `Manager` to control how tasks are loaded and when they run:

```go
package mymgr

import (
    "context"
    agentfleet "github.com/hoaitan/agentfleet"
)

type GRPCManager struct{ client pb.TaskClient }

func (m *GRPCManager) Run(ctx context.Context, fleet *agentfleet.Fleet) error {
    stream, _ := m.client.TaskStream(ctx)
    for {
        task, err := stream.Recv()
        if err != nil { return err }
        ag := agentfleet.NewPtyAgent(agentfleet.CommandFields(task))
        r  := agentfleet.NewRunner(task, ag, agentfleet.DefaultConfig().Fleet)
        fleet.Add(ctx, r)
        r.Start()
        go func(r *agentfleet.Runner) {
            <-r.Done()
            stream.Send(&pb.Result{Id: r.Task().ID(), Status: r.Status().String()})
        }(r)
    }
}
```

### Custom Hook

```go
package myhooks

import "github.com/hoaitan/agentfleet/hook"

type MyHook struct{}

func (h *MyHook) Process(data []byte, dir hook.Dir) ([]byte, error) {
    if dir == hook.DirOut {
        // Transform agent output: redact secrets, annotate, etc.
    }
    return data, nil
}
```

### Custom Task

```go
package mytasks

import agentfleet "github.com/hoaitan/agentfleet"

type MyTask struct {
    agentfleet.BasicTask
    CustomField string
}
```

### Custom Source

```go
package mysource

import (
    agentfleet "github.com/hoaitan/agentfleet"
    "github.com/hoaitan/agentfleet/source"
)

type DatabaseSource struct{ URL string }

func (s *DatabaseSource) Load() ([]agentfleet.Task, error) {
    // Query database, return []agentfleet.Task
}
```

## Architecture

| Package | Responsibility |
|---------|-----------------|
| `github.com/hoaitan/agentfleet` | Core: `Task`, `Fleet`, `Runner`, `Manager`, `Agent`, `Config` |
| `agentfleet/tui` | Bubbletea TUI dashboard — `tui.Run(ctx, fleet, cfg, onAttach)` |
| `agentfleet/source` | Task loaders: `FileSource`, `MarkdownSource`, `HTTPSource`, `GenerateSource`, `StepTask` |
| `agentfleet/hook` | Byte processing: `Hook`, `Chain`, `FileLogger`, `Logger` |
| `examples/file-manager` | Example: load tasks from JSON/YAML/Markdown |
| `examples/http-manager` | Example: load tasks from HTTP endpoint |
| `examples/generate-manager` | Example: generate tasks with Claude API |
| `examples/attach` | Terminal client: attach to a running agent's socket |
| `examples/taskserver` | Example HTTP server serving task definitions |

## License

MIT
