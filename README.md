# agentfleet

*Run your AI agent fleet from a single terminal dashboard*

agentfleet is a Go **library** for running multiple interactive CLI sessions (Claude Code, Codex, or any command) in parallel with a unified Bubbletea TUI dashboard. Each agent runs independently in a PTY with optional step injection, session recording, and remote attach capabilities via Unix sockets.

## Install

Use as a library:

```go
import agentfleet "github.com/hoaitan/agentfleet"
```

Or build the main example binary from source:

```bash
git clone https://github.com/hoaitan/agentfleet
cd agentfleet
go build -o agentfleet ./examples/file-manager/
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

## Examples

| Example | Purpose |
|---------|---------|
| [http-manager](examples/http-manager/) | Load tasks from an HTTP endpoint |
| [file-manager](examples/file-manager/) | Load tasks from JSON, YAML, or Markdown files |
| [generate-manager](examples/generate-manager/) | Generate tasks via Claude API |

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
| `examples/http-manager` | Example: load tasks from HTTP endpoint; includes `taskserver/` sub-directory |
| `examples/file-manager` | Example: load tasks from JSON/YAML/Markdown |
| `examples/generate-manager` | Example: generate tasks with Claude API |

## License

MIT
