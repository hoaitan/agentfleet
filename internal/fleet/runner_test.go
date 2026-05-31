package fleet_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/agent"
	"github.com/tan/agentfleet/internal/fleet"
)

func TestRunnerStartAndStop(t *testing.T) {
	ag := agent.NewMockAgent()
	task := &fleet.BasicTask{TaskID: "t1", TaskName: "Test", Cmd: "echo", TaskSteps: nil}
	r := fleet.NewRunner(task, ag)

	assert.Equal(t, fleet.StatusPending, r.Status())
	r.Start()
	assert.Equal(t, fleet.StatusRunning, r.Status())

	ag.Stop()
	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("runner did not finish after agent stopped")
	}
	assert.Equal(t, fleet.StatusDone, r.Status())
}

func TestRunnerSetOutput(t *testing.T) {
	ag := agent.NewMockAgent()
	task := &fleet.BasicTask{TaskID: "t2", TaskName: "Out", Cmd: "echo"}
	r := fleet.NewRunner(task, ag)
	r.Start()

	var buf bytes.Buffer
	r.SetOutput(&buf)
	ag.SimulateOutput([]byte("agent says hi")) //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	assert.Contains(t, buf.String(), "agent says hi")

	ag.Stop()
	<-r.Done()
}

func TestRunnerStepInjection(t *testing.T) {
	ag := agent.NewMockAgent()
	task := &fleet.BasicTask{
		TaskID:   "t3",
		TaskName: "Steps",
		Cmd:      "echo",
		TaskSteps: []fleet.Step{
			{Delay: 0.05, Command: "step1"},
		},
	}
	r := fleet.NewRunner(task, ag)
	r.Start()

	got, err := ag.ReadInput(2 * time.Second)
	require.NoError(t, err)
	assert.Equal(t, "step1\r", string(got))

	ag.Stop()
	<-r.Done()
}
