# http-manager

Demonstrates loading tasks from an HTTP endpoint. The bundled `taskserver/` sub-directory provides a ready-made task source — edit its `tasks` slice to define your own agent workflows.

## Prerequisites

- Go 1.21+
- `claude` CLI (or substitute any command in `taskserver/main.go`)

## Run

**Terminal 1** — start the task server:

```bash
go run ./examples/http-manager/taskserver/
```

**Terminal 2** — run the fleet:

```bash
go run ./examples/http-manager/ --source http://localhost:8080/tasks
```

## Customize

Edit the `tasks` slice in `taskserver/main.go` to change which commands run and what steps they receive.
