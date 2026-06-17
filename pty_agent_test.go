package agentfleet_test

import (
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
