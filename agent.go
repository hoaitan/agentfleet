package agentfleet

import (
	"io"
	"sync"
	"time"
)

// Agent is any interactive CLI process running in a PTY.
type Agent interface {
	Start(rows, cols int) error
	Write(p []byte) (int, error)
	Read(p []byte) (int, error)
	Resize(rows, cols int) error
	Stop() error
	Done() <-chan struct{}
}

// MockAgent is an in-memory Agent for tests.
type MockAgent struct {
	inR  *io.PipeReader
	inW  *io.PipeWriter
	outR *io.PipeReader
	outW *io.PipeWriter
	done chan struct{}
	once sync.Once
}

func NewMockAgent() *MockAgent {
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	return &MockAgent{inR: inR, inW: inW, outR: outR, outW: outW, done: make(chan struct{})}
}

func (m *MockAgent) Start(rows, cols int) error { return nil }
func (m *MockAgent) Write(p []byte) (int, error) { return m.inW.Write(p) }
func (m *MockAgent) Read(p []byte) (int, error)  { return m.outR.Read(p) }
func (m *MockAgent) Resize(rows, cols int) error  { return nil }

func (m *MockAgent) Stop() error {
	m.once.Do(func() {
		m.outW.Close()
		m.inR.Close()
		close(m.done)
	})
	return nil
}

func (m *MockAgent) Done() <-chan struct{} { return m.done }

// SimulateOutput writes bytes as if the agent process produced them.
func (m *MockAgent) SimulateOutput(data []byte) error {
	_, err := m.outW.Write(data)
	return err
}

// ReadInput reads bytes that were written via Write, blocking up to timeout.
func (m *MockAgent) ReadInput(timeout time.Duration) ([]byte, error) {
	type result struct {
		data []byte
		err  error
	}
	ch := make(chan result, 1)
	go func() {
		buf := make([]byte, 4096)
		n, err := m.inR.Read(buf)
		if err != nil {
			ch <- result{nil, err}
			return
		}
		ch <- result{buf[:n], nil}
	}()
	select {
	case r := <-ch:
		return r.data, r.err
	case <-time.After(timeout):
		return nil, io.ErrNoProgress
	case <-m.done:
		return nil, io.ErrClosedPipe
	}
}
