package agentfleet

import (
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

// fanoutWriter writes to a primary writer (set via SetOutput) plus any number
// of secondary writers (socket attach clients). Secondary writers are best-effort;
// errors are silently ignored so a slow or disconnected client never blocks PTY output.
type fanoutWriter struct {
	mu        sync.RWMutex
	primary   io.Writer
	secondary []io.Writer
}

func (f *fanoutWriter) Write(p []byte) (int, error) {
	f.mu.RLock()
	primary := f.primary
	secondary := f.secondary
	f.mu.RUnlock()
	n, err := primary.Write(p)
	for _, w := range secondary {
		w.Write(p) //nolint:errcheck
	}
	return n, err
}

func (f *fanoutWriter) setPrimary(w io.Writer) {
	f.mu.Lock()
	f.primary = w
	f.mu.Unlock()
}

func (f *fanoutWriter) addSecondary(w io.Writer) {
	f.mu.Lock()
	f.secondary = append(f.secondary, w)
	f.mu.Unlock()
}

func (f *fanoutWriter) removeSecondary(w io.Writer) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, sw := range f.secondary {
		if sw == w {
			f.secondary = append(f.secondary[:i], f.secondary[i+1:]...)
			return
		}
	}
}

type outputTee struct{ f *os.File }

func (t *outputTee) Process(p []byte, dir hook.Dir) ([]byte, error) {
	if dir == hook.DirOut && t.f != nil {
		t.f.Write(p) //nolint:errcheck
	}
	return p, nil
}

// Runner manages one CLI agent session: starts the PTY, proxies I/O,
// serves a Unix socket for attach, and maintains a virtual terminal screen
// for preview. Step injection is not the Runner's concern — callers write to StdinWriter().
type Runner struct {
	task     Task
	cfg      FleetConfig
	agentCfg AgentConfig

	status atomic.Int32

	mu         sync.RWMutex
	once       sync.Once
	startedAt  time.Time
	finishedAt time.Time
	ag         Agent
	fw         *fanoutWriter
	vte        *vteHook
	done       chan struct{}
	prx        *proxy.Proxy
	pw         *io.PipeWriter
	logFile    *os.File
}

func NewRunner(task Task, ag Agent, cfg FleetConfig, agentCfg AgentConfig) *Runner {
	vteRows := cfg.VTERows
	if vteRows <= 0 {
		vteRows = 200
	}
	vteCols := agentCfg.PTYCols
	if vteCols <= 0 {
		vteCols = 220
	}
	return &Runner{
		task:     task,
		cfg:      cfg,
		agentCfg: agentCfg,
		ag:       ag,
		fw:       &fanoutWriter{primary: io.Discard},
		vte:      newVTEHook(vteCols, vteRows),
		done:     make(chan struct{}),
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

		r.prx = proxy.New(r.ag, pr, r.fw, r.agentCfg.PTYRows, r.agentCfg.PTYCols, hook.Chain{}, hook.Chain{tee, r.vte})
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

	go func() {
		<-r.done
		ln.Close()
		os.Remove(path)
	}()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				r.fw.addSecondary(conn)
				io.Copy(r.StdinWriter(), conn) //nolint:errcheck
				r.fw.removeSecondary(conn)
				conn.Close()
			}()
		}
	}()
}

func (r *Runner) Status() Status        { return Status(r.status.Load()) }
func (r *Runner) Done() <-chan struct{} { return r.done }

// Lines returns the current rendered screen of the virtual terminal emulator.
// All control sequences (backspace, \r overwrite, cursor movement, erase-line)
// have been applied, so the result matches what a real terminal would display.
func (r *Runner) Lines() []string { return r.vte.Screen() }

func (r *Runner) Task() Task         { return r.task }
func (r *Runner) setStatus(s Status) { r.status.Store(int32(s)) }

// SetOutput redirects agent output to w.
func (r *Runner) SetOutput(w io.Writer) { r.fw.setPrimary(w) }

// StdinWriter returns a writer whose bytes are forwarded to the agent's stdin.
func (r *Runner) StdinWriter() io.Writer {
	r.mu.RLock()
	defer r.mu.RUnlock()
	return r.pw
}

// Stop signals the underlying agent to terminate.
func (r *Runner) Stop() error { return r.ag.Stop() }

// Resize resizes both the underlying PTY agent and the virtual terminal
// emulator so Lines() keeps mirroring the agent's actual screen. Note the
// argument order: the PTY/agent take (rows, cols); vt10x takes (cols, rows).
func (r *Runner) Resize(rows, cols int) error {
	r.vte.Resize(cols, rows)
	return r.ag.Resize(rows, cols)
}

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
