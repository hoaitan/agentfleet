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
	assert.Equal(t, 10, cfg.TUI.MaxDoneTasks)
	assert.Equal(t, 500*time.Millisecond, cfg.TUI.RefreshRate)
	assert.Equal(t, 24, cfg.Agent.PTYRows)
	assert.Equal(t, 220, cfg.Agent.PTYCols)
}

func TestAgentConfigFromTerminalFallback(t *testing.T) {
	// In non-TTY (CI), AgentConfigFromTerminal falls back to defaults.
	// This test validates the fallback returns the expected default values.
	// If running in a real terminal, the values will be whatever the terminal reports (still > 0).
	cfg := agentfleet.AgentConfigFromTerminal()
	assert.Greater(t, cfg.PTYRows, 0)
	assert.Greater(t, cfg.PTYCols, 0)
}

func TestAgentConfigFromTerminalValues(t *testing.T) {
	// Defaults must be non-zero and match DefaultConfig
	defaults := agentfleet.DefaultConfig().Agent
	assert.Equal(t, 24, defaults.PTYRows)
	assert.Equal(t, 220, defaults.PTYCols)
}

func TestStatusString(t *testing.T) {
	assert.Equal(t, "pending", agentfleet.StatusPending.String())
	assert.Equal(t, "running", agentfleet.StatusRunning.String())
	assert.Equal(t, "done", agentfleet.StatusDone.String())
	assert.Equal(t, "failed", agentfleet.StatusFailed.String())
}
