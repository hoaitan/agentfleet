package fleet

// Task is the core interface all task definitions must implement.
// Implement this interface to add custom fields — or embed BasicTask for the defaults.
type Task interface {
	ID()      string
	Name()    string
	Command() string // CLI binary to spawn, e.g. "claude" or "codex"
	Steps()   []Step
}

// Step is one timed injection: wait Delay seconds, then send Command to the agent.
// An empty Command string stops the agent.
type Step struct {
	Delay   float64 `json:"delay"   yaml:"delay"`
	Command string  `json:"command" yaml:"command"`
}

// BasicTask is the default Task implementation used by all built-in sources.
// Users can embed it or implement Task from scratch.
type BasicTask struct {
	TaskID    string `json:"id"      yaml:"id"`
	TaskName  string `json:"name"    yaml:"name"`
	Cmd       string `json:"command" yaml:"command"`
	TaskSteps []Step `json:"steps"   yaml:"steps"`
}

func (t *BasicTask) ID()      string { return t.TaskID }
func (t *BasicTask) Name()    string { return t.TaskName }
func (t *BasicTask) Command() string { return t.Cmd }
func (t *BasicTask) Steps()   []Step { return t.TaskSteps }
