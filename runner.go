package agentfleet

import (
	"bytes"
	"fmt"
	"io"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"github.com/hoaitan/agentfleet/hook"
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

type outputTee struct{ f *os.File }

func (t *outputTee) Process(p []byte, dir hook.Dir) ([]byte, error) {
	if dir == hook.DirOut && t.f != nil {
		t.f.Write(p) //nolint:errcheck
	}
	return p, nil
}

// Runner manages one CLI agent session: starts the PTY, proxies I/O,
// serves a Unix socket for attach, and maintains an output ring buffer.
// Step injection is not the Runner's concern — callers write to StdinWriter().
type Runner struct {
	task Task
	cfg  FleetConfig

	status atomic.Int32

	mu         sync.RWMutex
	once       sync.Once
	startedAt  time.Time
	finishedAt time.Time
	ag         Agent
	sw         *switchWriter
	ring       *ringBuffer
	done       chan struct{}
	prx        *proxy.Proxy
	pw         *io.PipeWriter
	logFile    *os.File
}

func NewRunner(task Task, ag Agent, cfg FleetConfig) *Runner {
	rbSize := cfg.RingBufferSize
	if rbSize <= 0 {
		rbSize = 200
	}
	return &Runner{
		task: task,
		cfg:  cfg,
		ag:   ag,
		sw:   &switchWriter{w: io.Discard},
		ring: newRingBuffer(rbSize),
		done: make(chan struct{}),
	}
}

// Start launches the PTY session, socket server, and log file. Non-blocking.
// Safe to call multiple times; only the first call has any effect.
func (r *Runner) Start() {
	r.once.Do(func() {
		pr, pw := io.Pipe()
		r.mu.Lock()
		r.pw = pw
		r.startedAt = time.Now()
		r.mu.Unlock()

		tee := &outputTee{}
		if r.cfg.LogDir != "" {
			logPath := filepath.Join(r.cfg.LogDir, "agentfleet-"+r.task.ID()+".log")
			f, _ := os.Create(logPath)
			r.mu.Lock()
			r.logFile = f
			r.mu.Unlock()
			tee = &outputTee{f: f}
		}

		ri := &ringHook{buf: r.ring}
		r.prx = proxy.New(r.ag, pr, r.sw, hook.Chain{}, hook.Chain{tee, ri})
		r.setStatus(StatusRunning)

		if r.cfg.SocketDir != "" {
			r.startSocketServer()
		}

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
	})
}

func (r *Runner) startSocketServer() {
	path := filepath.Join(r.cfg.SocketDir, "agentfleet-"+r.task.ID()+".sock")
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "socket listen %s: %v\n", path, err)
		return
	}

	var (
		connected  atomic.Bool
		activeMu   sync.Mutex
		activeConn net.Conn
	)

	go func() {
		<-r.done
		activeMu.Lock()
		if activeConn != nil {
			activeConn.Close()
		}
		activeMu.Unlock()
		ln.Close()
		os.Remove(path)
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			if !connected.CompareAndSwap(false, true) {
				conn.Write([]byte("already attached\n")) //nolint:errcheck
				conn.Close()
				continue
			}
			activeMu.Lock()
			activeConn = conn
			activeMu.Unlock()

			go func() {
				defer func() {
					activeMu.Lock()
					activeConn = nil
					activeMu.Unlock()
					r.SetOutput(io.Discard)
					connected.Store(false)
					conn.Close()
				}()
				r.SetOutput(conn)
				io.Copy(r.StdinWriter(), conn) //nolint:errcheck
			}()
		}
	}()
}

func (r *Runner) Status() Status       { return Status(r.status.Load()) }
func (r *Runner) Done() <-chan struct{} { return r.done }
func (r *Runner) Lines() []string      { return r.ring.snapshot() }
func (r *Runner) Task() Task           { return r.task }
func (r *Runner) setStatus(s Status)   { r.status.Store(int32(s)) }

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
