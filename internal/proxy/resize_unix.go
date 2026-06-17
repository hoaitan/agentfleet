//go:build !windows

package proxy

import (
	"os"
	"os/signal"
	"syscall"

	"golang.org/x/term"
)

func watchResize(fd int, ag agentProxy) func() {
	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGWINCH)
	go func() {
		for range sigCh {
			if c, r, err := term.GetSize(fd); err == nil {
				_ = ag.Resize(r, c)
			}
		}
	}()
	return func() {
		signal.Stop(sigCh)
		close(sigCh)
	}
}
