# agentfleet Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build the `agentfleet` open-source repo — a Go tool for running a fleet of AI agent CLI sessions from a single terminal dashboard, with flexible task sources and programmatic injection.

**Architecture:** Port and rename the core packages from `claude-code-bypass` (`interceptor→hook`, `manager→fleet`), introduce a `Task` interface for extensibility, add a `source` package with HTTP/Markdown/File/Generate adapters, then wire everything into an updated `cmd/agentfleet` binary with `--source` / `--generate` flags.

**Tech Stack:** Go 1.26, `github.com/creack/pty`, `golang.org/x/term`, `golang.org/x/sys`, `github.com/charmbracelet/bubbletea`, `github.com/charmbracelet/lipgloss`, `github.com/stretchr/testify`, `gopkg.in/yaml.v3`

**Module path:** `github.com/tan/agentfleet` (update if your GitHub username differs)

**Source repo to reference:** `~/try/claude-code-bypass/` — read files from there when porting.

---

## File map

```
agentfleet/
├── go.mod
├── go.sum                          (generated)
├── .gitignore
├── README.md
├── CLAUDE.md
├── internal/
│   ├── hook/
│   │   ├── hook.go                 Hook interface, HookFunc, Chain, Dir
│   │   ├── logger.go               in-memory channel logger
│   │   ├── file_logger.go          io.Writer logger
│   │   └── hook_test.go
│   ├── agent/
│   │   ├── agent.go                Agent interface + MockAgent
│   │   ├── pty_agent.go            PtyAgent implementation
│   │   ├── pty_darwin.go           setRawPtyFd (darwin)
│   │   ├── pty_linux.go            setRawPtyFd (linux)
│   │   └── agent_test.go
│   ├── proxy/
│   │   ├── proxy.go                transparent PTY proxy
│   │   ├── termios_darwin.go       enableOutputProcessing (darwin)
│   │   ├── termios_linux.go        enableOutputProcessing (linux)
│   │   └── proxy_test.go
│   ├── fleet/
│   │   ├── task.go                 Task interface, BasicTask, Step
│   │   ├── status.go               Status type
│   │   ├── runner.go               Runner (was TaskRunner)
│   │   └── runner_test.go
│   └── source/
│       ├── source.go               Source interface
│       ├── http.go                 HTTPSource
│       ├── http_test.go
│       ├── markdown.go             MarkdownSource
│       ├── markdown_test.go
│       ├── file.go                 FileSource (JSON + YAML)
│       ├── file_test.go
│       ├── generate.go             GenerateSource (Claude API via HTTP)
│       └── generate_test.go
├── cmd/
│   ├── agentfleet/
│   │   ├── main.go                 source selection, confirmation, TUI launch
│   │   ├── tui.go                  fleet dashboard (Bubbletea)
│   │   └── socketserver.go         per-task Unix socket
│   └── attach/
│       └── main.go                 connects stdin/stdout to running agent slot
└── examples/
    └── taskserver/
        └── main.go                 example HTTP task definition server
```

---

## Task 1: Initialize the module

**Files:**
- Create: `go.mod`
- Create: `.gitignore`

- [ ] **Step 1: Write go.mod**

```
module github.com/tan/agentfleet

go 1.26.1

require (
	github.com/charmbracelet/bubbletea v1.3.10
	github.com/charmbracelet/lipgloss v1.1.0
	github.com/creack/pty v1.1.24
	github.com/stretchr/testify v1.11.1
	golang.org/x/sys v0.45.0
	golang.org/x/term v0.43.0
	gopkg.in/yaml.v3 v3.0.1
)
```

- [ ] **Step 2: Write .gitignore**

```gitignore
agentfleet
attach
/tmp/agentfleet-*.sock
*.log
```

- [ ] **Step 3: Create directory structure and run go mod tidy**

```bash
mkdir -p internal/hook internal/agent internal/proxy internal/fleet internal/source
mkdir -p cmd/agentfleet cmd/attach examples/taskserver
go mod tidy
```

Expected: go.sum is created, no errors.

- [ ] **Step 4: Commit**

```bash
git add go.mod go.sum .gitignore
git commit -m "feat: initialize module github.com/tan/agentfleet"
```

---

## Task 2: internal/hook package

**Files:**
- Create: `internal/hook/hook.go`
- Create: `internal/hook/logger.go`
- Create: `internal/hook/file_logger.go`
- Create: `internal/hook/hook_test.go`

- [ ] **Step 1: Write the failing tests**

`internal/hook/hook_test.go`:
```go
package hook_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/hook"
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

- [ ] **Step 2: Run tests to confirm they fail**

```bash
go test ./internal/hook/...
```

Expected: compile error — package does not exist yet.

- [ ] **Step 3: Write hook.go**

`internal/hook/hook.go`:
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

func (f HookFunc) Process(data []byte, dir Dir) ([]byte, error) {
	return f(data, dir)
}

// Chain is an ordered list of Hooks applied in sequence.
// On error the offending hook is skipped (fail-open).
// Returning nil from any hook suppresses the remaining chain.
type Chain []Hook

func (c Chain) Process(data []byte, dir Dir) ([]byte, error) {
	for _, h := range c {
		out, err := h.Process(data, dir)
		if err != nil {
			continue // fail-open
		}
		if out == nil {
			return nil, nil // suppressed
		}
		data = out
	}
	return data, nil
}
```

- [ ] **Step 4: Write logger.go**

`internal/hook/logger.go`:
```go
package hook

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
	default: // drop when buffer full — never block the data path
	}
	return data, nil
}
```

- [ ] **Step 5: Write file_logger.go**

`internal/hook/file_logger.go`:
```go
package hook

import (
	"fmt"
	"io"
	"sync"
)

// FileLogger writes [IN]/[OUT]-prefixed lines to an io.Writer for every chunk.
// Write errors are silently dropped so a log failure never interrupts the session.
type FileLogger struct {
	w  io.Writer
	mu sync.Mutex
}

func NewFileLogger(w io.Writer) *FileLogger {
	return &FileLogger{w: w}
}

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

- [ ] **Step 6: Run tests — all must pass**

```bash
go test ./internal/hook/... -v
```

Expected: 6 tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/hook/
git commit -m "feat: add internal/hook package (Hook interface, Chain, Logger)"
```

