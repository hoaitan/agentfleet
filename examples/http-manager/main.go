// http-manager demonstrates loading tasks from an HTTP endpoint.
// Run examples/taskserver first: go run ./examples/taskserver/
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	agentfleet "github.com/hoaitan/agentfleet"
	"github.com/hoaitan/agentfleet/source"
	"github.com/hoaitan/agentfleet/tui"
)

var taskIDRe = regexp.MustCompile(`^[a-z0-9\-]+$`)

func main() {
	url := flag.String("source", "", "HTTP endpoint returning JSON task array")
	flag.Parse()
	if *url == "" {
		log.Fatal("--source <url> is required")
	}

	tasks, err := (&source.HTTPSource{URL: *url}).Load()
	if err != nil {
		log.Fatalf("load tasks: %v", err)
	}
	if len(tasks) == 0 {
		log.Fatal("task list is empty")
	}

	cfg := agentfleet.DefaultConfig()
	cfg.Agent = agentfleet.AgentConfigFromTerminal()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fleet := agentfleet.NewFleet(cfg.Fleet)

	for _, task := range tasks {
		if strings.TrimSpace(task.Command()) == "" {
			log.Fatalf("task %q has empty command", task.ID())
		}
		if !taskIDRe.MatchString(task.ID()) {
			log.Fatalf("task %q has invalid ID %q", task.Name(), task.ID())
		}

		ag := agentfleet.NewPtyAgent(agentfleet.CommandFields(task))
		r := agentfleet.NewRunner(task, ag, cfg.Fleet)
		r.Start()

		if err := fleet.Add(ctx, r); err != nil {
			log.Fatalf("add task: %v", err)
		}

		if st, ok := task.(*source.StepTask); ok && len(st.Steps()) > 0 {
			go injectSteps(r, st.Steps())
		}
	}

	if err := tui.Run(ctx, fleet, cfg.TUI, nil); err != nil {
		log.Fatalf("TUI: %v", err)
	}

	for _, r := range fleet.Runners() {
		if r.Status() == agentfleet.StatusRunning {
			r.Stop() //nolint:errcheck
		}
	}
}

func injectSteps(r *agentfleet.Runner, steps []source.Step) {
	w := r.StdinWriter()
	for _, s := range steps {
		select {
		case <-r.Done():
			return
		case <-time.After(time.Duration(s.Delay * float64(time.Second))):
		}
		if s.Command == "" {
			r.Stop() //nolint:errcheck
			return
		}
		fmt.Fprintf(w, "%s\r", s.Command)
	}
}
