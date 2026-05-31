# agentfleet Library Refactor Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Refactor agentfleet from a single CLI tool into a reusable Go library with public packages at the module root, a `Manager` abstraction for orchestration strategies, and `cmd/` replaced by `examples/`.

**Architecture:** Core interfaces (`Task`, `Fleet`, `Runner`, `Manager`, `Agent`) live in the root `agentfleet` package. Sub-packages `hook/`, `source/`, and `tui/` provide reusable implementations. `cmd/` is deleted; concrete managers live in `examples/`. `Task` no longer has `Steps()` — step injection is example-layer only via `Runner.StdinWriter()`.

**Tech Stack:** Go 1.26, `github.com/creack/pty`, `github.com/charmbracelet/bubbletea`, `golang.org/x/term`, `gopkg.in/yaml.v3`, `github.com/stretchr/testify`

---

## File Map

**Create (new public packages):**
- `hook/hook.go` — Hook, Dir, HookFunc, Chain, Logger
- `hook/file_logger.go` — FileLogger
- `hook/hook_test.go` — moved + updated from `internal/hook/hook_test.go`
- `source/source.go` — Source interface, StepTask, Step
- `source/file.go` — FileSource
- `source/markdown.go` — MarkdownSource
- `source/http.go` — HTTPSource
- `source/generate.go` — GenerateSource
- `source/file_test.go`, `source/markdown_test.go`, `source/http_test.go`, `source/generate_test.go`
- `tui/tui.go` — Bubbletea program taking `*Fleet` + `TUIConfig`

**Create (root package):**
- `task.go` — Task interface, BasicTask
- `status.go` — Status constants
- `manager.go` — Manager interface
- `config.go` — Config, FleetConfig, TUIConfig, AgentConfig, DefaultConfig, AgentConfigFromTerminal
- `agent.go` — Agent interface, PtyAgent, MockAgent
- `pty_darwin.go`, `pty_linux.go` — platform-specific PTY helpers (moved)
- `runner.go` — Runner (no steps; includes socket server)
- `fleet.go` — Fleet (dynamic registry, semaphore, WaitGroup)
- `runner_test.go`, `fleet_test.go`, `agent_test.go`, `task_test.go`

**Modify:**
- `internal/proxy/proxy.go` — replace `internal/agent` + `internal/hook` imports with local interface + `agentfleet/hook`

**Create (examples):**
- `examples/attach/main.go` — moved from `cmd/attach/main.go`
- `examples/file-manager/main.go` — FileManager with StepTask + TUI
- `examples/http-manager/main.go` — HTTPManager with StepTask + TUI
- `examples/generate-manager/main.go` — GenerateManager with TUI

**Update:**
- `examples/taskserver/main.go` — use `source.StepTask` instead of `fleet.BasicTask`

**Delete:**
- `internal/fleet/` (task.go, runner.go, status.go, *_test.go)
- `internal/agent/` (agent.go, pty_agent.go, pty_darwin.go, pty_linux.go, agent_test.go)
- `internal/hook/` (hook.go, file_logger.go, logger.go, hook_test.go)
- `internal/source/` (all files)
- `cmd/agentfleet/` (main.go, tui.go, socketserver.go)
- `cmd/attach/` (main.go)

---

## Task 1: Create `hook/` Package

**Files:**
- Create: `hook/hook.go`
- Create: `hook/file_logger.go`
- Create: `hook/hook_test.go`

- [ ] **Step 1: Write failing test**

```bash
mkdir -p hook
```

Create `hook/hook_test.go`:
```go
package hook_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hoaitan/agentfleet/hook"
)

func TestEmptyChainPassThrough(t *testing.T) {
	out, err := hook.Chain{}.Process([]byte("hello"), hook.DirOut)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), out)
}

func TestChainTransforms(t *testing.T) {
	ch := hook.Chain{
		hook.HookFunc(func(data []byte, dir hook.Dir) ([]byte, error) {
			return append([]byte("!"), data...), nil
		}),
	}
	out, err := ch.Process([]byte("hi"), hook.DirIn)
	require.NoError(t, err)
	assert.Equal(t, []byte("!hi"), out)
}

func TestChainFailOpen(t *testing.T) {
	ch := hook.Chain{
		hook.HookFunc(func(data []byte, dir hook.Dir) ([]byte, error) {
			return nil, errors.New("fail")
		}),
	}
	out, err := ch.Process([]byte("data"), hook.DirOut)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), out)
}

func TestChainSuppressNil(t *testing.T) {
	ch := hook.Chain{
		hook.HookFunc(func(data []byte, dir hook.Dir) ([]byte, error) {
			return nil, nil
		}),
	}
	out, err := ch.Process([]byte("data"), hook.DirOut)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestFileLogger(t *testing.T) {
	var buf strings.Builder
	fl := hook.NewFileLogger(&buf)
	out, err := fl.Process([]byte("hello"), hook.DirOut)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), out)
	assert.Contains(t, buf.String(), "[OUT]")
	assert.Contains(t, buf.String(), "hello")
}

func TestLogger(t *testing.T) {
	lg := hook.NewLogger(10)
	out, err := lg.Process([]byte("world"), hook.DirIn)
	require.NoError(t, err)
	assert.Equal(t, []byte("world"), out)
	evt := <-lg.Events
	assert.Equal(t, hook.DirIn, evt.Dir)
	assert.Equal(t, []byte("world"), evt.Data)
}
```

- [ ] **Step 2: Run test — expect compile failure**

```bash
go test ./hook/
```
Expected: `cannot find package "github.com/hoaitan/agentfleet/hook"`

- [ ] **Step 3: Create `hook/hook.go`**

```go
package hook

// Dir indicates byte flow direction.
type Dir uint8

const (
	DirIn  Dir = iota // user / wrapper → agent
	DirOut            // agent → display
)

// Hook processes a byte slice in a given direction.
// Returning nil bytes suppresses the data.
// Returning a non-nil error causes the chain to skip this hook (fail-open).
type Hook interface {
	Process(data []byte, dir Dir) ([]byte, error)
}

// HookFunc adapts a function to the Hook interface.
type HookFunc func([]byte, Dir) ([]byte, error)

func (f HookFunc) Process(data []byte, dir Dir) ([]byte, error) { return f(data, dir) }

// Chain is an ordered list of Hooks applied in sequence.
type Chain []Hook

func (c Chain) Process(data []byte, dir Dir) ([]byte, error) {
	for _, h := range c {
		out, err := h.Process(data, dir)
		if err != nil {
			continue
		}
		if out == nil {
			return nil, nil
		}
		data = out
	}
	return data, nil
}

// Event is one intercepted byte slice snapshot.
type Event struct {
	Dir  Dir
	Data []byte
}

// Logger records intercepted bytes to a non-blocking channel.
type Logger struct {
	Events chan Event
}

func NewLogger(bufSize int) *Logger {
	return &Logger{Events: make(chan Event, bufSize)}
}

func (l *Logger) Process(data []byte, dir Dir) ([]byte, error) {
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case l.Events <- Event{Dir: dir, Data: cp}:
	default:
	}
	return data, nil
}
```

- [ ] **Step 4: Create `hook/file_logger.go`**

```go
package hook

import (
	"fmt"
	"io"
	"sync"
)

// FileLogger writes [IN]/[OUT]-prefixed lines to an io.Writer for every chunk.
type FileLogger struct {
	w  io.Writer
	mu sync.Mutex
}

func NewFileLogger(w io.Writer) *FileLogger { return &FileLogger{w: w} }

func (l *FileLogger) Process(data []byte, dir Dir) ([]byte, error) {
	prefix := "[IN]"
	if dir == DirOut {
		prefix = "[OUT]"
	}
	l.mu.Lock()
	fmt.Fprintf(l.w, "%s %s\n", prefix, data) //nolint:errcheck
	l.mu.Unlock()
	return data, nil
}
```

