# file-manager

Demonstrates loading tasks from a local file. Supports Markdown, JSON, and YAML formats.

## Prerequisites

- Go 1.21+
- `claude` CLI (or substitute any command in your task file)

## Build

```bash
go build -o agentfleet ./examples/file-manager/
```

## Run

**Markdown** — create `tasks.md`:

```markdown
## Task: Get today's date
command: claude

- delay: 2, inject: "What is today's date?"
- delay: 8, inject: "/exit"
```

```bash
./agentfleet --source tasks.md
```

**JSON:**

```bash
./agentfleet --source tasks.json
```

**YAML:**

```bash
./agentfleet --source tasks.yaml
```

You can also use `go run` without building:

```bash
go run ./examples/file-manager/ --source tasks.md
```

## Task File Formats

See the root [README](../../README.md) for full JSON and YAML schema examples.
