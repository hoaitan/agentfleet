package agentfleet

import "strings"

// Task is the minimum contract for any runnable unit.
type Task interface {
	ID() string
	Name() string
	Command() string
}

// BasicTask is the default Task implementation.
type BasicTask struct {
	TaskID   string `json:"id"      yaml:"id"`
	TaskName string `json:"name"    yaml:"name"`
	Cmd      string `json:"command" yaml:"command"`
}

func (t *BasicTask) ID() string      { return t.TaskID }
func (t *BasicTask) Name() string    { return t.TaskName }
func (t *BasicTask) Command() string { return t.Cmd }

// CommandFields splits Task.Command() into argv for NewPtyAgent.
func CommandFields(t Task) []string { return strings.Fields(t.Command()) }