---

## Task 3: internal/agent package

**Files:**
- Create: `internal/agent/agent.go`
- Create: `internal/agent/pty_agent.go`
- Create: `internal/agent/pty_darwin.go`
- Create: `internal/agent/pty_linux.go`
- Create: `internal/agent/agent_test.go`

These are ported directly from `~/try/claude-code-bypass/internal/agent/` with only the module path updated.

- [ ] **Step 1: Write agent_test.go**

`internal/agent/agent_test.go`:
```go
package agent_test

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/agent"
)

func TestMockAgentRoundTrip(t *testing.T) {
	ag := agent.NewMockAgent()
	require.NoError(t, ag.Start(24, 80))

	go func() { ag.SimulateOutput([]byte("hello")) }()

	buf := make([]byte, 16)
	n, err := ag.Read(buf)
	require.NoError(t, err)
	assert.Equal(t, "hello", string(buf[:n]))
}

func TestMockAgentWrite(t *testing.T) {
	ag := agent.NewMockAgent()
	require.NoError(t, ag.Start(24, 80))

	go func() { ag.Write([]byte("input")) }() //nolint:errcheck

	got, err := ag.ReadInput(time.Second)
	require.NoError(t, err)
	assert.Equal(t, []byte("input"), got)
}

func TestMockAgentStop(t *testing.T) {
	ag := agent.NewMockAgent()
	require.NoError(t, ag.Start(24, 80))
	require.NoError(t, ag.Stop())

	select {
	case <-ag.Done():
	case <-time.After(time.Second):
		t.Fatal("Done() channel not closed after Stop()")
	}
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/agent/...
```

Expected: compile error.

- [ ] **Step 3: Write agent.go**

`internal/agent/agent.go`:
```go
package agent

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

- [ ] **Step 4: Write pty_agent.go**

`internal/agent/pty_agent.go`:
```go
package agent

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
	err := a.cmd.Process.Signal(syscall.SIGTERM)
	if err != nil {
		return a.cmd.Process.Kill()
	}
	return nil
}

func (a *PtyAgent) Done() <-chan struct{} { return a.done }
```

- [ ] **Step 5: Write pty_darwin.go**

`internal/agent/pty_darwin.go`:
```go
//go:build darwin

package agent

import "golang.org/x/sys/unix"

func setRawPtyFd(fd int) {
	t, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, t)
}
```

- [ ] **Step 6: Write pty_linux.go**

`internal/agent/pty_linux.go`:
```go
//go:build linux

package agent

import "golang.org/x/sys/unix"

func setRawPtyFd(fd int) {
	t, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return
	}
	t.Iflag &^= unix.IGNBRK | unix.BRKINT | unix.PARMRK | unix.ISTRIP |
		unix.INLCR | unix.IGNCR | unix.ICRNL | unix.IXON
	t.Oflag &^= unix.OPOST
	t.Lflag &^= unix.ECHO | unix.ECHONL | unix.ICANON | unix.ISIG | unix.IEXTEN
	t.Cflag &^= unix.CSIZE | unix.PARENB
	t.Cflag |= unix.CS8
	t.Cc[unix.VMIN] = 1
	t.Cc[unix.VTIME] = 0
	_ = unix.IoctlSetTermios(fd, unix.TCSETS, t)
}
```

- [ ] **Step 7: Run tests**

```bash
go test ./internal/agent/... -v
```

Expected: 3 tests pass (MockAgent tests only; PtyAgent needs a real terminal).

- [ ] **Step 8: Commit**

```bash
git add internal/agent/
git commit -m "feat: add internal/agent package (Agent interface, MockAgent, PtyAgent)"
```

---

## Task 4: internal/proxy package

**Files:**
- Create: `internal/proxy/proxy.go`
- Create: `internal/proxy/termios_darwin.go`
- Create: `internal/proxy/termios_linux.go`
- Create: `internal/proxy/proxy_test.go`

- [ ] **Step 1: Write proxy_test.go**

`internal/proxy/proxy_test.go`:
```go
package proxy_test

import (
	"io"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/agent"
	"github.com/tan/agentfleet/internal/hook"
	"github.com/tan/agentfleet/internal/proxy"
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
	go p.Run() //nolint:errcheck

	require.NoError(t, p.Inject([]byte("injected")))
	got, err := ag.ReadInput(time.Second)
	require.NoError(t, err)
	assert.Equal(t, []byte("injected"), got)

	ag.Stop()
	pw.Close()
}

type collectWriter struct{ data []byte }

