package agentfleet_test

import (
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentfleet "github.com/hoaitan/agentfleet"
)

// testCfg returns a FleetConfig with no socket or log (safe for unit tests).
func testCfg() agentfleet.FleetConfig {
	return agentfleet.FleetConfig{VTERows: 200}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestRunnerStartAndStop(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "t1", TaskName: "Test", Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, testCfg(), agentfleet.AgentConfig{})

	assert.Equal(t, agentfleet.StatusPending, r.Status())
	r.Start()
	assert.Equal(t, agentfleet.StatusRunning, r.Status())

	ag.Stop()
	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("runner did not finish after agent stopped")
	}
	assert.Equal(t, agentfleet.StatusDone, r.Status())
}

func TestRunnerSetOutput(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "t2", TaskName: "Out", Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, testCfg(), agentfleet.AgentConfig{})
	r.Start()

	var mu sync.Mutex
	var captured []byte
	r.SetOutput(writerFunc(func(p []byte) (int, error) {
		mu.Lock()
		captured = append(captured, p...)
		mu.Unlock()
		return len(p), nil
	}))

	require.NoError(t, ag.SimulateOutput([]byte("agent says hi")))
	time.Sleep(50 * time.Millisecond)

	mu.Lock()
	got := string(captured)
	mu.Unlock()
	assert.Contains(t, got, "agent says hi")

	ag.Stop()
	<-r.Done()
}

func TestRunnerStdinWriter(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "t3", TaskName: "Stdin", Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, testCfg(), agentfleet.AgentConfig{})
	r.Start()

	_, err := r.StdinWriter().Write([]byte("hello\r"))
	require.NoError(t, err)

	got, err := ag.ReadInput(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "hello\r", string(got))

	ag.Stop()
	<-r.Done()
}

func TestRunnerStartIdempotent(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "t4", TaskName: "Idempotent", Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, testCfg(), agentfleet.AgentConfig{})

	r.Start()
	r.Start() // second call should be a no-op
	assert.Equal(t, agentfleet.StatusRunning, r.Status())

	ag.Stop()
	<-r.Done()
}

func TestRunnerResize(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "resize-t", TaskName: "resize", Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, testCfg(), agentfleet.AgentConfig{})
	r.Start()

	err := r.Resize(40, 100)
	assert.NoError(t, err)

	ag.Stop()
	<-r.Done()
}
