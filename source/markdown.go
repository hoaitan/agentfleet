package source

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	agentfleet "github.com/hoaitan/agentfleet"
)

// MarkdownSource loads tasks from a Markdown file.
//
// Format:
//
//	## Task: <name>
//	command: <cli>
//
//	- delay: <N>, inject: "<text>"
type MarkdownSource struct {
	Path string
}

func (s *MarkdownSource) Load() ([]agentfleet.Task, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", s.Path, err)
	}
	defer f.Close()

	var tasks []*StepTask
	var current *StepTask

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "## Task: ") {
			if current != nil {
				tasks = append(tasks, current)
			}
			name := strings.TrimPrefix(line, "## Task: ")
			current = &StepTask{TaskID: slugify(name), TaskName: name}
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "command: ") {
			current.Cmd = strings.TrimPrefix(line, "command: ")
			continue
		}

		if strings.HasPrefix(line, "- delay: ") {
			if step, err := parseMarkdownStep(line); err == nil {
				current.TaskSteps = append(current.TaskSteps, step)
			}
		}
	}

	if current != nil {
		tasks = append(tasks, current)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	out := make([]agentfleet.Task, len(tasks))
	for i, t := range tasks {
		out[i] = t
	}
	return out, nil
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func parseMarkdownStep(line string) (Step, error) {
	line = strings.TrimPrefix(line, "- ")
	parts := strings.SplitN(line, ", inject: ", 2)
	if len(parts) != 2 {
		return Step{}, fmt.Errorf("invalid step: %q", line)
	}
	delayStr := strings.TrimPrefix(parts[0], "delay: ")
	delay, err := strconv.ParseFloat(delayStr, 64)
	if err != nil {
		return Step{}, fmt.Errorf("invalid delay: %q", delayStr)
	}
	return Step{Delay: delay, Command: strings.Trim(parts[1], `"`)}, nil
}
