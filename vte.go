package agentfleet

import (
	"strings"
	"sync"

	"github.com/hinshun/vt10x"
	"github.com/hoaitan/agentfleet/hook"
)

// vteHook feeds PTY output bytes into a virtual terminal emulator so that
// control sequences (backspace, \r overwrite, cursor movement, erase-line)
// are applied before the content is surfaced for preview. Each Runner owns
// one vteHook; the virtual screen persists for the lifetime of the session,
// so switching between tasks in the TUI always shows each task's current
// rendered screen state.
type vteHook struct {
	mu   sync.Mutex
	term vt10x.Terminal
	cols int
	rows int
}

func newVTEHook(cols, rows int) *vteHook {
	return &vteHook{
		term: vt10x.New(vt10x.WithSize(cols, rows)),
		cols: cols,
		rows: rows,
	}
}

// Process implements hook.Hook. Only output bytes (PTY → reader) are fed into
// the VTE; input bytes are passed through unchanged.
func (h *vteHook) Process(p []byte, dir hook.Dir) ([]byte, error) {
	if dir == hook.DirOut {
		h.mu.Lock()
		h.term.Write(p) //nolint:errcheck
		h.mu.Unlock()
	}
	return p, nil
}

// Screen returns the current rendered screen as a slice of strings, one per
// row, with trailing spaces and blank trailing rows removed.
// vt10x.Terminal.String() acquires the internal mutex itself; we must NOT call
// h.term.Lock() here or it will deadlock with String()'s own Lock() call.
func (h *vteHook) Screen() []string {
	h.mu.Lock()
	raw := h.term.String()
	h.mu.Unlock()

	rawLines := strings.Split(raw, "\n")
	out := make([]string, 0, len(rawLines))
	for _, l := range rawLines {
		out = append(out, strings.TrimRight(l, " "))
	}
	// trim trailing blank rows
	for len(out) > 0 && out[len(out)-1] == "" {
		out = out[:len(out)-1]
	}
	return out
}

// Resize changes the emulator's dimensions so the rendered screen keeps
// mirroring the PTY after a window-size change. vt10x.Terminal.Resize acquires
// its own internal mutex, so holding h.mu here is safe (it does not re-enter
// h.mu).
func (h *vteHook) Resize(cols, rows int) {
	h.mu.Lock()
	h.term.Resize(cols, rows)
	h.cols = cols
	h.rows = rows
	h.mu.Unlock()
}
