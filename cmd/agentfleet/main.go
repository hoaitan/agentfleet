package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/hoaitan/agentfleet/internal/agent"
	"github.com/hoaitan/agentfleet/internal/fleet"
	"github.com/hoaitan/agentfleet/internal/source"
)

// validTaskID reports whether id is safe for use in file paths and shell strings.
// Accepts only lowercase alphanumeric and hyphens (the same set slugify produces).
var taskIDRe = regexp.MustCompile(`^[a-z0-9\-]+$`)

func validTaskID(id string) bool {
	return taskIDRe.MatchString(id)
}

func main() {
	src := flag.String("source", "", "task source: http URL, .md file, .json/.yaml file")
	generate := flag.String("generate", "", "natural-language goal — calls Claude to generate tasks")
	flag.Parse()

	if *src == "" && *generate == "" {
		log.Fatal("--source <url|file> or --generate <goal> is required")
	}

	tasks, err := loadTasks(*src, *generate)
	if err != nil {
		log.Fatalf("load tasks: %v", err)
	}
	if len(tasks) == 0 {
		log.Fatal("task list is empty")
	}

	if *generate != "" {
		printGeneratedTasks(tasks)
		if !confirm("Launch these tasks? [y/N] ") {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
	}

	runners := make([]*fleet.Runner, len(tasks))
	for i, task := range tasks {
		if strings.TrimSpace(task.Command()) == "" {
			log.Fatalf("task %q has empty command", task.ID())
		}
		if !validTaskID(task.ID()) {
			log.Fatalf("task %q has invalid ID %q (must match [a-z0-9-]+)", task.Name(), task.ID())
		}
		ag := agent.NewPtyAgent(fleet.CommandFields(task))
		runners[i] = fleet.NewRunner(task, ag)
		runners[i].Start()
		go serveTask(runners[i], task.ID())
		openTabFn(task.ID())
	}

	if _, err := tea.NewProgram(newModel(runners), tea.WithAltScreen()).Run(); err != nil {
		log.Fatalf("TUI: %v", err)
	}

	for _, r := range runners {
		if r.Status() == fleet.StatusRunning {
			r.Stop() //nolint:errcheck
		}
	}
}

func loadTasks(src, generate string) ([]fleet.Task, error) {
	if generate != "" {
		return source.NewGenerateSource(generate, "", "").Load()
	}
	switch {
	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		return (&source.HTTPSource{URL: src}).Load()
	case strings.HasSuffix(src, ".md"):
		return (&source.MarkdownSource{Path: src}).Load()
	default:
		return (&source.FileSource{Path: src}).Load()
	}
}

func printGeneratedTasks(tasks []fleet.Task) {
	fmt.Printf("\nGenerated %d task(s):\n\n", len(tasks))
	for _, t := range tasks {
		fmt.Printf("  [%s] %s — command: %s (%d steps)\n", t.ID(), t.Name(), t.Command(), len(t.Steps()))
	}
	fmt.Println()
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}
