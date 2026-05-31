// generate-manager calls the Claude API to generate tasks from a natural-language goal,
// shows the generated tasks for confirmation, then runs them with the TUI.
//
// Usage:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/generate-manager/ --generate "Run 3 coding challenges"
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
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
	goal := flag.String("generate", "", "natural-language goal — calls Claude to generate tasks")
	flag.Parse()
	if *goal == "" {
		log.Fatal("--generate <goal> is required")
	}

	tasks, err := source.NewGenerateSource(*goal, "", "").Load()
	if err != nil {
		log.Fatalf("generate tasks: %v", err)
	}
	if len(tasks) == 0 {
		log.Fatal("no tasks generated")
	}

	fmt.Printf("\nGenerated %d task(s):\n\n", len(tasks))
	for _, t := range tasks {
		steps := 0
		if st, ok := t.(*source.StepTask); ok {
			steps = len(st.Steps())
		}
		fmt.Printf("  [%s] %s — command: %s (%d steps)\n", t.ID(), t.Name(), t.Command(), steps)
	}
	fmt.Println()

	if !confirm("Launch these tasks? [y/N] ") {
		fmt.Println("Aborted.")
		os.Exit(0)
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

		ag := agentfleet.NewPtyAgent(agentfleet.CommandFields(task), cfg.Agent)
		r := agentfleet.NewRunner(task, ag, cfg.Fleet, cfg.Agent)
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

func confirm(prompt string) bool {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}