- [ ] **Step 5: Run tests — expect PASS**

```bash
go test ./hook/
```
Expected: `ok  github.com/hoaitan/agentfleet/hook`

- [ ] **Step 6: Commit**

```bash
git add hook/
git commit -m "feat: add public hook/ package"
```

---

## Task 2: Create Root Package Core Types

**Files:**
- Create: `task.go`
- Create: `status.go`
- Create: `manager.go`
- Create: `config.go`
- Create: `task_test.go`

- [ ] **Step 1: Write failing test**

Create `task_test.go`:
```go
package agentfleet_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"

	agentfleet "github.com/hoaitan/agentfleet"
)

func TestBasicTaskImplementsTask(t *testing.T) {
	var _ agentfleet.Task = &agentfleet.BasicTask{}
}

func TestBasicTaskAccessors(t *testing.T) {
	task := &agentfleet.BasicTask{TaskID: "t-1", TaskName: "Say Hello", Cmd: "claude"}
	assert.Equal(t, "t-1", task.ID())
	assert.Equal(t, "Say Hello", task.Name())
	assert.Equal(t, "claude", task.Command())
}

func TestDefaultConfig(t *testing.T) {
	cfg := agentfleet.DefaultConfig()
	assert.Equal(t, 9, cfg.Fleet.MaxConcurrent)
	assert.Equal(t, 200, cfg.Fleet.RingBufferSize)
	assert.Equal(t, "/tmp", cfg.Fleet.SocketDir)
	assert.Equal(t, "/tmp", cfg.Fleet.LogDir)
	assert.Equal(t, 3, cfg.TUI.Columns)
	assert.Equal(t, 500*time.Millisecond, cfg.TUI.RefreshRate)
	assert.Equal(t, 24, cfg.Agent.PTYRows)
	assert.Equal(t, 220, cfg.Agent.PTYCols)
}
```

- [ ] **Step 2: Run test — expect compile failure**

```bash
go test .
```
Expected: `no Go files in .` or package not found errors.

- [ ] **Step 3: Create `task.go`**

```go
package agentfleet

// Task is the minimum contract for any runnable unit.
type Task interface {
	ID()      string
	Name()    string
	Command() string
}

// BasicTask is the default Task implementation.
type BasicTask struct {
	TaskID   string `json:"id"      yaml:"id"`
	TaskName string `json:"name"    yaml:"name"`
	Cmd      string `json:"command" yaml:"command"`
}

func (t *BasicTask) ID()      string { return t.TaskID }
func (t *BasicTask) Name()    string { return t.TaskName }
func (t *BasicTask) Command() string { return t.Cmd }

// CommandFields splits Task.Command() into argv.
func CommandFields(t Task) []string {
	// avoid importing strings in this file — used inline
	return splitFields(t.Command())
}
```

- [ ] **Step 4: Create `status.go`**

```go
package agentfleet

// Status represents the lifecycle state of a Runner.
type Status int32

const (
	StatusPending Status = iota
	StatusRunning
	StatusDone
	StatusFailed
)

func (s Status) String() string {
	switch s {
	case StatusRunning:
		return "running"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	default:
		return "pending"
	}
}
```

- [ ] **Step 5: Create `manager.go`**

```go
package agentfleet

import "context"

// Manager is the orchestration strategy extension point.
// Implementations load or stream tasks into a Fleet via Add().
type Manager interface {
	Run(ctx context.Context, fleet *Fleet) error
}
```

- [ ] **Step 6: Create `config.go`**

```go
package agentfleet

import (
	"os"
	"time"

	"golang.org/x/term"
)

// Config holds all configuration for a fleet run.
type Config struct {
	Fleet FleetConfig
	TUI   TUIConfig
	Agent AgentConfig
}

// FleetConfig controls task scheduling and I/O paths.
type FleetConfig struct {
	MaxConcurrent  int    // max tasks running in parallel — default: 9
	RingBufferSize int    // output lines kept per runner   — default: 200
	SocketDir      string // Unix socket dir; empty = no socket server — default: /tmp
	LogDir         string // session log dir; empty = no log file    — default: /tmp
}

// TUIConfig controls the Bubbletea dashboard appearance.
type TUIConfig struct {
	Columns      int           // grid columns               — default: 3
	PreviewLines int           // output lines shown in card — default: 3
	CardWidth    int           // card width in chars        — default: 64
	RefreshRate  time.Duration // TUI tick interval          — default: 500ms
}

// AgentConfig controls PTY dimensions.
type AgentConfig struct {
	PTYRows int // default: 24
	PTYCols int // default: 220
}

// DefaultConfig returns sensible production defaults.
func DefaultConfig() Config {
	return Config{
		Fleet: FleetConfig{
			MaxConcurrent:  9,
			RingBufferSize: 200,
			SocketDir:      "/tmp",
			LogDir:         "/tmp",
		},
		TUI: TUIConfig{
			Columns:      3,
			PreviewLines: 3,
			CardWidth:    64,
			RefreshRate:  500 * time.Millisecond,
		},
		Agent: AgentConfig{PTYRows: 24, PTYCols: 220},
	}
}

// AgentConfigFromTerminal reads actual terminal dimensions.
// Falls back to DefaultConfig().Agent when stdout is not a TTY.
func AgentConfigFromTerminal() AgentConfig {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || rows <= 0 || cols <= 0 {
		return DefaultConfig().Agent
	}
	return AgentConfig{PTYRows: rows, PTYCols: cols}
}
```

- [ ] **Step 7: Create `strings.go`** (helper used by task.go to avoid import in task.go itself)

Actually, add `CommandFields` properly in `task.go` by importing strings directly:

Edit `task.go` — replace the `CommandFields` func with:
```go
package agentfleet

import "strings"

// Task is the minimum contract for any runnable unit.
type Task interface {
	ID()      string
	Name()    string
	Command() string
}

// BasicTask is the default Task implementation.
type BasicTask struct {
	TaskID   string `json:"id"      yaml:"id"`
	TaskName string `json:"name"    yaml:"name"`
	Cmd      string `json:"command" yaml:"command"`
}

func (t *BasicTask) ID()      string { return t.TaskID }
func (t *BasicTask) Name()    string { return t.TaskName }
func (t *BasicTask) Command() string { return t.Cmd }

// CommandFields splits Task.Command() into argv for NewPtyAgent.
func CommandFields(t Task) []string { return strings.Fields(t.Command()) }
```

- [ ] **Step 8: Run tests — expect PASS**

```bash
go test . -run TestBasicTask -run TestDefaultConfig -v
```
Expected: all PASS

- [ ] **Step 9: Commit**

```bash
git add task.go status.go manager.go config.go task_test.go
git commit -m "feat: add root package core types (Task, Status, Manager, Config)"
```

---

## Task 3: Create Root Package Agent Types

**Files:**
- Create: `agent.go`
- Create: `pty_darwin.go`
- Create: `pty_linux.go`
- Create: `agent_test.go`

- [ ] **Step 1: Write failing test**

Create `agent_test.go`:
```go
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
```

- [ ] **Step 2: Run test — expect compile failure**

```bash
go test . -run TestMockAgent
```
Expected: `undefined: agentfleet.NewMockAgent`

- [ ] **Step 3: Create `agent.go`**

