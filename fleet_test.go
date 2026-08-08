package agentfleet_test

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentfleet "github.com/hoaitan/agentfleet"
)

func newTestRunner(id string) (*agentfleet.Runner, *agentfleet.MockAgent) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: id, TaskName: id, Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, testCfg(), agentfleet.AgentConfig{})
	return r, ag
}

func TestFleetAddAndRunners(t *testing.T) {
	f := agentfleet.NewFleet(agentfleet.FleetConfig{MaxConcurrent: 3, VTERows: 10})
	ctx := context.Background()

	r1, ag1 := newTestRunner("f1")
	r1.Start()
	require.NoError(t, f.Add(ctx, r1))

	r2, ag2 := newTestRunner("f2")
	r2.Start()
	require.NoError(t, f.Add(ctx, r2))

	runners := f.Runners()
	assert.Len(t, runners, 2)

	ag1.Stop()
	ag2.Stop()
	require.NoError(t, f.Wait(ctx))
}

func TestFleetMaxConcurrent(t *testing.T) {
	f := agentfleet.NewFleet(agentfleet.FleetConfig{MaxConcurrent: 2, VTERows: 10})
	ctx := context.Background()

	r1, ag1 := newTestRunner("c1")
	r1.Start()
	require.NoError(t, f.Add(ctx, r1))

	r2, ag2 := newTestRunner("c2")
	r2.Start()
	require.NoError(t, f.Add(ctx, r2))

	// third Add should block until a slot opens
	added := make(chan struct{})
	r3, ag3 := newTestRunner("c3")
	r3.Start()
	go func() {
		f.Add(ctx, r3) //nolint:errcheck
		close(added)
	}()

	select {
	case <-added:
		t.Fatal("Add() should have blocked")
	case <-time.After(50 * time.Millisecond):
	}

	// free a slot
	ag1.Stop()
	select {
	case <-added:
	case <-time.After(time.Second):
		t.Fatal("Add() did not unblock after slot freed")
	}

	ag2.Stop()
	ag3.Stop()
	require.NoError(t, f.Wait(ctx))
}

func TestFleetWaitCtxCancel(t *testing.T) {
	f := agentfleet.NewFleet(agentfleet.FleetConfig{MaxConcurrent: 9, VTERows: 10})
	r, _ := newTestRunner("w1")
	r.Start()
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, f.Add(ctx, r))

	cancel()
	err := f.Wait(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}

func TestFleetEmptyWait(t *testing.T) {
	f := agentfleet.NewFleet(agentfleet.FleetConfig{MaxConcurrent: 9, VTERows: 10})
	ctx := context.Background()
	// Empty fleet should return immediately
	require.NoError(t, f.Wait(ctx))
}
