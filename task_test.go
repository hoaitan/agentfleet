package agentfleet_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	agentfleet "github.com/hoaitan/agentfleet"
)

func TestBasicTaskImplementsTask(t *testing.T) {
	var _ agentfleet.Task = &agentfleet.BasicTask{}
}

func TestBasicTaskAccessors(t *testing.T) {
	task := &agentfleet.BasicTask{TaskID: "t-1", TaskName: "Say Hello", Cmd: "claude"}
	assert.Equal(t, "t-1", task.ID())
	assert.Equal(t, "Say Hello", task.Name())
	assert.Equal(t, "claude", task.Command())
}

func TestCommandFields(t *testing.T) {
	task := &agentfleet.BasicTask{TaskID: "t1", TaskName: "T", Cmd: "claude --no-update"}
	fields := agentfleet.CommandFields(task)
	assert.Equal(t, []string{"claude", "--no-update"}, fields)
}

func TestDefaultConfig(t *testing.T) {
	cfg := agentfleet.DefaultConfig()
	assert.Equal(t, 9, cfg.Fleet.MaxConcurrent)
	assert.Equal(t, 200, cfg.Fleet.RingBufferSize)
	assert.Equal(t, "/tmp", cfg.Fleet.SocketDir)
	assert.Equal(t, "/tmp", cfg.Fleet.LogDir)
	assert.Equal(t, 3, cfg.TUI.Columns)
	assert.Equal(t, 500*time.Millisecond, cfg.TUI.RefreshRate)
	assert.Equal(t, 24, cfg.Agent.PTYRows)
	assert.Equal(t, 220, cfg.Agent.PTYCols)
}

func TestAgentConfigFromTerminalFallback(t *testing.T) {
	// In CI / non-TTY, should return defaults without error
	cfg := agentfleet.AgentConfigFromTerminal()
	assert.Greater(t, cfg.PTYRows, 0)
	assert.Greater(t, cfg.PTYCols, 0)
}