```go
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
```

- [ ] **Step 4: Create `pty_agent.go`**

```go
package agentfleet

import (
	"os"
	"os/exec"
	"sync"
	"syscall"

	"github.com/creack/pty"
)

// PtyAgent runs any CLI process inside a PTY.
type PtyAgent struct {
	cmd  *exec.Cmd
	mu   sync.RWMutex
	ptmx *os.File
	done chan struct{}
	once sync.Once
}

func NewPtyAgent(command []string) *PtyAgent {
	return &PtyAgent{
		cmd:  exec.Command(command[0], command[1:]...),
		done: make(chan struct{}),
	}
}

func (a *PtyAgent) Start(rows, cols int) error {
	ptmx, pts, err := pty.Open()
	if err != nil {
		return err
	}
	if err := pty.Setsize(ptmx, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)}); err != nil {
		ptmx.Close()
		pts.Close()
		return err
	}
	setRawPtyFd(int(pts.Fd()))

	a.cmd.Stdin = pts
	a.cmd.Stdout = pts
	a.cmd.Stderr = pts

	if a.cmd.SysProcAttr == nil {
		a.cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	a.cmd.SysProcAttr.Setsid = true
	a.cmd.SysProcAttr.Setctty = true
	a.cmd.SysProcAttr.Ctty = 1

	if err := a.cmd.Start(); err != nil {
		ptmx.Close()
		pts.Close()
		return err
	}
	pts.Close()

	a.ptmx = ptmx
	go func() {
		_ = a.cmd.Wait()
		a.mu.Lock()
		if a.ptmx != nil {
			_ = a.ptmx.Close()
			a.ptmx = nil
		}
		a.mu.Unlock()
		a.once.Do(func() { close(a.done) })
	}()
	return nil
}

func (a *PtyAgent) Write(p []byte) (int, error) {
	a.mu.RLock()
	f := a.ptmx
	a.mu.RUnlock()
	if f == nil {
		return 0, os.ErrClosed
	}
	return f.Write(p)
}

func (a *PtyAgent) Read(p []byte) (int, error) {
	a.mu.RLock()
	f := a.ptmx
	a.mu.RUnlock()
	if f == nil {
		return 0, os.ErrClosed
	}
	return f.Read(p)
}

func (a *PtyAgent) Resize(rows, cols int) error {
	a.mu.RLock()
	f := a.ptmx
	a.mu.RUnlock()
	if f == nil {
		return os.ErrClosed
	}
	return pty.Setsize(f, &pty.Winsize{Rows: uint16(rows), Cols: uint16(cols)})
}

func (a *PtyAgent) Stop() error {
	if a.cmd.Process == nil {
		return nil
	}
	if err := a.cmd.Process.Signal(syscall.SIGTERM); err != nil {
		return a.cmd.Process.Kill()
	}
	return nil
}

func (a *PtyAgent) Done() <-chan struct{} { return a.done }
```

- [ ] **Step 5: Create `pty_darwin.go`**

Copy content from `internal/agent/pty_darwin.go` and change `package agent` to `package agentfleet`:
```go
package agentfleet

import "golang.org/x/sys/unix"

func setRawPtyFd(fd int) {
	// Read the current termios, strip ECHO and canonical mode, write back.
	termios, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}
	termios.Lflag &^= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ICANON
	unix.IoctlSetTermios(fd, unix.TIOCSETA, termios) //nolint:errcheck
}
```

- [ ] **Step 6: Create `pty_linux.go`**

Copy content from `internal/agent/pty_linux.go` and change `package agent` to `package agentfleet`:
```go
package agentfleet

import "golang.org/x/sys/unix"

func setRawPtyFd(fd int) {
	termios, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return
	}
	termios.Lflag &^= unix.ECHO | unix.ECHOE | unix.ECHOK | unix.ECHONL | unix.ICANON
	unix.IoctlSetTermios(fd, unix.TCSETS, termios) //nolint:errcheck
}
```

- [ ] **Step 7: Run tests — expect PASS**

```bash
go test . -run TestMockAgent -v
```
Expected: all 3 tests PASS

- [ ] **Step 8: Commit**

```bash
git add agent.go pty_agent.go pty_darwin.go pty_linux.go agent_test.go
git commit -m "feat: add Agent interface, PtyAgent, MockAgent to root package"
```

---

## Task 4: Update `internal/proxy` to Use `agentfleet/hook`

**Files:**
- Modify: `internal/proxy/proxy.go`

- [ ] **Step 1: Update `internal/proxy/proxy.go`**

Replace the entire file:
```go
package proxy

import (
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/hoaitan/agentfleet/hook"
)

// agentProxy is a local interface satisfied by any Agent (avoids importing root package).
type agentProxy interface {
	Start(rows, cols int) error
	Write(p []byte) (int, error)
	Read(p []byte) (int, error)
	Resize(rows, cols int) error
	Stop() error
	Done() <-chan struct{}
}

// Proxy is a transparent PTY proxy: bytes flow stdin→agent and agent→stdout
// through hook chains.
type Proxy struct {
	ag       agentProxy
	in       io.Reader
	out      io.Writer
	inChain  hook.Chain
	outChain hook.Chain
}

func New(ag agentProxy, in io.Reader, out io.Writer, inChain, outChain hook.Chain) *Proxy {
	return &Proxy{ag: ag, in: in, out: out, inChain: inChain, outChain: outChain}
}

// Inject writes bytes into the agent's input stream as if the user typed them.
func (p *Proxy) Inject(data []byte) error {
	out, _ := p.inChain.Process(data, hook.DirIn)
	if out != nil {
		_, err := p.ag.Write(out)
		return err
	}
	return nil
}

// Run starts the wrapped command and blocks until it exits.
func (p *Proxy) Run() error {
	rows, cols := 24, 80

	if f, ok := p.in.(*os.File); ok && term.IsTerminal(int(f.Fd())) {
		if c, r, err := term.GetSize(int(f.Fd())); err == nil {
			rows, cols = r, c
		}
		if oldState, err := term.MakeRaw(int(f.Fd())); err == nil {
			defer term.Restore(int(f.Fd()), oldState) //nolint:errcheck
			enableOutputProcessing(int(f.Fd()))
		}

		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, syscall.SIGWINCH)
		defer signal.Stop(sigCh)
		go func() {
			for range sigCh {
				if c, r, err := term.GetSize(int(f.Fd())); err == nil {
					_ = p.ag.Resize(r, c)
				}
			}
		}()
	}

	if err := p.ag.Start(rows, cols); err != nil {
		return err
	}

	go func() {
		buf := make([]byte, 256)
		for {
			n, err := p.in.Read(buf)
			if n > 0 {
				out, _ := p.inChain.Process(buf[:n], hook.DirIn)
				if out != nil {
					_, _ = p.ag.Write(out)
				}
			}
			if err != nil {
				return
			}
		}
	}()

	buf := make([]byte, 4096)
	for {
		n, err := p.ag.Read(buf)
		if n > 0 {
			out, _ := p.outChain.Process(buf[:n], hook.DirOut)
			if out != nil {
				_, _ = p.out.Write(out)
			}
		}
		if err != nil {
			return nil
		}
	}
}
```

- [ ] **Step 2: Verify proxy still compiles (old packages still exist)**

```bash
go build ./internal/proxy/
```
Expected: compiles cleanly.

- [ ] **Step 3: Run proxy tests**

```bash
go test ./internal/proxy/
```
Expected: PASS

- [ ] **Step 4: Commit**

```bash
git add internal/proxy/proxy.go
git commit -m "refactor: proxy uses agentfleet/hook and local agentProxy interface"
```

