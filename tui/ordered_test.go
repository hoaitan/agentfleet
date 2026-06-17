package tui

import (
	"fmt"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentfleet "github.com/hoaitan/agentfleet"
)

func makeTestRunner(id, name string) (*agentfleet.Runner, *agentfleet.MockAgent) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: id, TaskName: name}
	r := agentfleet.NewRunner(task, ag, agentfleet.FleetConfig{VTERows: 10}, agentfleet.AgentConfig{})
	return r, ag
}

func waitDone(t *testing.T, r *agentfleet.Runner) {
	t.Helper()
	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("runner did not finish in time")
	}
}

func TestOrderedRunners_ActiveNewestFirst(t *testing.T) {
	r1, ag1 := makeTestRunner("t1", "task1")
	r2, ag2 := makeTestRunner("t2", "task2")
	r3, _ := makeTestRunner("t3", "task3")

	r1.Start()
	r2.Start()
	r3.Start()
	defer ag2.Stop() //nolint:errcheck
	defer r3.Stop()  //nolint:errcheck

	// r1 finishes → StatusDone
	ag1.Stop() //nolint:errcheck
	waitDone(t, r1)

	// fleet order (oldest→newest): [r1, r2, r3]
	runners := []*agentfleet.Runner{r1, r2, r3}
	active, done := orderedRunners(runners, 10)

	// active newest-first: r3, r2
	require.Len(t, active, 2)
	assert.Equal(t, "t3", active[0].Task().ID())
	assert.Equal(t, "t2", active[1].Task().ID())

	// done: r1
	require.Len(t, done, 1)
	assert.Equal(t, "t1", done[0].Task().ID())
}

func TestOrderedRunners_MaxDoneTasks_keepsNewest(t *testing.T) {
	var runners []*agentfleet.Runner
	for i := 0; i < 5; i++ {
		r, ag := makeTestRunner(fmt.Sprintf("t%d", i), fmt.Sprintf("task%d", i))
		r.Start()
		ag.Stop() //nolint:errcheck
		waitDone(t, r)
		runners = append(runners, r)
	}

	// fleet order: t0(oldest)…t4(newest); all done
	_, done := orderedRunners(runners, 3)

	// keeps 3 newest: t4, t3, t2
	require.Len(t, done, 3)
	assert.Equal(t, "t4", done[0].Task().ID())
	assert.Equal(t, "t3", done[1].Task().ID())
	assert.Equal(t, "t2", done[2].Task().ID())
}

func TestOrderedRunners_MaxDoneZero_noLimit(t *testing.T) {
	var runners []*agentfleet.Runner
	for i := 0; i < 5; i++ {
		r, ag := makeTestRunner(fmt.Sprintf("t%d", i), fmt.Sprintf("task%d", i))
		r.Start()
		ag.Stop() //nolint:errcheck
		waitDone(t, r)
		runners = append(runners, r)
	}
	_, done := orderedRunners(runners, 0)
	assert.Len(t, done, 5)
}

func TestOrderedRunners_Empty(t *testing.T) {
	active, done := orderedRunners(nil, 10)
	assert.Empty(t, active)
	assert.Empty(t, done)
}
