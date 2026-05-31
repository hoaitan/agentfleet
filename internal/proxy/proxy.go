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