---

## Task 5: Create Root Package Runner

**Files:**
- Create: `runner.go`
- Create: `runner_test.go`

- [ ] **Step 1: Write failing test**

Create `runner_test.go`:
```go
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
	return agentfleet.FleetConfig{RingBufferSize: 200}
}

type writerFunc func([]byte) (int, error)

func (f writerFunc) Write(p []byte) (int, error) { return f(p) }

func TestRunnerStartAndStop(t *testing.T) {
	ag := agentfleet.NewMockAgent()
	task := &agentfleet.BasicTask{TaskID: "t1", TaskName: "Test", Cmd: "echo"}
	r := agentfleet.NewRunner(task, ag, testCfg())

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
	r := agentfleet.NewRunner(task, ag, testCfg())
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
	r := agentfleet.NewRunner(task, ag, testCfg())
	r.Start()

	_, err := r.StdinWriter().Write([]byte("hello\r"))
	require.NoError(t, err)

	got, err := ag.ReadInput(time.Second)
	require.NoError(t, err)
	assert.Equal(t, "hello\r", string(got))

	ag.Stop()
	<-r.Done()
}
```

- [ ] **Step 2: Run test — expect compile failure**

```bash
go test . -run TestRunner
```
Expected: `undefined: agentfleet.NewRunner`

- [ ] **Step 3: Create `runner.go`**

```go
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
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test . -run TestRunner -v
```
Expected: all 3 tests PASS

- [ ] **Step 5: Commit**

```bash
git add runner.go runner_test.go
git commit -m "feat: add Runner to root package (no steps, includes socket server)"
```

---

## Task 6: Create Root Package Fleet

**Files:**
- Create: `fleet.go`
- Create: `fleet_test.go`

- [ ] **Step 1: Write failing test**

Create `fleet_test.go`:
```go
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
	r := agentfleet.NewRunner(task, ag, testCfg())
	return r, ag
}

func TestFleetAddAndRunners(t *testing.T) {
	f := agentfleet.NewFleet(agentfleet.FleetConfig{MaxConcurrent: 3, RingBufferSize: 10})
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
	f := agentfleet.NewFleet(agentfleet.FleetConfig{MaxConcurrent: 2, RingBufferSize: 10})
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
	f := agentfleet.NewFleet(agentfleet.FleetConfig{MaxConcurrent: 9, RingBufferSize: 10})
	r, _ := newTestRunner("w1")
	r.Start()
	ctx, cancel := context.WithCancel(context.Background())
	require.NoError(t, f.Add(ctx, r))

	cancel()
	err := f.Wait(ctx)
	assert.ErrorIs(t, err, context.Canceled)
}
```

- [ ] **Step 2: Run test — expect compile failure**

```bash
go test . -run TestFleet
```
Expected: `undefined: agentfleet.NewFleet`

- [ ] **Step 3: Create `fleet.go`**

```go
package agentfleet

import (
	"context"
	"sync"
)

// Fleet is a thread-safe, dynamic registry of Runners.
// Managers add Runners via Add(); the TUI reads via Runners().
// Fleet enforces MaxConcurrent: Add() blocks until a slot is available.
type Fleet struct {
	mu  sync.RWMutex
	runners []*Runner
	sem chan struct{}
	wg  sync.WaitGroup
	cfg FleetConfig
}

func NewFleet(cfg FleetConfig) *Fleet {
	max := cfg.MaxConcurrent
	if max <= 0 {
		max = 9
	}
	return &Fleet{
		cfg: cfg,
		sem: make(chan struct{}, max),
	}
}

// Add registers a Runner with the Fleet and blocks until a concurrency slot is available.
// Returns ctx.Err() if the context is cancelled while waiting.
// The Runner must already be Start()-ed before calling Add().
func (f *Fleet) Add(ctx context.Context, r *Runner) error {
	select {
	case f.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	f.wg.Add(1)
	f.mu.Lock()
	f.runners = append(f.runners, r)
	f.mu.Unlock()
	go func() {
		<-r.Done()
		<-f.sem
		f.wg.Done()
	}()
	return nil
}

// Runners returns a snapshot of all registered Runners.
func (f *Fleet) Runners() []*Runner {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Runner, len(f.runners))
	copy(out, f.runners)
	return out
}

// Wait blocks until all Runners have completed or ctx is cancelled.
func (f *Fleet) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		f.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
```

- [ ] **Step 4: Run tests — expect PASS**

```bash
go test . -run TestFleet -v
```
Expected: all 3 tests PASS

- [ ] **Step 5: Run all root tests**

```bash
go test . -v
```
Expected: all tests PASS

- [ ] **Step 6: Commit**

```bash
git add fleet.go fleet_test.go
git commit -m "feat: add Fleet to root package (dynamic registry with MaxConcurrent)"
```

---

## Task 7: Create `source/` Package

**Files:**
- Create: `source/source.go` (Source interface + StepTask + Step)
- Create: `source/file.go`
- Create: `source/markdown.go`
- Create: `source/http.go`
- Create: `source/generate.go`
- Create: `source/file_test.go`
- Create: `source/markdown_test.go`
- Create: `source/http_test.go`
- Create: `source/generate_test.go`

- [ ] **Step 1: Write failing tests**

```bash
mkdir -p source
```

Create `source/file_test.go`:
```go
package source_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hoaitan/agentfleet/source"
)

func TestFileSourceJSON(t *testing.T) {
	f, _ := os.CreateTemp("", "*.json")
	f.WriteString(`[
		{"id":"t1","name":"JSON Task","command":"claude","steps":[{"delay":1,"command":"hello"}]}
	]`)
	f.Close()
	defer os.Remove(f.Name())

	src := &source.FileSource{Path: f.Name()}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "t1", tasks[0].ID())
	assert.Equal(t, "JSON Task", tasks[0].Name())

	st, ok := tasks[0].(*source.StepTask)
	require.True(t, ok, "expected *source.StepTask")
	require.Len(t, st.Steps(), 1)
	assert.Equal(t, "hello", st.Steps()[0].Command)
}

func TestFileSourceYAML(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`- id: y1
  name: YAML Task
  command: codex
  steps:
    - delay: 2
      command: "summarize this"
`)
	f.Close()
	defer os.Remove(f.Name())

	src := &source.FileSource{Path: f.Name()}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "y1", tasks[0].ID())

	st := tasks[0].(*source.StepTask)
	assert.Equal(t, "summarize this", st.Steps()[0].Command)
}

func TestFileSourceMissing(t *testing.T) {
	src := &source.FileSource{Path: "/tmp/no-such-file-agentfleet.json"}
	_, err := src.Load()
	require.Error(t, err)
}
```

- [ ] **Step 2: Run test — expect compile failure**

```bash
go test ./source/
```
Expected: `cannot find package`

- [ ] **Step 3: Create `source/source.go`**

```go
package source

import agentfleet "github.com/hoaitan/agentfleet"

// Source loads a list of tasks from some external system.
type Source interface {
	Load() ([]agentfleet.Task, error)
}

// Step is one timed injection: wait Delay seconds, then send Command.
// An empty Command string stops the agent.
type Step struct {
	Delay   float64 `json:"delay"   yaml:"delay"`
	Command string  `json:"command" yaml:"command"`
}

// StepTask is a Task with timed injection steps.
// It is the serialized format used by all built-in sources (JSON/YAML/Markdown/LLM).
// Example managers type-assert to *StepTask to access Steps().
type StepTask struct {
	TaskID    string `json:"id"      yaml:"id"`
	TaskName  string `json:"name"    yaml:"name"`
	Cmd       string `json:"command" yaml:"command"`
	TaskSteps []Step `json:"steps"   yaml:"steps"`
}

func (t *StepTask) ID()      string { return t.TaskID }
func (t *StepTask) Name()    string { return t.TaskName }
func (t *StepTask) Command() string { return t.Cmd }
func (t *StepTask) Steps()   []Step { return t.TaskSteps }
```

