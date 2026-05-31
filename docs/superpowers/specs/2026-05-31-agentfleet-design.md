# agentfleet — Design Spec

**Date:** 2026-05-31
**Status:** Approved

---

## Goal

Open-source a Go tool that lets developers run a fleet of AI agent CLI sessions (Claude Code, Codex, or any interactive CLI) from a single terminal dashboard, with automatic task injection and flexible task sources.

**Tagline:** *"Run your AI agent fleet from a single terminal dashboard"*

**Repo:** `github.com/you/agentfleet`

---

## Target audience

Developers running AI agents at scale on a VM or server — people who want to drive multiple `claude`, `codex`, or similar sessions in parallel, inject prompts programmatically, and monitor output from one place.

---

## Architecture

```
agentfleet/
├── cmd/
│   ├── agentfleet/          # main binary — fleet dashboard TUI
│   │   ├── main.go
│   │   ├── tui.go           # Bubbletea fleet dashboard
│   │   └── socketserver.go  # per-task Unix socket for attach
│   └── attach/              # connect stdin/stdout to a running agent slot
│       └── main.go
├── internal/
│   ├── agent/               # PtyAgent — wraps any CLI in a PTY
│   ├── proxy/               # transparent PTY proxy with inject + hooks
│   ├── hook/                # middleware chain (Hook interface, Chain, Dir)
│   ├── fleet/               # Runner, Task interface, BasicTask, Step, Status
│   └── source/              # task source adapters
│       ├── source.go        # Source interface
│       ├── http.go          # HTTPSource
│       ├── markdown.go      # MarkdownSource
│       ├── file.go          # FileSource (JSON/YAML)
│       └── generate.go      # GenerateSource (LLM-generated tasks)
├── examples/
│   └── taskserver/          # example HTTP task server
├── README.md
└── CLAUDE.md
```

---

## Core interfaces

### fleet.Task (extensible)

```go
type Task interface {
    ID()      string
    Name()    string
    Command() string  // CLI to spawn, e.g. "claude", "codex"
    Steps()   []Step  // timed injection sequence
}

// BasicTask is the default concrete implementation.
// Users can embed it or implement Task from scratch to add custom fields.
type BasicTask struct {
    TaskID    string `json:"id"`
    TaskName  string `json:"name"`
    Cmd       string `json:"command"`
    TaskSteps []Step `json:"steps"`
}

type Step struct {
    Delay   float64 `json:"delay"`   // seconds to wait before injecting
    Command string  `json:"command"` // text to inject; empty = stop agent
}
```

### hook.Hook (replaces interceptor)

```go
type Dir int
const (DirIn Dir = iota; DirOut)

type Hook interface {
    Process(data []byte, dir Dir) ([]byte, error)
}

type HookFunc func([]byte, Dir) ([]byte, error)  // inline adapter

type Chain []Hook  // processes hooks in order; fail-open
```

### source.Source

```go
type Source interface {
    Load() ([]fleet.Task, error)
}
```

### fleet.Runner (replaces TaskRunner)

```go
type Runner struct { ... }

func NewRunner(task Task, ag agent.Agent) *Runner
func (r *Runner) Start()
func (r *Runner) Stop() error
func (r *Runner) SetOutput(w io.Writer)
func (r *Runner) StdinWriter() io.Writer
func (r *Runner) Done() <-chan struct{}
func (r *Runner) Lines() []string       // last N lines for TUI preview
func (r *Runner) Status() Status
func (r *Runner) StartedAt() time.Time
func (r *Runner) FinishedAt() time.Time
```

---

## Task sources

The `agentfleet` binary selects a source via flags:

```bash
agentfleet --source http://localhost:8080/tasks   # HTTP JSON
agentfleet --source tasks.md                      # Markdown file
agentfleet --source tasks.yaml                    # YAML/JSON file
agentfleet --generate "summarize HN top 5"        # LLM-generated
```

### Markdown format

```markdown
## Task: Summarize HN
command: claude

- delay: 2, inject: "Go to https://news.ycombinator.com and summarize top 5"
- delay: 30, inject: "/exit"

## Task: Write tests
command: claude

- delay: 2, inject: "Write unit tests for internal/fleet/runner.go"
- delay: 60, inject: "/exit"
```

### HTTP JSON format

```json
[
  {
    "id": "task-1",
    "name": "Summarize HN",
    "command": "claude",
    "steps": [
      {"delay": 2, "command": "Summarize HN top 5"},
      {"delay": 30, "command": "/exit"}
    ]
  }
]
```

### LLM generation (`--generate`)

- Calls Claude API with the `fleet.Task` JSON schema as context
- System prompt instructs it to return a valid JSON task list
- Generated tasks are printed as JSON to stdout before the TUI starts
- User sees a `Launch these tasks? [y/N]` prompt; TUI only opens on confirmation

---

## Fleet dashboard (TUI)

Built with Bubbletea + Lipgloss. Shows all running agents as cards:

```
◈ agentfleet  3 tasks · 2 running · 1 done

▶ task-1  Summarize HN                           ● running
  01:23 elapsed · 2 steps

  Human: Summarize HN top 5
  Assistant: Here are the top 5 stories...
  ▌

  task-2  Write tests                            ✓ done
  task-3  Review PR                              ● running
```

Keys: `↑↓ / j/k` navigate, `enter` open agent in new iTerm2 tab, `q` quit.

Each task gets a Unix socket at `/tmp/agentfleet-<id>.sock`. The `attach` binary connects to it, piping local stdin/stdout to the live agent session.

---

## Package rename mapping

| Old | New |
|-----|-----|
| `internal/interceptor` | `internal/hook` |
| `internal/manager` | `internal/fleet` |
| `manager.TaskRunner` | `fleet.Runner` |
| `manager.TaskDef` | `fleet.BasicTask` + `fleet.Task` (interface) |
| `manager.Step` | `fleet.Step` |
| `interceptor.Interceptor` | `hook.Hook` |
| `interceptor.Chain` | `hook.Chain` |
| `interceptor.Direction` | `hook.Dir` |
| `cmd/managerv2` | `cmd/agentfleet` |
| `cmd/bypass-attach` | `cmd/attach` |
| `cmd/taskserver` | `examples/taskserver` |

---

## README structure

1. What it is (1 paragraph + GIF/screenshot)
2. Install / quick start
3. Task sources (HTTP, Markdown, YAML, `--generate`)
4. Fleet dashboard key bindings
5. Extending: implementing `Task`, writing custom `Hook`s, building a custom `Source`
6. Architecture diagram

## CLAUDE.md

Documents the project goal, package layout, naming conventions, and key interfaces for future Claude Code sessions working in this repo.

---

## Out of scope (v1)

- Non-iTerm2 terminal tab opening (macOS only for now)
- Web UI
- Remote agent execution (SSH)
- Agent output parsing / done-detection
