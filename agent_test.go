package agentfleet_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentfleet "github.com/hoaitan/agentfleet"
)

func TestMockAgentRoundTrip(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	require.NoError(t, ag.Start(24, 80))

	go func() { ag.SimulateOutput([]byte("hello")) }() //nolint:errcheck

	buf := make([]byte, 16)
	n, err := ag.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf[:n]))
}

func TestMockAgentWrite(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	require.NoError(t, ag.Start(24, 80))

	go func() { ag.Write([]byte("input")) }() //nolint:errcheck

	got, err := ag.ReadInput(time.Second)
	require.NoError(t, err)
	assert.Equal(t, []byte("input"), got)
}

func TestMockAgentStop(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	require.NoError(t, ag.Start(24, 80))
	require.NoError(t, ag.Stop())

	select {
	case <-ag.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() channel not closed after Stop()")
	}
}

func TestMockAgentImplementsAgent(t *testing.T) {
	var _ agentfleet.Agent = agentfleet.NewMockAgent()
}