- [ ] **Step 4: Create `source/file.go`**

```go
package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	agentfleet "github.com/hoaitan/agentfleet"
)

// FileSource loads tasks from a local JSON or YAML file.
type FileSource struct {
	Path string
}

func (s *FileSource) Load() ([]agentfleet.Task, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.Path, err)
	}

	var raw []*StepTask
	ext := strings.ToLower(filepath.Ext(s.Path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse yaml %s: %w", s.Path, err)
		}
	default:
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse json %s: %w", s.Path, err)
		}
	}

	tasks := make([]agentfleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
```

- [ ] **Step 5: Create `source/markdown.go`**

```go
package source

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	agentfleet "github.com/hoaitan/agentfleet"
)

// MarkdownSource loads tasks from a Markdown file.
//
// Format:
//
//	## Task: <name>
//	command: <cli>
//
//	- delay: <N>, inject: "<text>"
type MarkdownSource struct {
	Path string
}

func (s *MarkdownSource) Load() ([]agentfleet.Task, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", s.Path, err)
	}
	defer f.Close()

	var tasks []*StepTask
	var current *StepTask

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "## Task: ") {
			if current != nil {
				tasks = append(tasks, current)
			}
			name := strings.TrimPrefix(line, "## Task: ")
			current = &StepTask{TaskID: slugify(name), TaskName: name}
			continue
		}

		if current == nil {
			continue
		}

		if strings.HasPrefix(line, "command: ") {
			current.Cmd = strings.TrimPrefix(line, "command: ")
			continue
		}

		if strings.HasPrefix(line, "- delay: ") {
			if step, err := parseMarkdownStep(line); err == nil {
				current.TaskSteps = append(current.TaskSteps, step)
			}
		}
	}

	if current != nil {
		tasks = append(tasks, current)
	}

	if err := scanner.Err(); err != nil {
		return nil, err
	}

	out := make([]agentfleet.Task, len(tasks))
	for i, t := range tasks {
		out[i] = t
	}
	return out, nil
}

func slugify(s string) string {
	s = strings.ToLower(s)
	var b strings.Builder
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= '0' && r <= '9':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	return strings.Trim(b.String(), "-")
}

func parseMarkdownStep(line string) (Step, error) {
	line = strings.TrimPrefix(line, "- ")
	parts := strings.SplitN(line, ", inject: ", 2)
	if len(parts) != 2 {
		return Step{}, fmt.Errorf("invalid step: %q", line)
	}
	delayStr := strings.TrimPrefix(parts[0], "delay: ")
	delay, err := strconv.ParseFloat(delayStr, 64)
	if err != nil {
		return Step{}, fmt.Errorf("invalid delay: %q", delayStr)
	}
	return Step{Delay: delay, Command: strings.Trim(parts[1], `"`)}, nil
}
```

- [ ] **Step 6: Create `source/http.go`**

```go
package source

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	agentfleet "github.com/hoaitan/agentfleet"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// HTTPSource loads tasks from a JSON HTTP endpoint.
// The endpoint must return a JSON array of StepTask-compatible objects.
type HTTPSource struct {
	URL string
}

func (s *HTTPSource) Load() ([]agentfleet.Task, error) {
	resp, err := httpClient.Get(s.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", s.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var raw []*StepTask
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	tasks := make([]agentfleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
```

- [ ] **Step 7: Create `source/generate.go`**

```go
package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	agentfleet "github.com/hoaitan/agentfleet"
)

var generateClient = &http.Client{Timeout: 60 * time.Second}

const defaultAPIURL = "https://api.anthropic.com/v1/messages"

const generateSystemPrompt = `You are a task planner for an AI agent fleet runner.
Given a goal, output a JSON array of tasks. Each task must have:
- id: string (unique, kebab-case)
- name: string (human-readable title)
- command: string (CLI binary to run, e.g. "claude")
- steps: array of objects with "delay" (float, seconds to wait) and "command" (string to inject; empty string stops the agent)

Return ONLY a valid JSON array. No markdown, no code fences, no explanation.`

// GenerateSource calls the Claude API to generate tasks from a natural-language goal.
// Set ANTHROPIC_API_KEY in the environment before calling Load().
type GenerateSource struct {
	goal   string
	apiURL string
	apiKey string
}

func NewGenerateSource(goal, apiURL, apiKey string) *GenerateSource {
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return &GenerateSource{goal: goal, apiURL: apiURL, apiKey: apiKey}
}

func (s *GenerateSource) Load() ([]agentfleet.Task, error) {
	if s.apiKey == "" {
		return nil, fmt.Errorf("ANTHROPIC_API_KEY is not set")
	}

	body, err := json.Marshal(map[string]any{
		"model":      "claude-sonnet-4-6",
		"max_tokens": 2048,
		"system":     generateSystemPrompt,
		"messages":   []map[string]any{{"role": "user", "content": s.goal}},
	})
	if err != nil {
		return nil, fmt.Errorf("marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(context.Background(), http.MethodPost, s.apiURL, bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-api-key", s.apiKey)
	req.Header.Set("anthropic-version", "2023-06-01")

	resp, err := generateClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		var apiErr struct {
			Error struct{ Message string `json:"message"` } `json:"error"`
		}
		json.NewDecoder(resp.Body).Decode(&apiErr) //nolint:errcheck
		msg := apiErr.Error.Message
		if msg == "" {
			msg = http.StatusText(resp.StatusCode)
		}
		return nil, fmt.Errorf("API error %d: %s", resp.StatusCode, msg)
	}

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from model")
	}

	var raw []*StepTask
	if err := json.Unmarshal([]byte(result.Content[0].Text), &raw); err != nil {
		return nil, fmt.Errorf("parse generated tasks: %w", err)
	}

	tasks := make([]agentfleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
```

- [ ] **Step 8: Copy and update remaining source tests**

Create `source/markdown_test.go` (copy from `internal/source/markdown_test.go`, update import to `github.com/hoaitan/agentfleet/source`, update Steps assertions to use type assertion `tasks[0].(*source.StepTask).Steps()`).

Create `source/http_test.go` (copy from `internal/source/http_test.go`, update import, same Step access pattern).

Create `source/generate_test.go` (copy from `internal/source/generate_test.go`, update import, same Step access pattern).

- [ ] **Step 9: Run source tests — expect PASS**

```bash
go test ./source/ -run TestFile -run TestMarkdown -v
```
Expected: PASS (HTTP and Generate tests need network/API, run separately)

- [ ] **Step 10: Commit**

```bash
git add source/
git commit -m "feat: add public source/ package with StepTask"
```

---

## Task 8: Create `tui/` Package

**Files:**
- Create: `tui/tui.go`

- [ ] **Step 1: Create `tui/tui.go`**

```go
package tui

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	agentfleet "github.com/hoaitan/agentfleet"
)

var ansiRe = regexp.MustCompile(
	`\x1b(?:` +
		`\][^\x07\x1b]*(?:\x07|\x1b\\)` +
		`|[@-Z\\-_]` +
		`|\[[0-?]*[ -/]*[@-~]` +
		`|[PX^_][^\x1b]*\x1b\\` +
		`)`,
)

func stripANSI(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 || r == '\t' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c084fc"))
	styleSummary = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	styleMeta    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	styleSelID   = lipgloss.NewStyle().Foreground(lipgloss.Color("#c084fc"))
	styleOutput  = lipgloss.NewStyle().Foreground(lipgloss.Color("#d1d5db"))
	styleFooter  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4b5563"))
	styleRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ade80"))
	styleDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("#34d399"))
	styleFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
	stylePending = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))

	cardSelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7c3aed")).
			Background(lipgloss.Color("#1e1730"))

	cardOtherStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#374151")).
			Background(lipgloss.Color("#1a1826"))
)