func (c *collectWriter) Write(p []byte) (int, error) {
	c.data = append(c.data, p...)
	return len(p), nil
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/proxy/...
```

Expected: compile error.

- [ ] **Step 3: Write proxy.go**

`internal/proxy/proxy.go`:
```go
package proxy

import (
	"io"
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"

	"github.com/tan/agentfleet/internal/agent"
	"github.com/tan/agentfleet/internal/hook"
)

// Proxy is a transparent PTY proxy: bytes flow stdin→agent and agent→stdout
// through hook chains, with no modification to the user's terminal experience.
type Proxy struct {
	ag       agent.Agent
	in       io.Reader
	out      io.Writer
	inChain  hook.Chain
	outChain hook.Chain
}

func New(ag agent.Agent, in io.Reader, out io.Writer, inChain, outChain hook.Chain) *Proxy {
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
// If in is a real terminal (*os.File), Run puts it in raw mode and handles SIGWINCH.
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

- [ ] **Step 4: Write termios_darwin.go**

`internal/proxy/termios_darwin.go`:
```go
//go:build darwin

package proxy

import "golang.org/x/sys/unix"

func enableOutputProcessing(fd int) {
	cur, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}
	cur.Oflag |= unix.OPOST | unix.ONLCR
	_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, cur)
}
```

- [ ] **Step 5: Write termios_linux.go**

`internal/proxy/termios_linux.go`:
```go
//go:build linux

package proxy

import "golang.org/x/sys/unix"

func enableOutputProcessing(fd int) {
	cur, err := unix.IoctlGetTermios(fd, unix.TCGETS)
	if err != nil {
		return
	}
	cur.Oflag |= unix.OPOST | unix.ONLCR
	_ = unix.IoctlSetTermios(fd, unix.TCSETS, cur)
}
```

- [ ] **Step 6: Run tests**

```bash
go test ./internal/proxy/... -v
```

Expected: 2 tests pass.

- [ ] **Step 7: Commit**

```bash
git add internal/proxy/
git commit -m "feat: add internal/proxy package"
```

---

## Task 5: internal/fleet — Task interface and BasicTask

**Files:**
- Create: `internal/fleet/task.go`
- Create: `internal/fleet/status.go`
- Create: `internal/fleet/task_test.go`

- [ ] **Step 1: Write task_test.go**

`internal/fleet/task_test.go`:
```go
package fleet_test

import (
	"testing"

	"github.com/stretchr/testify/assert"

	"github.com/tan/agentfleet/internal/fleet"
)

func TestBasicTaskImplementsTask(t *testing.T) {
	var _ fleet.Task = &fleet.BasicTask{}
}

func TestBasicTaskAccessors(t *testing.T) {
	task := &fleet.BasicTask{
		TaskID:    "t-1",
		TaskName:  "Say Hello",
		Cmd:       "claude",
		TaskSteps: []fleet.Step{{Delay: 2, Command: "hello"}, {Delay: 5, Command: "/exit"}},
	}
	assert.Equal(t, "t-1", task.ID())
	assert.Equal(t, "Say Hello", task.Name())
	assert.Equal(t, "claude", task.Command())
	assert.Len(t, task.Steps(), 2)
	assert.Equal(t, 2.0, task.Steps()[0].Delay)
	assert.Equal(t, "hello", task.Steps()[0].Command)
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/fleet/...
```

Expected: compile error.

- [ ] **Step 3: Write task.go**

`internal/fleet/task.go`:
```go
package fleet

// Task is the core interface all task definitions must implement.
// Implement this interface to add custom fields — or embed BasicTask for the defaults.
type Task interface {
	ID()      string
	Name()    string
	Command() string // CLI binary to spawn, e.g. "claude" or "codex"
	Steps()   []Step
}

// Step is one timed injection: wait Delay seconds, then send Command to the agent.
// An empty Command string stops the agent.
type Step struct {
	Delay   float64 `json:"delay"   yaml:"delay"`
	Command string  `json:"command" yaml:"command"`
}

// BasicTask is the default Task implementation used by all built-in sources.
// Users can embed it or implement Task from scratch.
type BasicTask struct {
	TaskID    string `json:"id"      yaml:"id"`
	TaskName  string `json:"name"    yaml:"name"`
	Cmd       string `json:"command" yaml:"command"`
	TaskSteps []Step `json:"steps"   yaml:"steps"`
}

func (t *BasicTask) ID()      string { return t.TaskID }
func (t *BasicTask) Name()    string { return t.TaskName }
func (t *BasicTask) Command() string { return t.Cmd }
func (t *BasicTask) Steps()   []Step { return t.TaskSteps }
```

- [ ] **Step 4: Write status.go**

`internal/fleet/status.go`:
```go
package fleet

// Status represents the lifecycle state of a Runner.
type Status int32

const (
	StatusPending Status = iota
	StatusRunning
	StatusDone
	StatusFailed
)
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/fleet/... -v
```

Expected: 2 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/fleet/task.go internal/fleet/status.go internal/fleet/task_test.go
git commit -m "feat: add fleet.Task interface, BasicTask, Step, Status"
```

---

## Task 6: internal/fleet — Runner

**Files:**
- Create: `internal/fleet/runner.go`
- Create: `internal/fleet/runner_test.go`

- [ ] **Step 1: Write runner_test.go**

`internal/fleet/runner_test.go`:
```go
package fleet_test

import (
	"bytes"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/agent"
	"github.com/tan/agentfleet/internal/fleet"
)

func TestRunnerStartAndStop(t *testing.T) {
	ag := agent.NewMockAgent()
	task := &fleet.BasicTask{TaskID: "t1", TaskName: "Test", Cmd: "echo", TaskSteps: nil}
	r := fleet.NewRunner(task, ag)

	assert.Equal(t, fleet.StatusPending, r.Status())
	r.Start()
	assert.Equal(t, fleet.StatusRunning, r.Status())

	ag.Stop()
	select {
	case <-r.Done():
	case <-time.After(time.Second):
		t.Fatal("runner did not finish after agent stopped")
	}
	assert.Equal(t, fleet.StatusDone, r.Status())
}

func TestRunnerSetOutput(t *testing.T) {
	ag := agent.NewMockAgent()
	task := &fleet.BasicTask{TaskID: "t2", TaskName: "Out", Cmd: "echo"}
	r := fleet.NewRunner(task, ag)
	r.Start()

	var buf bytes.Buffer
	r.SetOutput(&buf)
	ag.SimulateOutput([]byte("agent says hi")) //nolint:errcheck
	time.Sleep(50 * time.Millisecond)

	assert.Contains(t, buf.String(), "agent says hi")

	ag.Stop()
	<-r.Done()
}

func TestRunnerStepInjection(t *testing.T) {
	ag := agent.NewMockAgent()
	task := &fleet.BasicTask{
		TaskID:  "t3",
		TaskName: "Steps",
		Cmd:     "echo",
		TaskSteps: []fleet.Step{
			{Delay: 0.05, Command: "step1"},
		},
	}
	r := fleet.NewRunner(task, ag)
	r.Start()

	got, err := ag.ReadInput(2 * time.Second)
	require.NoError(t, err)
	assert.Equal(t, "step1\r", string(got))

	ag.Stop()
	<-r.Done()
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/fleet/... -run TestRunner
```

Expected: compile error (Runner not defined).

- [ ] **Step 3: Write runner.go**

`internal/fleet/runner.go`:
```go
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

	"github.com/tan/agentfleet/internal/agent"
	"github.com/tan/agentfleet/internal/hook"
	"github.com/tan/agentfleet/internal/proxy"
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
func (r *Runner) Done() <-chan struct{}  { return r.done }
func (r *Runner) Lines() []string       { return r.ring.snapshot() }
func (r *Runner) Task() Task            { return r.task }
func (r *Runner) setStatus(s Status)   { r.status.Store(int32(s)) }

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
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/fleet/... -v
```

Expected: 5 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/fleet/runner.go internal/fleet/runner_test.go
git commit -m "feat: add fleet.Runner (step injection, ring buffer, switchable output)"
```

---

## Task 7: internal/source — Source interface + HTTPSource

**Files:**
- Create: `internal/source/source.go`
- Create: `internal/source/http.go`
- Create: `internal/source/http_test.go`

- [ ] **Step 1: Write http_test.go**

`internal/source/http_test.go`:
```go
package source_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/fleet"
	"github.com/tan/agentfleet/internal/source"
)

func TestHTTPSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]*fleet.BasicTask{ //nolint:errcheck
			{TaskID: "t1", TaskName: "Task One", Cmd: "claude", TaskSteps: []fleet.Step{{Delay: 1, Command: "hi"}}},
			{TaskID: "t2", TaskName: "Task Two", Cmd: "codex", TaskSteps: nil},
		})
	}))
	defer srv.Close()

	src := &source.HTTPSource{URL: srv.URL}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "t1", tasks[0].ID())
	assert.Equal(t, "Task One", tasks[0].Name())
	assert.Equal(t, "claude", tasks[0].Command())
	assert.Len(t, tasks[0].Steps(), 1)
	assert.Equal(t, "t2", tasks[1].ID())
}

func TestHTTPSourceInvalidURL(t *testing.T) {
	src := &source.HTTPSource{URL: "http://localhost:0/tasks"}
	_, err := src.Load()
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/source/...
```

Expected: compile error.

- [ ] **Step 3: Write source.go**

`internal/source/source.go`:
```go
package source

import "github.com/tan/agentfleet/internal/fleet"

// Source loads a list of tasks from some external system.
// Implement this interface to add custom task sources.
type Source interface {
	Load() ([]fleet.Task, error)
}
```

- [ ] **Step 4: Write http.go**

`internal/source/http.go`:
```go
package source

import (
	"encoding/json"
	"fmt"
	"net/http"

	"github.com/tan/agentfleet/internal/fleet"
)

// HTTPSource loads tasks from a JSON HTTP endpoint.
// The endpoint must return a JSON array matching fleet.BasicTask.
type HTTPSource struct {
	URL string
}

func (s *HTTPSource) Load() ([]fleet.Task, error) {
	resp, err := http.Get(s.URL) //nolint:noctx
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", s.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var raw []*fleet.BasicTask
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	tasks := make([]fleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/source/... -run TestHTTP -v
```

Expected: 2 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/source/source.go internal/source/http.go internal/source/http_test.go
git commit -m "feat: add source.Source interface and HTTPSource"
```

---

## Task 8: internal/source — MarkdownSource

**Files:**
- Create: `internal/source/markdown.go`
- Create: `internal/source/markdown_test.go`

Markdown format supported:
```markdown
## Task: <name>
command: <cmd>

- delay: <N>, inject: "<text>"
```

- [ ] **Step 1: Write markdown_test.go**

`internal/source/markdown_test.go`:
```go
package source_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/source"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "agentfleet-*.md")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestMarkdownSourceSingleTask(t *testing.T) {
	path := writeTempFile(t, `## Task: Say Hello
command: claude

- delay: 2, inject: "Hello world"
- delay: 5, inject: "/exit"
`)
	src := &source.MarkdownSource{Path: path}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "say-hello", tasks[0].ID())
	assert.Equal(t, "Say Hello", tasks[0].Name())
	assert.Equal(t, "claude", tasks[0].Command())
	require.Len(t, tasks[0].Steps(), 2)
	assert.Equal(t, 2.0, tasks[0].Steps()[0].Delay)
	assert.Equal(t, "Hello world", tasks[0].Steps()[0].Command)
	assert.Equal(t, 5.0, tasks[0].Steps()[1].Delay)
	assert.Equal(t, "/exit", tasks[0].Steps()[1].Command)
}

func TestMarkdownSourceMultipleTasks(t *testing.T) {
	path := writeTempFile(t, `## Task: First Task
command: claude

- delay: 1, inject: "hello"

## Task: Second Task
command: codex

- delay: 2, inject: "world"
- delay: 3, inject: ""
`)
	src := &source.MarkdownSource{Path: path}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "first-task", tasks[0].ID())
	assert.Equal(t, "codex", tasks[1].Command())
	assert.Equal(t, "", tasks[1].Steps()[1].Command) // empty = stop agent
}

