package agentfleet_test

import (
	"os"
	"strings"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentfleet "github.com/hoaitan/agentfleet"
)

func TestPtyAgentEnv(t *testing.T) {
	ag := agentfleet.NewPtyAgent(
		[]string{"sh", "-c", "printenv RETASK_FLEET_TEST_ENV"},
		agentfleet.AgentConfig{
			PTYRows: 24,
			PTYCols: 80,
			Env:     []string{"RETASK_FLEET_TEST_ENV=sentinel_xyz"},
		},
	)
	task := &agentfleet.BasicTask{TaskID: "env-t", TaskName: "env test", Cmd: "sh"}
	cfg := agentfleet.FleetConfig{VTERows: 200}
	r := agentfleet.NewRunner(task, ag, cfg, agentfleet.AgentConfig{
		PTYRows: 24, PTYCols: 80,
		Env: []string{"RETASK_FLEET_TEST_ENV=sentinel_xyz"},
	})
	r.Start()

	select {
	case <-r.Done():
	case <-time.After(5 * time.Second):
		require.Fail(t, "timeout: sh process did not exit")
	}

	output := strings.Join(r.Lines(), "\n")
	assert.Contains(t, output, "sentinel_xyz")
}

// runEnvProbe runs a shell that prints env markers (bracketed so PTY line
// wrapping cannot split a value from its key) and returns the output.
func runEnvProbe(t *testing.T, agentCfg agentfleet.AgentConfig) string {
	t.Helper()
	ag := agentfleet.NewPtyAgent(
		[]string{"/bin/sh", "-c", `echo "HOST=[${AF_HOST_ONLY-unset}] PASSED=[${AF_PASSED-unset}]"`},
		agentCfg,
	)
	task := &agentfleet.BasicTask{TaskID: "env-probe", TaskName: "env probe", Cmd: "/bin/sh"}
	r := agentfleet.NewRunner(task, ag, agentfleet.FleetConfig{VTERows: 200}, agentCfg)
	r.Start()

	select {
	case <-r.Done():
	case <-time.After(5 * time.Second):
		require.Fail(t, "timeout: sh process did not exit")
	}

	return strings.Join(r.Lines(), "\n")
}

func TestPtyAgentReplaceEnv(t *testing.T) {
	t.Setenv("AF_HOST_ONLY", "leak")
	output := runEnvProbe(t, agentfleet.AgentConfig{
		PTYRows:    24,
		PTYCols:    80,
		Env:        []string{"AF_PASSED=ok", "PATH=" + os.Getenv("PATH")},
		ReplaceEnv: true,
	})
	assert.Contains(t, output, "HOST=[unset]")
	assert.Contains(t, output, "PASSED=[ok]")
}

func TestPtyAgentAppendEnvKeepsHostEnv(t *testing.T) {
	t.Setenv("AF_HOST_ONLY", "leak")
	output := runEnvProbe(t, agentfleet.AgentConfig{
		PTYRows: 24,
		PTYCols: 80,
		Env:     []string{"AF_PASSED=ok", "PATH=" + os.Getenv("PATH")},
	})
	assert.Contains(t, output, "HOST=[leak]")
	assert.Contains(t, output, "PASSED=[ok]")
}

func TestPtyAgentReplaceEnvEmpty(t *testing.T) {
	t.Setenv("AF_HOST_ONLY", "leak")
	output := runEnvProbe(t, agentfleet.AgentConfig{
		PTYRows:    24,
		PTYCols:    80,
		ReplaceEnv: true,
	})
	assert.Contains(t, output, "HOST=[unset]")
	assert.Contains(t, output, "PASSED=[unset]")
}
