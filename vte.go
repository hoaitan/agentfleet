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
	mu      sync.Mutex
	term    vt10x.Terminal
	cols    int
	rows    int
	pending []byte // start of an escape sequence split across Process calls
}

func newVTEHook(cols, rows int) *vteHook {
	return &vteHook{
		term: vt10x.New(vt10x.WithSize(cols, rows)),
		cols: cols,
		rows: rows,
	}
}

// Process implements hook.Hook. Only output bytes (PTY → reader) are fed into
// the VTE; input bytes are passed through unchanged. Kitty keyboard protocol
// sequences are removed from the emulator's copy only (see stripKittyKeyboard);
// the returned bytes are always p, so the real terminal still receives them.
func (h *vteHook) Process(p []byte, dir hook.Dir) ([]byte, error) {
	if dir == hook.DirOut {
		h.mu.Lock()
		h.term.Write(h.stripKittyKeyboard(p)) //nolint:errcheck
		h.mu.Unlock()
	}
	return p, nil
}

// maxPendingEscape bounds how many bytes of an unterminated private CSI
// sequence are held back waiting for its final byte. Kitty keyboard sequences
// are only a few bytes long; a longer run is not one and is released as-is.
const maxPendingEscape = 32

// stripKittyKeyboard returns p without kitty keyboard protocol sequences: CSI,
// a private prefix (? > < =), numeric parameters, and the final byte 'u'.
// Agents such as Claude Code push, pop, and query these keyboard flags around
// drawing a frame. vt10x does not implement the protocol and runs any CSI ... u
// as ANSI.SYS restore-cursor, which moves its cursor to the last saved position
// (usually the top-left corner). Every later cursor-relative redraw then lands
// on the wrong rows and Screen() no longer matches what a real terminal shows.
//
// A trailing sequence that could still turn out to be a kitty sequence is held
// in h.pending and re-examined with the next call. Caller must hold h.mu.
func (h *vteHook) stripKittyKeyboard(p []byte) []byte {
	data := p
	if len(h.pending) > 0 {
		data = append(h.pending, p...)
		h.pending = nil
	}
	var out []byte
	changed := len(data) != len(p)
	start := 0 // data[start:i] is kept but not yet copied into out
	for i := 0; i < len(data); i++ {
		if data[i] != 0x1b {
			continue
		}
		n, needMore := kittyKeyboardSeqLen(data[i:])
		if n == 0 && !needMore {
			continue
		}
		out = append(out, data[start:i]...)
		changed = true
		if needMore {
			h.pending = append([]byte(nil), data[i:]...)
			return out
		}
		i += n - 1
		start = i + 1
	}
	if !changed {
		return p
	}
	return append(out, data[start:]...)
}

// kittyKeyboardSeqLen reports the length of the kitty keyboard protocol
// sequence at the start of b (which begins with ESC), or 0 if b starts with
// something else. needMore is true when b ends before that can be decided.
func kittyKeyboardSeqLen(b []byte) (n int, needMore bool) {
	if len(b) < 3 {
		return 0, len(b) == 1 || b[1] == '['
	}
	if b[1] != '[' || !strings.ContainsRune("?><=", rune(b[2])) {
		return 0, false
	}
	j := 3
	for j < len(b) && (b[j] >= '0' && b[j] <= '9' || b[j] == ';' || b[j] == ':') {
		j++
	}
	switch {
	case j == len(b):
		return 0, len(b) < maxPendingEscape
	case b[j] == 'u':
		return j + 1, false
	default:
		return 0, false
	}
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
