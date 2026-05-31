// taskserver is an example HTTP server that serves task definitions to agentfleet.
// Edit the tasks slice to define your own agent workflows.
//
// Usage:
//
//	go run ./examples/taskserver/
//	agentfleet --source http://localhost:8080/tasks
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/hoaitan/agentfleet/internal/fleet"
)

var tasks = []fleet.BasicTask{
	{
		TaskID:   "task-1",
		TaskName: "Ask today's date",
		Cmd:      "claude",
		TaskSteps: []fleet.Step{
			{Delay: 2, Command: "What is today's date?"},
			{Delay: 8, Command: "/exit"},
		},
	},
	{
		TaskID:   "task-2",
		TaskName: "Interactive session",
		Cmd:      "claude",
		TaskSteps: []fleet.Step{
			{Delay: 2, Command: "What is tomorrow's date?"},
		},
	},
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	http.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tasks) //nolint:errcheck
	})

	log.Printf("task server listening on %s — GET /tasks", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
