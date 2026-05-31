# agentfleet

*Run your AI agent fleet from a single terminal dashboard*

agentfleet is a Go tool for running multiple interactive CLI sessions (Claude Code, Codex, or any command) in parallel with a unified Bubbletea TUI dashboard. Each agent runs independently in a PTY with automatic step injection (timed prompts), session recording, and remote attach capabilities via Unix sockets.

## Install

Download pre-built binaries or build from source:

```bash
go install github.com/hoaitan/agentfleet/cmd/agentfleet@latest
go install github.com/hoaitan/agentfleet/cmd/attach@latest
```

Or build from source:

```bash
git clone https://github.com/hoaitan/agentfleet
cd agentfleet
go build -o agentfleet ./cmd/agentfleet/
go build -o attach ./cmd/attach/
```

## Quick Start

### Tasks from Markdown file

Create `tasks.md`:

```markdown
## Task: Get today's date
command: claude

- delay: 2, inject: "What is today's date?"
- delay: 8, inject: "/exit"

## Task: Interactive session
command: claude

- delay: 3, inject: "Tell me a joke"
```

Launch:

```bash
agentfleet --source tasks.md
```

### Tasks from JSON/YAML file

Create `tasks.json`:

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
  },
  {
    "id": "task-2",
    "name": "Interactive session",
    "command": "claude",
    "steps": [
      {"delay": 3, "command": "Tell me a joke"}
    ]
  }
]
```

Launch with JSON:

```bash
agentfleet --source tasks.json
```

Or with YAML (`tasks.yaml`):

```bash
agentfleet --source tasks.yaml
```

### LLM-generated tasks

Generate tasks using Claude:

```bash
ANTHROPIC_API_KEY=sk-... agentfleet --generate "Run 5 different coding challenges"
```

This calls the Claude API to generate appropriate task definitions based on your goal. You'll be shown the generated tasks and asked to confirm before launch.

## Fleet Dashboard

The dashboard displays all agents in a grid layout:

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

Open a new terminal and attach to a running agent:

```bash
./attach task-1
```

This connects to the agent's Unix socket at `/tmp/agentfleet-task-1.sock`, giving you full interactive control as if you were running the command directly. Your terminal stays in sync with the dashboard — you can type, the output appears in both places, and you can detach cleanly.

## Extending

### Custom Task Type

Implement the `fleet.Task` interface or embed `fleet.BasicTask`:

```go
package mytasks

import "github.com/hoaitan/agentfleet/internal/fleet"

type MyTask struct {
    *fleet.BasicTask
    CustomField string
}

// Optional: add custom methods or override defaults
func (t *MyTask) ID() string {
    return t.TaskID // or compute from CustomField
}
```

Use it in a custom source:

```go
package mysource

import "github.com/hoaitan/agentfleet/internal/source"

type MySource struct {
    Path string
}

func (s *MySource) Load() ([]fleet.Task, error) {
    // Parse path, return []mytasks.MyTask cast to []fleet.Task
}
```

### Custom Hook

Implement the `hook.Hook` interface to process bytes flowing in/out:

```go
package myhooks

import (
    "github.com/hoaitan/agentfleet/internal/hook"
)

type MyHook struct{}

func (h *MyHook) Process(data []byte, dir hook.Dir) ([]byte, error) {
    if dir == hook.DirOut {
        // Transform agent output: redact secrets, annotate, etc.
    }
    return data, nil
}
```

Use it:

```go
chain := hook.Chain{
    &myhooks.MyHook{},
    &hook.FileLogger{Path: "/tmp/session.log"},
}
// Pass chain to Runner or Proxy
```

### Custom Source

Implement the `source.Source` interface to load tasks from anywhere:

```go
package mysource

import "github.com/hoaitan/agentfleet/internal/source"

type DatabaseSource struct {
    URL string
}

func (s *DatabaseSource) Load() ([]fleet.Task, error) {
    // Query database, return []fleet.Task
}
```

Then wire it in `cmd/agentfleet/main.go`:

```go
func loadTasks(src, generate string) ([]fleet.Task, error) {
    if src == "db://..." {
        return (&mysource.DatabaseSource{URL: src}).Load()
    }
    // ... existing logic
}
```

## Architecture

| Package | Responsibility |
|---------|-----------------|
| `cmd/agentfleet` | Main TUI dashboard, task loading, socket server, iTerm2 tab opener |
| `cmd/attach` | Terminal client: connects to agent socket and multiplexes I/O |
| `internal/fleet` | Core task/runner lifecycle: Task interface, BasicTask, Runner state machine |
| `internal/agent` | PTY abstraction: Agent interface, PtyAgent (start/stop/read/write), MockAgent for tests |
| `internal/source` | Task loaders: MarkdownSource, FileSource (JSON/YAML), HTTPSource, GenerateSource (Claude API) |
| `internal/hook` | Byte processing pipeline: Hook interface, Chain, FileLogger |
| `internal/proxy` | PTY proxying: split I/O streams into PTY + socket, handle resize signals |
| `internal` | Imports only, no implementation |

## License

MIT
