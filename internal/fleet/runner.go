package fleet

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hoaitan/agentfleet/internal/agent"
	"github.com/hoaitan/agentfleet/internal/hook"
	"github.com/hoaitan/agentfleet/internal/proxy"
)

// switchWriter is an io.Writer whose target can be swapped at runtime.
type switchWriter struct {
	mu sync.RWMutex
	w  io.Writer
}

func (s *switchWriter) Write(p []byte) (int, error) {
	s.mu.RLock()
	w := s.w
	s.mu.RUnlock()
	return w.Write(p)
}

func (s *switchWriter) set(w io.Writer) {
	s.mu.Lock()
	s.w = w
	s.mu.Unlock()
}

// ringBuffer stores the last max lines of output.
type ringBuffer struct {
	mu    sync.RWMutex
	lines []string
	max   int
}

func newRingBuffer(max int) *ringBuffer {
	return &ringBuffer{max: max, lines: make([]string, 0, max)}
}

func (r *ringBuffer) write(line string) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.lines) >= r.max {
		r.lines = r.lines[1:]
	}
	r.lines = append(r.lines, line)
}

func (r *ringBuffer) snapshot() []string {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]string, len(r.lines))
	copy(out, r.lines)
	return out
}

// ringHook captures output lines into a ringBuffer.
type ringHook struct {
	mu  sync.Mutex
	buf *ringBuffer
	acc []byte
}

func (ri *ringHook) Process(p []byte, dir hook.Dir) ([]byte, error) {
	if dir == hook.DirOut {
		ri.mu.Lock()
		ri.acc = append(ri.acc, p...)
		for {
			idx := bytes.IndexByte(ri.acc, '\n')
			if idx < 0 {
				break
			}
			ri.buf.write(string(ri.acc[:idx]))
			ri.acc = ri.acc[idx+1:]
		}
		ri.mu.Unlock()
	}
	return p, nil
}

// outputTee writes raw output bytes to a file as a side-channel.
type outputTee struct{ f *os.File }

func (t *outputTee) Process(p []byte, dir hook.Dir) ([]byte, error) {
	if dir == hook.DirOut && t.f != nil {
		t.f.Write(p) //nolint:errcheck
	}
	return p, nil
}

// Runner manages one CLI agent session with automatic step injection
// and a ring buffer for TUI output preview.
type Runner struct {
	task Task // read-only after construction

	status atomic.Int32

	mu         sync.RWMutex
	once       sync.Once
	startedAt  time.Time
	finishedAt time.Time
	ag         agent.Agent
	sw         *switchWriter
	ring       *ringBuffer
	done       chan struct{}
	prx        *proxy.Proxy
	pw         *io.PipeWriter
	logFile    *os.File
}

func NewRunner(task Task, ag agent.Agent) *Runner {
	return &Runner{
		task: task,
		ag:   ag,
		sw:   &switchWriter{w: io.Discard},
		ring: newRingBuffer(200),
		done: make(chan struct{}),
	}
}

// Start launches the PTY session and step injector. Non-blocking.
// Safe to call multiple times; only the first call has any effect.
func (r *Runner) Start() {
	r.once.Do(func() {
		pr, pw := io.Pipe()
		r.mu.Lock()
		r.pw = pw
		r.startedAt = time.Now()
		r.mu.Unlock()

		logPath := fmt.Sprintf("/tmp/agentfleet-%s.log", r.task.ID())
		f, _ := os.Create(logPath)
		r.mu.Lock()
		r.logFile = f
		r.mu.Unlock()

		tee := &outputTee{f: f}
		ri := &ringHook{buf: r.ring}
		r.prx = proxy.New(r.ag, pr, r.sw, hook.Chain{}, hook.Chain{tee, ri})
		r.setStatus(StatusRunning)

		go func() {
			if err := r.prx.Run(); err != nil {
				r.setStatus(StatusFailed)
			} else {
				r.setStatus(StatusDone)
			}
			r.mu.Lock()
			r.finishedAt = time.Now()
			if r.logFile != nil {
				r.logFile.Close()
				r.logFile = nil
			}
			r.mu.Unlock()
			close(r.done)
			_ = pw.Close()
		}()

		go r.runSteps()
	})
}

func (r *Runner) Status() Status        { return Status(r.status.Load()) }
func (r *Runner) Done() <-chan struct{} { return r.done }
func (r *Runner) Lines() []string       { return r.ring.snapshot() }
func (r *Runner) Task() Task            { return r.task }
func (r *Runner) setStatus(s Status)    { r.status.Store(int32(s)) }

func (r *Runner) runSteps() {
	for _, step := range r.task.Steps() {
		select {
		case <-r.done:
			return
		case <-time.After(time.Duration(step.Delay * float64(time.Second))):
		}
		if step.Command == "" {
			r.ag.Stop() //nolint:errcheck
			return
		}
		if err := r.prx.Inject([]byte(step.Command + "\r")); err != nil {
			return
		}
	}
}

// SetOutput redirects agent output to w.
func (r *Runner) SetOutput(w io.Writer) { r.sw.set(w) }

// StdinWriter returns a writer whose bytes are forwarded to the agent's stdin.
func (r *Runner) StdinWriter() io.Writer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pw
}

// Stop signals the underlying agent to terminate.
func (r *Runner) Stop() error { return r.ag.Stop() }

func (r *Runner) StartedAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.startedAt
}

func (r *Runner) FinishedAt() time.Time {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.finishedAt
}

// CommandFields splits Task.Command() into argv for NewPtyAgent.
func CommandFields(t Task) []string {
	return strings.Fields(t.Command())
}
