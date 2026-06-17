//go:build windows

package agentfleet

import (
	"errors"
	"os"
	"os/exec"
	"sync"
)

var errNotSupportedOnWindows = errors.New("PtyAgent not supported on Windows")

// PtyAgent is a stub on Windows; PTY support requires a Unix host.
type PtyAgent struct {
	cmd  *exec.Cmd
	mu   sync.RWMutex
	done chan struct{}
	cfg  AgentConfig
}

func NewPtyAgent(command []string, cfg AgentConfig) *PtyAgent {
	return &PtyAgent{
		cmd:  exec.Command(command[0], command[1:]...),
		done: make(chan struct{}),
		cfg:  cfg,
	}
}

func (a *PtyAgent) Start(_, _ int) error        { return errNotSupportedOnWindows }
func (a *PtyAgent) Write(p []byte) (int, error) { return 0, os.ErrClosed }
func (a *PtyAgent) Read(p []byte) (int, error)  { return 0, os.ErrClosed }
func (a *PtyAgent) Resize(_, _ int) error       { return errNotSupportedOnWindows }
func (a *PtyAgent) Stop() error                 { return nil }
func (a *PtyAgent) Done() <-chan struct{}       { return a.done }