func TestMarkdownSourceMissingFile(t *testing.T) {
	src := &source.MarkdownSource{Path: "/tmp/does-not-exist-agentfleet.md"}
	_, err := src.Load()
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/source/... -run TestMarkdown
```

Expected: compile error (MarkdownSource not defined).

- [ ] **Step 3: Write markdown.go**

`internal/source/markdown.go`:
```go
package source

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/tan/agentfleet/internal/fleet"
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

func (s *MarkdownSource) Load() ([]fleet.Task, error) {
	f, err := os.Open(s.Path)
	if err != nil {
		return nil, fmt.Errorf("open %s: %w", s.Path, err)
	}
	defer f.Close()

	var tasks []*fleet.BasicTask
	var current *fleet.BasicTask

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())

		if strings.HasPrefix(line, "## Task: ") {
			if current != nil {
				tasks = append(tasks, current)
			}
			name := strings.TrimPrefix(line, "## Task: ")
			current = &fleet.BasicTask{
				TaskID:   slugify(name),
				TaskName: name,
			}
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

	out := make([]fleet.Task, len(tasks))
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

// parseMarkdownStep parses "- delay: 2, inject: "Hello world""
func parseMarkdownStep(line string) (fleet.Step, error) {
	line = strings.TrimPrefix(line, "- ")
	parts := strings.SplitN(line, ", inject: ", 2)
	if len(parts) != 2 {
		return fleet.Step{}, fmt.Errorf("invalid step: %q", line)
	}
	delayStr := strings.TrimPrefix(parts[0], "delay: ")
	delay, err := strconv.ParseFloat(delayStr, 64)
	if err != nil {
		return fleet.Step{}, fmt.Errorf("invalid delay: %q", delayStr)
	}
	inject := strings.Trim(parts[1], `"`)
	return fleet.Step{Delay: delay, Command: inject}, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/source/... -run TestMarkdown -v
```

Expected: 3 tests pass.

- [ ] **Step 5: Commit**

```bash
git add internal/source/markdown.go internal/source/markdown_test.go
git commit -m "feat: add MarkdownSource task loader"
```

---

## Task 9: internal/source — FileSource (JSON + YAML)

**Files:**
- Create: `internal/source/file.go`
- Create: `internal/source/file_test.go`

- [ ] **Step 1: Write file_test.go**

`internal/source/file_test.go`:
```go
package source_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/source"
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
	assert.Len(t, tasks[0].Steps(), 1)
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
	assert.Equal(t, "codex", tasks[0].Command())
	assert.Equal(t, "summarize this", tasks[0].Steps()[0].Command)
}

func TestFileSourceMissing(t *testing.T) {
	src := &source.FileSource{Path: "/tmp/no-such-file-agentfleet.json"}
	_, err := src.Load()
	require.Error(t, err)
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/source/... -run TestFileSource
```

Expected: compile error.

- [ ] **Step 3: Write file.go**

`internal/source/file.go`:
```go
package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	"github.com/tan/agentfleet/internal/fleet"
)

// FileSource loads tasks from a local JSON or YAML file.
// File type is detected by extension: .yaml / .yml → YAML, everything else → JSON.
type FileSource struct {
	Path string
}

func (s *FileSource) Load() ([]fleet.Task, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.Path, err)
	}

	var raw []*fleet.BasicTask
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

	tasks := make([]fleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
```

- [ ] **Step 4: Run go mod tidy (yaml was indirect, now direct)**

```bash
go mod tidy
```

- [ ] **Step 5: Run tests**

```bash
go test ./internal/source/... -run TestFileSource -v
```

Expected: 3 tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/source/file.go internal/source/file_test.go go.mod go.sum
git commit -m "feat: add FileSource (JSON and YAML)"
```

---

## Task 10: internal/source — GenerateSource (Claude API)

**Files:**
- Create: `internal/source/generate.go`
- Create: `internal/source/generate_test.go`

Uses the Anthropic HTTP API directly (no extra SDK dependency). Reads `ANTHROPIC_API_KEY` from the environment.

- [ ] **Step 1: Write generate_test.go**

`internal/source/generate_test.go`:
```go
package source_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/source"
)

func TestGenerateSourceParsesResponse(t *testing.T) {
	// Simulate Anthropic API response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": `[{"id":"g1","name":"Generated","command":"claude","steps":[{"delay":2,"command":"hello"}]}]`},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	src := source.NewGenerateSource("do something cool", srv.URL, "test-key")
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "g1", tasks[0].ID())
	assert.Equal(t, "Generated", tasks[0].Name())
	assert.Equal(t, "claude", tasks[0].Command())
}

func TestGenerateSourceMissingKey(t *testing.T) {
	src := source.NewGenerateSource("goal", "https://api.anthropic.com/v1/messages", "")
	_, err := src.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY")
}
```

- [ ] **Step 2: Run tests — must fail**

```bash
go test ./internal/source/... -run TestGenerate
```

Expected: compile error.

- [ ] **Step 3: Write generate.go**

`internal/source/generate.go`:
```go
package source

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"

	"github.com/tan/agentfleet/internal/fleet"
)

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

// NewGenerateSource creates a GenerateSource. apiURL defaults to the Anthropic API;
// override it in tests. apiKey is read from ANTHROPIC_API_KEY if empty.
func NewGenerateSource(goal, apiURL, apiKey string) *GenerateSource {
	if apiURL == "" {
		apiURL = defaultAPIURL
	}
	if apiKey == "" {
		apiKey = os.Getenv("ANTHROPIC_API_KEY")
	}
	return &GenerateSource{goal: goal, apiURL: apiURL, apiKey: apiKey}
}

func (s *GenerateSource) Load() ([]fleet.Task, error) {
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

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("call API: %w", err)
	}
	defer resp.Body.Close()

	var result struct {
		Content []struct {
			Text string `json:"text"`
		} `json:"content"`
		Error struct {
			Message string `json:"message"`
		} `json:"error"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("API error: %s", result.Error.Message)
	}
	if len(result.Content) == 0 {
		return nil, fmt.Errorf("empty response from model")
	}

	var raw []*fleet.BasicTask
	if err := json.Unmarshal([]byte(result.Content[0].Text), &raw); err != nil {
		return nil, fmt.Errorf("parse generated tasks: %w", err)
	}

	tasks := make([]fleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
```

- [ ] **Step 4: Run tests**

```bash
go test ./internal/source/... -run TestGenerate -v
```

Expected: 2 tests pass (mock server + missing key check).

- [ ] **Step 5: Run all source tests**

```bash
go test ./internal/source/... -v
```

Expected: all tests pass.

- [ ] **Step 6: Commit**

```bash
git add internal/source/generate.go internal/source/generate_test.go
git commit -m "feat: add GenerateSource (LLM-generated tasks via Claude API)"
```

---

## Task 11: cmd/agentfleet

**Files:**
- Create: `cmd/agentfleet/main.go`
- Create: `cmd/agentfleet/tui.go`
- Create: `cmd/agentfleet/socketserver.go`

- [ ] **Step 1: Write socketserver.go**

`cmd/agentfleet/socketserver.go`:
```go
package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/tan/agentfleet/internal/fleet"
)

type taskSession interface {
	SetOutput(io.Writer)
	StdinWriter() io.Writer
	Done() <-chan struct{}
}

var _ taskSession = (*fleet.Runner)(nil)

func serveTask(session taskSession, id string) {
	path := "/tmp/agentfleet-" + id + ".sock"
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "socket listen %s: %v\n", path, err)
		return
	}
	defer os.Remove(path)

	var (
		connected  atomic.Bool
		activeMu   sync.Mutex
		activeConn net.Conn
	)

	go func() {
		<-session.Done()
		activeMu.Lock()
		if activeConn != nil {
			activeConn.Close()
		}
		activeMu.Unlock()
		ln.Close()
	}()

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
				session.SetOutput(io.Discard)
				connected.Store(false)
				conn.Close()
			}()
			session.SetOutput(conn)
			io.Copy(session.StdinWriter(), conn) //nolint:errcheck
		}()
	}
}
```

- [ ] **Step 2: Write tui.go**

`cmd/agentfleet/tui.go`:
```go
package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tan/agentfleet/internal/fleet"
)

