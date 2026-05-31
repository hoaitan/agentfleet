package fleet_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tan/agentfleet/internal/fleet"
)

func TestBasicTaskImplementsTask(t *testing.T) {
	var _ fleet.Task = &fleet.BasicTask{}
}

func TestBasicTaskAccessors(t *testing.T) {
	task := &fleet.BasicTask{
		TaskID:    "t-1",
		TaskName:  "Say Hello",
		Cmd:       "claude",
		TaskSteps: []fleet.Step{{Delay: 2, Command: "hello"}, {Delay: 5, Command: "/exit"}},
	}
	assert.Equal(t, "t-1", task.ID())
	assert.Equal(t, "Say Hello", task.Name())
	assert.Equal(t, "claude", task.Command())
	assert.Len(t, task.Steps(), 2)
	assert.Equal(t, 2.0, task.Steps()[0].Delay)
	assert.Equal(t, "hello", task.Steps()[0].Command)
}
