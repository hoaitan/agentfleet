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
	cfg  AgentConfig
}

func NewPtyAgent(command []string, cfg AgentConfig) *PtyAgent {
	return &PtyAgent{
		cmd:  exec.Command(command[0], command[1:]...),
		done: make(chan struct{}),
		cfg:  cfg,
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

	if len(a.cfg.Env) > 0 {
		a.cmd.Env = append(os.Environ(), a.cfg.Env...)
	}

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
