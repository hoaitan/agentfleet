package proxy_test

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hoaitan/agentfleet/hook"
	"github.com/hoaitan/agentfleet/internal/agent"
	"github.com/hoaitan/agentfleet/internal/proxy"
)

func TestProxyPassThrough(t *testing.T) {
	ag := agent.NewMockAgent()
	pr, pw := io.Pipe()
	out := &collectWriter{}

	p := proxy.New(ag, pr, out, hook.Chain{}, hook.Chain{})

	done := make(chan error, 1)
	go func() { done <- p.Run() }()

	go func() { ag.SimulateOutput([]byte("from agent")) }()
	time.Sleep(50 * time.Millisecond)

	ag.Stop()
	pw.Close()
	<-done

	assert.Equal(t, "from agent", string(out.data))
}

func TestProxyInject(t *testing.T) {
	ag := agent.NewMockAgent()
	pr, pw := io.Pipe()
	out := &collectWriter{}

	p := proxy.New(ag, pr, out, hook.Chain{}, hook.Chain{})
	done := make(chan error, 1)
	go func() { done <- p.Run() }()

	time.Sleep(10 * time.Millisecond) // let Run() start the agent

	readCh := make(chan []byte, 1)
	errCh := make(chan error, 1)
	go func() {
		got, err := ag.ReadInput(time.Second)
		if err != nil {
			errCh <- err
		} else {
			readCh <- got
		}
	}()

	time.Sleep(5 * time.Millisecond) // let ag.ReadInput() start reading
	require.NoError(t, p.Inject([]byte("injected")))

	select {
	case got := <-readCh:
		assert.Equal(t, []byte("injected"), got)
	case err := <-errCh:
		require.NoError(t, err)
	case <-time.After(time.Second):
		t.Fatal("timeout reading input")
	}

	pw.Close()
	ag.Stop()

	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("timeout waiting for Run() to return")
	}
}

type collectWriter struct{ data []byte }

func (c *collectWriter) Write(p []byte) (int, error) {
	c.data = append(c.data, p...)
	return len(p), nil
}
