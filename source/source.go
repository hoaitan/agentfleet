package source

import agentfleet "github.com/hoaitan/agentfleet"

// Source loads a list of tasks from some external system.
type Source interface {
	Load() ([]agentfleet.Task, error)
}

// Step is one timed injection: wait Delay seconds, then send Command.
// An empty Command string stops the agent.
type Step struct {
	Delay   float64 `json:"delay"   yaml:"delay"`
	Command string  `json:"command" yaml:"command"`
}

// StepTask is a Task with timed injection steps.
// It is the serialized format used by all built-in sources (JSON/YAML/Markdown/LLM).
// Example managers type-assert to *StepTask to access Steps().
type StepTask struct {
	TaskID    string `json:"id"      yaml:"id"`
	TaskName  string `json:"name"    yaml:"name"`
	Cmd       string `json:"command" yaml:"command"`
	TaskSteps []Step `json:"steps"   yaml:"steps"`
}

func (t *StepTask) ID() string { return t.TaskID }
func (t *StepTask) Name() string { return t.TaskName }
func (t *StepTask) Command() string { return t.Cmd }
func (t *StepTask) Steps() []Step { return t.TaskSteps }