const previewLines = 3
const cardWidth = 64

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

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// openTabFn is a package-level var so tests can replace it without invoking osascript.
var openTabFn = func(taskID string) {
	attachBin, _ := filepath.Abs("./attach")
	script := fmt.Sprintf(`tell application "iTerm2"
	tell current window
		create tab with default profile command "%s %s"
	end tell
end tell`, attachBin, taskID)
	exec.Command("osascript", "-e", script).Start() //nolint:errcheck
}

// Model is the Bubbletea model for the agentfleet TUI.
type Model struct {
	runners []*fleet.Runner
	cursor  int
	termW   int
	termH   int
}

func newModel(runners []*fleet.Runner) Model {
	return Model{runners: runners}
}

func (m Model) Init() tea.Cmd { return tickCmd() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.runners)-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			if len(m.runners) > 0 && m.runners[m.cursor].Status() == fleet.StatusRunning {
				openTabFn(m.runners[m.cursor].Task().ID())
			}
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
		switch msg.String() {
		case "j":
			if m.cursor < len(m.runners)-1 {
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

func (m Model) View() string { return renderListView(m) }

func renderHeader(m Model) string {
	var running, done, failed int
	for _, r := range m.runners {
		switch r.Status() {
		case fleet.StatusRunning:
			running++
		case fleet.StatusDone:
			done++
		case fleet.StatusFailed:
			failed++
		}
	}
	summary := fmt.Sprintf("%d tasks · %d running · %d done", len(m.runners), running, done)
	if failed > 0 {
		summary += fmt.Sprintf(" · %d failed", failed)
	}
	return styleTitle.Render("◈ agentfleet") + "  " + styleSummary.Render(summary)
}

func renderListView(m Model) string {
	var b strings.Builder
	b.WriteString(renderHeader(m) + "\n\n")
	for i, r := range m.runners {
		b.WriteString(renderCard(r, i == m.cursor) + "\n")
	}
	b.WriteString("\n" + styleFooter.Render("[↑↓ j/k] navigate  [enter] open tab  [q] quit"))
	return b.String()
}

func statusBadge(s fleet.Status) string {
	const w = 10
	switch s {
	case fleet.StatusRunning:
		return styleRunning.Width(w).Render("● running")
	case fleet.StatusDone:
		return styleDone.Width(w).Render("✓ done")
	case fleet.StatusFailed:
		return styleFailed.Width(w).Render("✗ failed")
	default:
		return stylePending.Width(w).Render("○ pending")
	}
}

func renderCard(r *fleet.Runner, selected bool) string {
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
	nameMaxW := cardWidth - cursorW - idW - 2 - badgeW - 1
	if nameMaxW < 8 {
		nameMaxW = 8
	}
	name := truncateVisual(task.Name(), nameMaxW)
	left := cursor + idStyle.Render(task.ID()) + "  " + name
	gap := cardWidth - lipgloss.Width(left) - badgeW
	if gap < 1 {
		gap = 1
	}

	var lines []string
	lines = append(lines, left+strings.Repeat(" ", gap)+badge)

	if selected {
		elapsed := elapsedStr(r.StartedAt(), r.FinishedAt())
		meta := elapsed
		if n := len(task.Steps()); n > 0 {
			if meta != "" {
				meta += " · "
			}
			meta += fmt.Sprintf("%d steps", n)
		}
		lines = append(lines, styleMeta.Render("  "+meta))

		allLines := r.Lines()
		start := len(allLines) - previewLines
		if start < 0 {
			start = 0
		}
		preview := allLines[start:]
		lines = append(lines, "")
		for i := 0; i < previewLines; i++ {
			if i < len(preview) {
				text := truncateVisual(stripANSI(preview[i]), cardWidth-4)
				lines = append(lines, styleOutput.Render("  "+text))
			} else {
				lines = append(lines, "")
			}
		}
		return cardSelStyle.Width(cardWidth).Render(strings.Join(lines, "\n"))
	}

	return cardOtherStyle.Width(cardWidth).Render(strings.Join(lines, "\n"))
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

- [ ] **Step 3: Write main.go**

`cmd/agentfleet/main.go`:
```go
package main

import (
	"bufio"
	"flag"
	"fmt"
	"log"
	"os"
	"strings"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/tan/agentfleet/internal/agent"
	"github.com/tan/agentfleet/internal/fleet"
	"github.com/tan/agentfleet/internal/source"
)

func main() {
	src := flag.String("source", "", "task source: http URL, .md file, .json/.yaml file")
	generate := flag.String("generate", "", "natural-language goal — calls Claude to generate tasks")
	flag.Parse()

	if *src == "" && *generate == "" {
		log.Fatal("--source <url|file> or --generate <goal> is required")
	}

	tasks, err := loadTasks(*src, *generate)
	if err != nil {
		log.Fatalf("load tasks: %v", err)
	}
	if len(tasks) == 0 {
		log.Fatal("task list is empty")
	}

	if *generate != "" {
		printGeneratedTasks(tasks)
		if !confirm("Launch these tasks? [y/N] ") {
			fmt.Println("Aborted.")
			os.Exit(0)
		}
	}

	runners := make([]*fleet.Runner, len(tasks))
	for i, task := range tasks {
		if strings.TrimSpace(task.Command()) == "" {
			log.Fatalf("task %q has empty command", task.ID())
		}
		ag := agent.NewPtyAgent(fleet.CommandFields(task))
		runners[i] = fleet.NewRunner(task, ag)
		runners[i].Start()
		go serveTask(runners[i], task.ID())
		openTabFn(task.ID())
	}

	if _, err := tea.NewProgram(newModel(runners), tea.WithAltScreen()).Run(); err != nil {
		log.Fatalf("TUI: %v", err)
	}

	for _, r := range runners {
		if r.Status() == fleet.StatusRunning {
			r.Stop() //nolint:errcheck
		}
	}
}

func loadTasks(src, generate string) ([]fleet.Task, error) {
	if generate != "" {
		return source.NewGenerateSource(generate, "", "").Load()
	}
	switch {
	case strings.HasPrefix(src, "http://"), strings.HasPrefix(src, "https://"):
		return (&source.HTTPSource{URL: src}).Load()
	case strings.HasSuffix(src, ".md"):
		return (&source.MarkdownSource{Path: src}).Load()
	default:
		return (&source.FileSource{Path: src}).Load()
	}
}

func printGeneratedTasks(tasks []fleet.Task) {
	fmt.Printf("\nGenerated %d task(s):\n\n", len(tasks))
	for _, t := range tasks {
		fmt.Printf("  [%s] %s — command: %s (%d steps)\n", t.ID(), t.Name(), t.Command(), len(t.Steps()))
	}
	fmt.Println()
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

- [ ] **Step 4: Build**

```bash
go build ./cmd/agentfleet/
```

Expected: binary `agentfleet` built with no errors.

- [ ] **Step 5: Commit**

```bash
git add cmd/agentfleet/
git commit -m "feat: add cmd/agentfleet (fleet dashboard TUI with --source and --generate flags)"
```

---

## Task 12: cmd/attach

**Files:**
- Create: `cmd/attach/main.go`

- [ ] **Step 1: Write main.go**

`cmd/attach/main.go`:
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
	// Retry for up to 3 seconds — socket may not be ready when tab opens.
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
go build ./cmd/attach/
```

Expected: binary `attach` built with no errors.

- [ ] **Step 3: Commit**

```bash
git add cmd/attach/
git commit -m "feat: add cmd/attach (connect to running agent slot)"
```

---

## Task 13: examples/taskserver

**Files:**
- Create: `examples/taskserver/main.go`

- [ ] **Step 1: Write main.go**

`examples/taskserver/main.go`:
```go
// taskserver is an example HTTP server that serves task definitions to agentfleet.
// Edit the tasks slice to define your own agent workflows.
//
// Usage:
//
//	go run ./examples/taskserver/
//	agentfleet --source http://localhost:8080/tasks
package main

import (
	"encoding/json"
	"flag"
	"log"
	"net/http"

	"github.com/tan/agentfleet/internal/fleet"
)

var tasks = []fleet.BasicTask{
	{
		TaskID:   "task-1",
		TaskName: "Ask today's date",
		Cmd:      "claude",
		TaskSteps: []fleet.Step{
			{Delay: 2, Command: "What is today's date?"},
			{Delay: 8, Command: "/exit"},
		},
	},
	{
		TaskID:   "task-2",
		TaskName: "Interactive session",
		Cmd:      "claude",
		TaskSteps: []fleet.Step{
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

Expected: builds with no errors.

- [ ] **Step 3: Commit**

```bash
git add examples/taskserver/
git commit -m "feat: add examples/taskserver (example HTTP task definition server)"
```

---

## Task 14: README.md and CLAUDE.md

**Files:**
- Create: `README.md`
- Create: `CLAUDE.md`

- [ ] **Step 1: Write README.md**

`README.md`:
````markdown
# agentfleet

Run your AI agent fleet from a single terminal dashboard.

`agentfleet` is a Go tool that launches multiple AI agent CLI sessions (Claude Code, Codex, or any interactive CLI) in parallel, injects prompts programmatically, and lets you monitor all sessions from one Bubbletea dashboard. Each running agent gets its own terminal tab (iTerm2) and a Unix socket you can attach to at any time.

## Install

```bash
go install github.com/tan/agentfleet/cmd/agentfleet@latest
go install github.com/tan/agentfleet/cmd/attach@latest
```

Or build from source:

```bash
git clone https://github.com/tan/agentfleet
cd agentfleet
go build -o agentfleet ./cmd/agentfleet/
go build -o attach ./cmd/attach/
```

## Quick start

### Tasks from a Markdown file

```bash
cat > tasks.md << 'EOF'
## Task: Summarize news
command: claude

- delay: 2, inject: "Summarize the top 5 AI news stories from this week"
- delay: 30, inject: "/exit"

## Task: Write tests
command: claude

- delay: 2, inject: "Write unit tests for internal/fleet/runner.go"
- delay: 60, inject: "/exit"
EOF

./agentfleet --source tasks.md
```

### Tasks from a JSON or YAML file

```bash
./agentfleet --source tasks.json
./agentfleet --source tasks.yaml
```

JSON format:
```json
[
  {
    "id": "task-1",
    "name": "Say hello",
    "command": "claude",
    "steps": [
      {"delay": 2, "command": "Hello! What can you help me with today?"},
      {"delay": 30, "command": "/exit"}
    ]
  }
]
```

### Tasks from an HTTP endpoint

```bash
go run ./examples/taskserver/        # serves on :8080
./agentfleet --source http://localhost:8080/tasks
```

### LLM-generated tasks

```bash
export ANTHROPIC_API_KEY=sk-ant-...
./agentfleet --generate "Research and summarize the top 3 Go web frameworks"
```

agentfleet calls Claude, shows you the generated task list, and asks for confirmation before starting.

## Fleet dashboard

```
◈ agentfleet  2 tasks · 2 running · 0 done

▶ task-1  Summarize news                         ● running
  00:43 elapsed · 2 steps

  Human: Summarize the top 5 AI news...
  Assistant: Here are the top stories...
  ▌

  task-2  Write tests                            ● running
```

Keys: `↑↓` / `j/k` navigate · `enter` open agent in new iTerm2 tab · `q` quit

### Attaching to a session

```bash
./attach task-1    # connects your terminal to the running agent
```

## Extending agentfleet

### Custom Task type

```go
type MyTask struct {
    fleet.BasicTask
    Priority int
    Tags     []string
}

// MyTask satisfies fleet.Task via embedded BasicTask.
// Add your own fields; fleet.Runner only calls the interface methods.
```

### Custom Hook (middleware)

```go
type SecretRedactor struct{}

func (s *SecretRedactor) Process(data []byte, dir hook.Dir) ([]byte, error) {
    return bytes.ReplaceAll(data, []byte("sk-ant-"), []byte("sk-***-")), nil
}
```

Wire it into a Runner by modifying `fleet.NewRunner` to accept hook chains, or wrap the proxy directly.

### Custom Source

```go
type DatabaseSource struct{ DB *sql.DB }

func (s *DatabaseSource) Load() ([]fleet.Task, error) {
    rows, err := s.DB.Query("SELECT id, name, command FROM tasks WHERE active = true")
    // ... scan rows into []fleet.BasicTask
}
```

## Architecture

```
cmd/agentfleet/          CLI + Bubbletea fleet dashboard
cmd/attach/              attach stdin/stdout to a running agent slot

internal/fleet/          Task interface, BasicTask, Runner (step injection, ring buffer)
internal/source/         Source adapters: HTTP, Markdown, File (JSON/YAML), Generate (LLM)
internal/proxy/          transparent PTY proxy (stdin→agent, agent→stdout, with hook chains)
internal/agent/          Agent interface + PtyAgent (any CLI in a PTY)
internal/hook/           Hook interface, HookFunc adapter, Chain, Logger, FileLogger

examples/taskserver/     example HTTP server serving task definitions
```

## License

MIT
````

- [ ] **Step 2: Write CLAUDE.md**

`CLAUDE.md`:
```markdown
# agentfleet

## What this is

A Go tool for running a fleet of AI agent CLI sessions in parallel from a single terminal dashboard. Users define tasks (sequences of timed injections into a CLI process), and agentfleet runs them all concurrently, showing live status in a Bubbletea TUI.

## Module

`github.com/tan/agentfleet`

## Package layout

| Package | Responsibility |
|---------|----------------|
| `internal/fleet` | `Task` interface, `BasicTask`, `Step`, `Runner`, `Status` |
| `internal/source` | `Source` interface + `HTTPSource`, `MarkdownSource`, `FileSource`, `GenerateSource` |
| `internal/proxy` | transparent PTY proxy with `Hook` chains and `Inject()` |
| `internal/agent` | `Agent` interface + `PtyAgent` (wraps any CLI in a PTY) |
| `internal/hook` | `Hook` interface, `HookFunc`, `Chain`, `Logger`, `FileLogger` |
| `cmd/agentfleet` | main binary — source selection, confirmation, TUI launch |
| `cmd/attach` | connect to `/tmp/agentfleet-<id>.sock` to attach terminal to running agent |
| `examples/taskserver` | example HTTP server for task definitions |

## Key interfaces

```go
// fleet.Task — implement to add custom fields
type Task interface {
    ID() string; Name() string; Command() string; Steps() []Step
}

// source.Source — implement to add custom task sources
type Source interface {
    Load() ([]fleet.Task, error)
}

// hook.Hook — implement to intercept/transform bytes
type Hook interface {
    Process(data []byte, dir Dir) ([]byte, error)
}
```

## Naming conventions

- Packages: lowercase single word (`fleet`, `hook`, `source`, `proxy`, `agent`)
- Exported types: `PascalCase`
- Socket files: `/tmp/agentfleet-<task-id>.sock`
- Log files: `/tmp/agentfleet-<task-id>.log`

## Testing

```bash
go test ./...
go build ./cmd/agentfleet/ ./cmd/attach/
```

Integration tests (PtyAgent) require a real terminal; skip in CI with `-short`.

## Source format (Markdown)

```markdown
## Task: <name>
command: <cli>

- delay: <seconds>, inject: "<text>"
```

## Source format (JSON/YAML)

```json
[{"id":"t1","name":"...","command":"claude","steps":[{"delay":2,"command":"..."}]}]
```
```

- [ ] **Step 3: Build everything and run all tests**

```bash
go test ./...
go build ./cmd/agentfleet/ ./cmd/attach/
```

Expected: all tests pass, both binaries build.

- [ ] **Step 4: Commit**

```bash
git add README.md CLAUDE.md
git commit -m "docs: add README and CLAUDE.md"
```

---

## Final verification

- [ ] **Run full test suite**

```bash
go test ./... -v
```

Expected: all tests pass, no compilation errors.

- [ ] **Build both binaries**

```bash
go build ./cmd/agentfleet/
go build ./cmd/attach/
```

Expected: `agentfleet` and `attach` binaries created.

- [ ] **Verify go.mod is tidy**

```bash
go mod tidy
git diff go.mod go.sum
```

Expected: no changes (already tidy).

- [ ] **Final commit and push**

```bash
git status
git push -u origin feat/initial-agentfleet
```
