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

func TestRunnerResizeResizesVTE(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "rz", TaskName: "Resize", Cmd: "echo"}
	// Start narrow: 10 cols.
	r := agentfleet.NewRunner(task, ag, testCfg(), agentfleet.AgentConfig{PTYCols: 10, PTYRows: 4})
	r.Start()

	require.NoError(t, ag.SimulateOutput([]byte("abcdefghijKLMNO"))) // 15 chars wrap at width 10
	time.Sleep(50 * time.Millisecond)
	lines := r.Lines()
	require.GreaterOrEqual(t, len(lines), 2)
	assert.Equal(t, "abcdefghij", lines[0], "confirms initial width 10")

	// Resize takes (rows, cols); the emulator must widen to 20.
	require.NoError(t, r.Resize(4, 20))
	require.NoError(t, ag.SimulateOutput([]byte("\x1b[2J\x1b[Habcdefghijklmnopqrst"))) // 20 chars
	time.Sleep(50 * time.Millisecond)
	lines = r.Lines()
	require.NotEmpty(t, lines)
	assert.Equal(t, "abcdefghijklmnopqrst", lines[0], "emulator widened to 20 (also proves rows/cols arg order)")
}

// With PTYRows set, the emulator is exactly that tall: a cursor move to row 50
// is clamped to the PTY height (<=6 rows of rendered screen), not the 200-row
// FleetConfig.VTERows.
func TestNewRunnerVTESizedFromPTYRows(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "sz", TaskName: "Size", Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, agentfleet.FleetConfig{VTERows: 200}, agentfleet.AgentConfig{PTYCols: 12, PTYRows: 6})
	r.Start()

	require.NoError(t, ag.SimulateOutput([]byte("\x1b[50;1Hmark"))) // row 50 on a 6-row screen
	time.Sleep(50 * time.Millisecond)
	lines := r.Lines()
	assert.LessOrEqual(t, len(lines), 6, "cursor clamped to the 6-row PTY height, not 200")
}

// With PTYRows unset (<=0), fall back to FleetConfig.VTERows so existing callers
// that rely on a tall preview emulator keep working.
func TestNewRunnerVTEFallsBackToVTERows(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "fb", TaskName: "Fallback", Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, agentfleet.FleetConfig{VTERows: 200}, agentfleet.AgentConfig{PTYCols: 12, PTYRows: 0})
	r.Start()

	require.NoError(t, ag.SimulateOutput([]byte("\x1b[50;1Hmark"))) // row 50 valid in a 200-row screen
	time.Sleep(50 * time.Millisecond)
	lines := r.Lines()
	require.Len(t, lines, 50, "200-row emulator keeps row 50")
	assert.Equal(t, "mark", lines[49])
}