type tickMsg struct{}
type ctxDoneMsg struct{}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

func ctxDoneCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		<-ctx.Done()
		return ctxDoneMsg{}
	}
}

type model struct {
	fleet    *agentfleet.Fleet
	cfg      agentfleet.TUIConfig
	onAttach func(taskID string)
	ctx      context.Context
	cursor   int
	termW    int
	termH    int
}

// Run starts the Bubbletea TUI and blocks until the user quits or ctx is cancelled.
// onAttach is called when the user presses Enter on a running task.
// If onAttach is nil, the default behaviour opens an iTerm2 tab with the attach binary.
func Run(ctx context.Context, fleet *agentfleet.Fleet, cfg agentfleet.TUIConfig, onAttach func(taskID string)) error {
	if onAttach == nil {
		onAttach = defaultOnAttach
	}
	m := model{fleet: fleet, cfg: cfg, onAttach: onAttach, ctx: ctx}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func defaultOnAttach(taskID string) {
	attachBin, _ := filepath.Abs("./attach")
	script := fmt.Sprintf(`tell application "iTerm2"
	tell current window
		create tab with default profile command "%s %s"
	end tell
end tell`, attachBin, taskID)
	exec.Command("osascript", "-e", script).Start() //nolint:errcheck
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(m.cfg.RefreshRate), ctxDoneCmd(m.ctx))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ctxDoneMsg:
		return m, tea.Quit
	case tickMsg:
		return m, tickCmd(m.cfg.RefreshRate)
	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		return m, nil
	case tea.KeyMsg:
		runners := m.fleet.Runners()
		switch msg.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(runners)-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			if len(runners) > 0 && runners[m.cursor].Status() == agentfleet.StatusRunning {
				m.onAttach(runners[m.cursor].Task().ID())
			}
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
		switch msg.String() {
		case "j":
			if m.cursor < len(m.fleet.Runners())-1 {
				m.cursor++
			}
		case "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m model) View() string { return renderListView(m) }

func renderHeader(m model) string {
	runners := m.fleet.Runners()
	var running, done, failed int
	for _, r := range runners {
		switch r.Status() {
		case agentfleet.StatusRunning:
			running++
		case agentfleet.StatusDone:
			done++
		case agentfleet.StatusFailed:
			failed++
		}
	}
	summary := fmt.Sprintf("%d tasks · %d running · %d done", len(runners), running, done)
	if failed > 0 {
		summary += fmt.Sprintf(" · %d failed", failed)
	}
	return styleTitle.Render("◈ agentfleet") + "  " + styleSummary.Render(summary)
}

func renderListView(m model) string {
	runners := m.fleet.Runners()
	var b strings.Builder
	b.WriteString(renderHeader(m) + "\n\n")
	for i, r := range runners {
		b.WriteString(renderCard(r, m.cfg, i == m.cursor) + "\n")
	}
	b.WriteString("\n" + styleFooter.Render("[↑↓ j/k] navigate  [enter] open tab  [q] quit"))
	return b.String()
}

func statusBadge(s agentfleet.Status) string {
	const w = 10
	switch s {
	case agentfleet.StatusRunning:
		return styleRunning.Width(w).Render("● running")
	case agentfleet.StatusDone:
		return styleDone.Width(w).Render("✓ done")
	case agentfleet.StatusFailed:
		return styleFailed.Width(w).Render("✗ failed")
	default:
		return stylePending.Width(w).Render("○ pending")
	}
}

func renderCard(r *agentfleet.Runner, cfg agentfleet.TUIConfig, selected bool) string {
	cursor := "  "
	idStyle := styleMeta
	if selected {
		cursor = "▶ "
		idStyle = styleSelID
	}

	badge := statusBadge(r.Status())
	task := r.Task()
	cursorW := lipgloss.Width(cursor)
	idW := lipgloss.Width(idStyle.Render(task.ID()))
	badgeW := lipgloss.Width(badge)
	nameMaxW := cfg.CardWidth - cursorW - idW - 2 - badgeW - 1
	if nameMaxW < 8 {
		nameMaxW = 8
	}
	name := truncateVisual(task.Name(), nameMaxW)
	left := cursor + idStyle.Render(task.ID()) + "  " + name
	gap := cfg.CardWidth - lipgloss.Width(left) - badgeW
	if gap < 1 {
		gap = 1
	}

	var lines []string
	lines = append(lines, left+strings.Repeat(" ", gap)+badge)

	if selected {
		elapsed := elapsedStr(r.StartedAt(), r.FinishedAt())
		lines = append(lines, styleMeta.Render("  "+elapsed))

		allLines := r.Lines()
		start := len(allLines) - cfg.PreviewLines
		if start < 0 {
			start = 0
		}
		preview := allLines[start:]
		lines = append(lines, "")
		for i := 0; i < cfg.PreviewLines; i++ {
			if i < len(preview) {
				text := truncateVisual(stripANSI(preview[i]), cfg.CardWidth-4)
				lines = append(lines, styleOutput.Render("  "+text))
			} else {
				lines = append(lines, "")
			}
		}
		return cardSelStyle.Width(cfg.CardWidth).Render(strings.Join(lines, "\n"))
	}

	return cardOtherStyle.Width(cfg.CardWidth).Render(strings.Join(lines, "\n"))
}

func elapsedStr(start, end time.Time) string {
	if start.IsZero() {
		return ""
	}
	if end.IsZero() {
		end = time.Now()
	}
	d := end.Sub(start).Round(time.Second)
	return fmt.Sprintf("%02d:%02d elapsed", int(d.Minutes()), int(d.Seconds())%60)
}

func truncateVisual(s string, maxW int) string {
	w := 0
	runes := []rune(s)
	for i, ch := range runes {
		cw := lipgloss.Width(string(ch))
		if w+cw > maxW {
			if w+1 <= maxW {
				return string(runes[:i]) + "…"
			}
			return string(runes[:i])
		}
		w += cw
	}
	return s
}
```

- [ ] **Step 2: Build the tui package**

```bash
go build ./tui/
```
Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add tui/
git commit -m "feat: add public tui/ package (takes *Fleet + TUIConfig)"
```

---

## Task 9: Create `examples/attach/`

**Files:**
- Create: `examples/attach/main.go`

- [ ] **Step 1: Create `examples/attach/main.go`**

Copy content from `cmd/attach/main.go` verbatim — no changes needed:
```go
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/term"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: attach <task-id>")
		os.Exit(1)
	}
	if err := attach(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func attach(taskID string) error {
	var (
		conn net.Conn
		err  error
	)
	for i := 0; i < 30; i++ {
		conn, err = net.Dial("unix", "/tmp/agentfleet-"+taskID+".sock")
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("connect to task %q: %w", taskID, err)
	}
	defer conn.Close()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState) //nolint:errcheck

	go io.Copy(conn, os.Stdin)   //nolint:errcheck
	io.Copy(os.Stdout, conn)     //nolint:errcheck
	fmt.Print("\r\n[session ended]\r\n")
	return nil
}
```

- [ ] **Step 2: Build**

```bash
go build ./examples/attach/
```
Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add examples/attach/
git commit -m "feat: move attach binary to examples/attach/"
```

---

## Task 10: Create `examples/file-manager/`

**Files:**
- Create: `examples/file-manager/main.go`

- [ ] **Step 1: Create `examples/file-manager/main.go`**

```go
// file-manager demonstrates loading tasks from a JSON/YAML/Markdown file and
// running them with the agentfleet TUI and step injection.
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	agentfleet "github.com/hoaitan/agentfleet"
	"github.com/hoaitan/agentfleet/source"
	"github.com/hoaitan/agentfleet/tui"
)

// validTaskID accepts only lowercase alphanumeric and hyphens.
var taskIDRe = regexp.MustCompile(`^[a-z0-9\-]+$`)

func main() {
	src := flag.String("source", "", "task file: .md, .json, or .yaml")
	flag.Parse()
	if *src == "" {
		log.Fatal("--source <file> is required")
	}

	tasks, err := loadTasks(*src)
	if err != nil {
		log.Fatalf("load tasks: %v", err)
	}
	if len(tasks) == 0 {
		log.Fatal("task list is empty")
	}

	cfg := agentfleet.DefaultConfig()
	cfg.Agent = agentfleet.AgentConfigFromTerminal()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fleet := agentfleet.NewFleet(cfg.Fleet)

	for _, task := range tasks {
		if strings.TrimSpace(task.Command()) == "" {
			log.Fatalf("task %q has empty command", task.ID())
		}
		if !taskIDRe.MatchString(task.ID()) {
			log.Fatalf("task %q has invalid ID %q (must match [a-z0-9-]+)", task.Name(), task.ID())
		}

		ag := agentfleet.NewPtyAgent(agentfleet.CommandFields(task))
		r := agentfleet.NewRunner(task, ag, cfg.Fleet)
		r.Start()

		if err := fleet.Add(ctx, r); err != nil {
			log.Fatalf("add task: %v", err)
		}

		if st, ok := task.(*source.StepTask); ok && len(st.Steps()) > 0 {
			go injectSteps(r, st.Steps())
		}
	}

	if err := tui.Run(ctx, fleet, cfg.TUI, nil); err != nil {
		log.Fatalf("TUI: %v", err)
	}

	for _, r := range fleet.Runners() {
		if r.Status() == agentfleet.StatusRunning {
			r.Stop() //nolint:errcheck
		}
	}
}

func loadTasks(path string) ([]agentfleet.Task, error) {
	switch {
	case strings.HasSuffix(path, ".md"):
		return (&source.MarkdownSource{Path: path}).Load()
	default:
		return (&source.FileSource{Path: path}).Load()
	}
}

// injectSteps writes timed commands to the runner's stdin.
func injectSteps(r *agentfleet.Runner, steps []source.Step) {
	w := r.StdinWriter()
	for _, s := range steps {
		select {
		case <-r.Done():
			return
		case <-time.After(time.Duration(s.Delay * float64(time.Second))):
		}
		if s.Command == "" {
			r.Stop() //nolint:errcheck
			return
		}
		fmt.Fprintf(w, "%s\r", s.Command)
	}
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}
```

- [ ] **Step 2: Build**

```bash
go build ./examples/file-manager/
```
Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add examples/file-manager/
git commit -m "feat: add examples/file-manager with step injection"
```

---

## Task 11: Create `examples/http-manager/`

**Files:**
- Create: `examples/http-manager/main.go`

- [ ] **Step 1: Create `examples/http-manager/main.go`**

```go
// http-manager demonstrates loading tasks from an HTTP endpoint.
// Run examples/taskserver first: go run ./examples/taskserver/
package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	agentfleet "github.com/hoaitan/agentfleet"
	"github.com/hoaitan/agentfleet/source"
	"github.com/hoaitan/agentfleet/tui"
)

var taskIDRe = regexp.MustCompile(`^[a-z0-9\-]+$`)

func main() {
	url := flag.String("source", "", "HTTP endpoint returning JSON task array")
	flag.Parse()
	if *url == "" {
		log.Fatal("--source <url> is required")
	}

	tasks, err := (&source.HTTPSource{URL: *url}).Load()
	if err != nil {
		log.Fatalf("load tasks: %v", err)
	}
	if len(tasks) == 0 {
		log.Fatal("task list is empty")
	}

	cfg := agentfleet.DefaultConfig()
	cfg.Agent = agentfleet.AgentConfigFromTerminal()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fleet := agentfleet.NewFleet(cfg.Fleet)

	for _, task := range tasks {
		if strings.TrimSpace(task.Command()) == "" {
			log.Fatalf("task %q has empty command", task.ID())
		}
		if !taskIDRe.MatchString(task.ID()) {
			log.Fatalf("task %q has invalid ID %q", task.Name(), task.ID())
		}

		ag := agentfleet.NewPtyAgent(agentfleet.CommandFields(task))
		r := agentfleet.NewRunner(task, ag, cfg.Fleet)
		r.Start()

		if err := fleet.Add(ctx, r); err != nil {
			log.Fatalf("add task: %v", err)
		}

		if st, ok := task.(*source.StepTask); ok && len(st.Steps()) > 0 {
			go injectSteps(r, st.Steps())
		}
	}

	if err := tui.Run(ctx, fleet, cfg.TUI, nil); err != nil {
		log.Fatalf("TUI: %v", err)
	}

	for _, r := range fleet.Runners() {
		if r.Status() == agentfleet.StatusRunning {
			r.Stop() //nolint:errcheck
		}
	}
}

func injectSteps(r *agentfleet.Runner, steps []source.Step) {
	w := r.StdinWriter()
	for _, s := range steps {
		select {
		case <-r.Done():
			return
		case <-time.After(time.Duration(s.Delay * float64(time.Second))):
		}
		if s.Command == "" {
			r.Stop() //nolint:errcheck
			return
		}
		fmt.Fprintf(w, "%s\r", s.Command)
	}
}
```

- [ ] **Step 2: Build**

```bash
go build ./examples/http-manager/
```
Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add examples/http-manager/
git commit -m "feat: add examples/http-manager"
```

---

## Task 12: Create `examples/generate-manager/`

**Files:**
- Create: `examples/generate-manager/main.go`

- [ ] **Step 1: Create `examples/generate-manager/main.go`**

```go
// generate-manager calls the Claude API to generate tasks from a natural-language goal,
// shows the generated tasks for confirmation, then runs them with the TUI.
//
// Usage:
//
//	ANTHROPIC_API_KEY=sk-... go run ./examples/generate-manager/ --generate "Run 3 coding challenges"
package main

import (
	"bufio"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"os/signal"
	"regexp"
	"strings"
	"syscall"
	"time"

	agentfleet "github.com/hoaitan/agentfleet"
	"github.com/hoaitan/agentfleet/source"
	"github.com/hoaitan/agentfleet/tui"
)

var taskIDRe = regexp.MustCompile(`^[a-z0-9\-]+$`)

func main() {
	goal := flag.String("generate", "", "natural-language goal — calls Claude to generate tasks")
	flag.Parse()
	if *goal == "" {
		log.Fatal("--generate <goal> is required")
	}

	tasks, err := source.NewGenerateSource(*goal, "", "").Load()
	if err != nil {
		log.Fatalf("generate tasks: %v", err)
	}
	if len(tasks) == 0 {
		log.Fatal("no tasks generated")
	}

	fmt.Printf("\nGenerated %d task(s):\n\n", len(tasks))
	for _, t := range tasks {
		steps := 0
		if st, ok := t.(*source.StepTask); ok {
			steps = len(st.Steps())
		}
		fmt.Printf("  [%s] %s — command: %s (%d steps)\n", t.ID(), t.Name(), t.Command(), steps)
	}
	fmt.Println()

	if !confirm("Launch these tasks? [y/N] ") {
		fmt.Println("Aborted.")
		os.Exit(0)
	}

	cfg := agentfleet.DefaultConfig()
	cfg.Agent = agentfleet.AgentConfigFromTerminal()

	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	fleet := agentfleet.NewFleet(cfg.Fleet)

	for _, task := range tasks {
		if strings.TrimSpace(task.Command()) == "" {
			log.Fatalf("task %q has empty command", task.ID())
		}
		if !taskIDRe.MatchString(task.ID()) {
			log.Fatalf("task %q has invalid ID %q", task.Name(), task.ID())
		}

		ag := agentfleet.NewPtyAgent(agentfleet.CommandFields(task))
		r := agentfleet.NewRunner(task, ag, cfg.Fleet)
		r.Start()

		if err := fleet.Add(ctx, r); err != nil {
			log.Fatalf("add task: %v", err)
		}

		if st, ok := task.(*source.StepTask); ok && len(st.Steps()) > 0 {
			go injectSteps(r, st.Steps())
		}
	}

	if err := tui.Run(ctx, fleet, cfg.TUI, nil); err != nil {
		log.Fatalf("TUI: %v", err)
	}

	for _, r := range fleet.Runners() {
		if r.Status() == agentfleet.StatusRunning {
			r.Stop() //nolint:errcheck
		}
	}
}

func injectSteps(r *agentfleet.Runner, steps []source.Step) {
	w := r.StdinWriter()
	for _, s := range steps {
		select {
		case <-r.Done():
			return
		case <-time.After(time.Duration(s.Delay * float64(time.Second))):
		}
		if s.Command == "" {
			r.Stop() //nolint:errcheck
			return
		}
		fmt.Fprintf(w, "%s\r", s.Command)
	}
}

func confirm(prompt string) bool {
	fmt.Print(prompt)
	scanner := bufio.NewScanner(os.Stdin)
	if scanner.Scan() {
		return strings.ToLower(strings.TrimSpace(scanner.Text())) == "y"
	}
	return false
}
```

- [ ] **Step 2: Build**

```bash
go build ./examples/generate-manager/
```
Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add examples/generate-manager/
git commit -m "feat: add examples/generate-manager"
```

---

## Task 13: Update `examples/taskserver/`

**Files:**
- Modify: `examples/taskserver/main.go`

- [ ] **Step 1: Update `examples/taskserver/main.go`**

Replace `internal/fleet` import with `agentfleet/source` and use `source.StepTask`:

```go
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/hoaitan/agentfleet/source"
)

var tasks = []source.StepTask{
	{
		TaskID:   "task-1",
		TaskName: "Ask today's date",
		Cmd:      "claude",
		TaskSteps: []source.Step{
			{Delay: 2, Command: "What is today's date?"},
			{Delay: 8, Command: "/exit"},
		},
	},
	{
		TaskID:   "task-2",
		TaskName: "Interactive session",
		Cmd:      "claude",
		TaskSteps: []source.Step{
			{Delay: 2, Command: "What is tomorrow's date?"},
		},
	},
}

func main() {
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	http.HandleFunc("/tasks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(tasks) //nolint:errcheck
	})

	log.Printf("task server listening on %s — GET /tasks", *addr)
	log.Fatal(http.ListenAndServe(*addr, nil))
}
```

- [ ] **Step 2: Build**

```bash
go build ./examples/taskserver/
```
Expected: compiles cleanly.

- [ ] **Step 3: Commit**

```bash
git add examples/taskserver/main.go
git commit -m "refactor: taskserver uses source.StepTask"
```

---

## Task 14: Delete Old Packages and `cmd/`

- [ ] **Step 1: Verify everything builds before deleting**

```bash
go build ./...
go test ./hook/ ./source/ . -count=1
```
Expected: all PASS

- [ ] **Step 2: Remove old internal packages**

```bash
git rm -r internal/fleet/ internal/agent/ internal/hook/ internal/source/
git rm -r cmd/
```

- [ ] **Step 3: Verify build still passes**

```bash
go build ./...
```
Expected: compiles cleanly (internal/proxy still exists and compiles).

- [ ] **Step 4: Run full test suite**

```bash
go test ./... -count=1
```
Expected: all PASS. Note: `source/generate_test.go` and `source/http_test.go` may need network; skip with `-run TestFile -run TestMarkdown` if in CI.

- [ ] **Step 5: Commit**

```bash
git commit -m "refactor: remove internal/fleet, internal/agent, internal/hook, internal/source, cmd/"
```

---

## Task 15: Update README.md and CLAUDE.md

- [ ] **Step 1: Rewrite `README.md`**

Replace the entire README to lead with library usage, then show example binaries. Key sections:
1. Install (library import + build examples from source)
2. Library quick start (import agentfleet, implement Manager)
3. Built-in examples (file-manager, http-manager, generate-manager, attach)
4. Fleet Dashboard (same ASCII art)
5. Extending (custom Manager, custom Hook, custom Task, custom Source)
6. Architecture table (updated package list)
7. License

The "Install" section should now be:
```markdown
## Install

```bash
# Build example binaries from source
git clone https://github.com/hoaitan/agentfleet
cd agentfleet
go build -o agentfleet ./examples/file-manager/
go build -o attach ./examples/attach/
```

Or use as a library:
```go
import agentfleet "github.com/hoaitan/agentfleet"
```
```

- [ ] **Step 2: Update `CLAUDE.md`**

Update the Package Layout table to match the new structure. Remove all references to `cmd/`. Update Key Interfaces section: remove `Steps()` from Task, add `Fleet`, add `Config`. Update Build and Test commands to reference `examples/` instead of `cmd/`.

- [ ] **Step 3: Build and test one final time**

```bash
go build ./...
go test ./hook/ ./source/ . -count=1 -v
```
Expected: all PASS

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: update README and CLAUDE.md for library refactor"
```

---

## Self-Review Notes

**Spec coverage check:**
- ✅ Root package: Task (no Steps), Fleet, Runner, Manager, Agent, Config
- ✅ `hook/` sub-package
- ✅ `source/` sub-package with StepTask
- ✅ `tui/` sub-package taking *Fleet + TUIConfig
- ✅ examples/: file-manager, http-manager, generate-manager, attach
- ✅ `FleetConfig.MaxConcurrent` enforced in Fleet.Add()
- ✅ `AgentConfigFromTerminal()`
- ✅ Socket server moved into Runner (SocketDir in FleetConfig)
- ✅ Step injection is example-layer only (injectSteps helper)
- ✅ Manager interface: `Run(ctx, fleet) error`
- ✅ gRPC streaming case supported by Fleet.Add(ctx, runner)
- ✅ Old packages deleted in Task 14
- ✅ README and CLAUDE.md updated in Task 15

**Type consistency:**
- `agentfleet.NewRunner(task, ag, cfg)` used consistently across Tasks 5, 10, 11, 12
- `fleet.Add(ctx, r)` used consistently across Tasks 6, 10, 11, 12
- `source.StepTask` / `source.Step` used in Tasks 7, 10, 11, 12, 13
- `tui.Run(ctx, fleet, cfg.TUI, nil)` used consistently across Tasks 8, 10, 11, 12
