# generate-manager

Demonstrates generating tasks from a natural-language goal using the Claude API. Shows the generated tasks for confirmation before running them.

## Prerequisites

- Go 1.21+
- `ANTHROPIC_API_KEY` environment variable set
- `claude` CLI (tasks are generated to run `claude` by default)

## Run

```bash
ANTHROPIC_API_KEY=sk-... go run . --generate "Run 5 coding challenges"
```

The binary prints the generated task list and prompts `Launch these tasks? [y/N]` before starting the fleet.
