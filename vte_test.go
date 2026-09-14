package agentfleet

import (
	"fmt"
	"strings"
	"testing"

	"github.com/hoaitan/agentfleet/hook"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// Resizing the emulator changes its effective width: at 10 cols a 20-char line
// wraps; after widening to 20 cols the same 20 chars fit on one row.
func TestVTEHookResizeChangesWidth(t *testing.T) {
	h := newVTEHook(10, 4) // cols=10, rows=4

	_, err := h.Process([]byte("abcdefghijKLMNO"), hook.DirOut) // 15 chars > 10
	require.NoError(t, err)
	got := h.Screen()
	require.GreaterOrEqual(t, len(got), 2, "15 chars must wrap at width 10")
	assert.Equal(t, "abcdefghij", got[0])

	h.Resize(20, 4)                                                              // widen to 20 cols
	_, err = h.Process([]byte("\x1b[2J\x1b[Habcdefghijklmnopqrst"), hook.DirOut) // clear, home, 20 chars
	require.NoError(t, err)
	got = h.Screen()
	require.NotEmpty(t, got)
	assert.Equal(t, "abcdefghijklmnopqrst", got[0], "20 chars must fit on one row at width 20")
}

// claudeTrustFrame and claudeTrustFocusDown reproduce, in miniature, the bytes
// Claude Code (2.1.269+) writes for its startup trust dialog: a first frame that
// parks the cursor on the focused option's marker cell and then sends terminal
// queries, including the kitty keyboard protocol query substituted at %s,
// followed by the purely cursor-relative redraw emitted when focus moves down.
const (
	claudeTrustFrame = "\x1b[?2026h" +
		"\r\nAccessing workspace:\r\n\r\n" +
		"\x1b[2G❯\x1b[4GNo, exit\r\n" +
		"\x1b[4GYes, I trust this folder\r\n\r\n" +
		"\x1b[2GEnter to confirm\r\n" +
		"\x1b[1C\x1b[4A\x1b[?2026l\x1b[>0q%s\x1b[c"
	claudeTrustFocusDown = "\x1b[?2026h\x1b[1D\x1b[4B\r\x1b[1C\x1b[4A \x1b[4GNo, exit" +
		"\r\x1b[1C\x1b[1B❯\x1b[4GYes, I trust this folder\r\n\n\n\x1b[1C\x1b[3A\x1b[?2026l"
)

func screenRow(lines []string, i int) string {
	if i < len(lines) {
		return lines[i]
	}
	return ""
}

// assertFocusMovedDown checks the redraw landed on the dialog's own rows. A
// cursor teleported to the top-left instead paints a copy of the menu over
// rows 0-1 and leaves the real rows unchanged.
func assertFocusMovedDown(t *testing.T, got []string) {
	t.Helper()
	assert.Equal(t, "", screenRow(got, 0), "redraw must not be painted over the top rows")
	assert.Equal(t, "Accessing workspace:", screenRow(got, 1))
	assert.Equal(t, "   No, exit", screenRow(got, 3))
	assert.Equal(t, " ❯ Yes, I trust this folder", screenRow(got, 4))
}

// Kitty keyboard protocol sequences end in "u" behind a private prefix. The
// emulator does not implement the protocol and would otherwise run them as
// ANSI.SYS restore-cursor, desynchronizing every later cursor-relative redraw.
func TestVTEHookIgnoresKittyKeyboardSequences(t *testing.T) {
	for _, seq := range []string{"\x1b[?u", "\x1b[>1u", "\x1b[<u", "\x1b[=1;1u"} {
		t.Run(fmt.Sprintf("%q", seq), func(t *testing.T) {
			h := newVTEHook(40, 10)
			_, err := h.Process([]byte(fmt.Sprintf(claudeTrustFrame, seq)), hook.DirOut)
			require.NoError(t, err)
			_, err = h.Process([]byte(claudeTrustFocusDown), hook.DirOut)
			require.NoError(t, err)

			assertFocusMovedDown(t, h.Screen())
		})
	}
}

// PTY reads can split an escape sequence across Process calls: the kitty query
// arrives one byte per call, and a private CSI that is not a kitty sequence is
// split mid-parameters. Splits stay on ASCII boundaries because vt10x decodes
// UTF-8 per Write and would garble a multi-byte glyph split the same way.
func TestVTEHookIgnoresKittyQuerySplitAcrossWrites(t *testing.T) {
	h := newVTEHook(40, 10)
	frame := fmt.Sprintf(claudeTrustFrame, "\x1b[?u")
	at := strings.Index(frame, "\x1b[?u")
	require.GreaterOrEqual(t, at, 0)
	chunks := []string{
		frame[:at], "\x1b", "[", "?", "u", frame[at+len("\x1b[?u"):],
		claudeTrustFocusDown[:5], claudeTrustFocusDown[5:9], claudeTrustFocusDown[9:],
	}
	require.Equal(t, fmt.Sprintf(claudeTrustFrame, "\x1b[?u")+claudeTrustFocusDown, strings.Join(chunks, ""))
	for _, c := range chunks {
		_, err := h.Process([]byte(c), hook.DirOut)
		require.NoError(t, err)
	}

	assertFocusMovedDown(t, h.Screen())
}

func TestVTEHookKeepsANSISaveRestoreCursor(t *testing.T) {
	h := newVTEHook(20, 5)
	_, err := h.Process([]byte("\x1b[3;5H\x1b[s\x1b[1;1H\x1b[uX"), hook.DirOut)
	require.NoError(t, err)

	assert.Equal(t, "    X", screenRow(h.Screen(), 2), "plain CSI s / CSI u still save and restore the cursor")
}

// Only the emulator's copy is filtered: the bytes forwarded to the real
// terminal must include the kitty query so it can answer.
func TestVTEHookProcessPassesOutputThroughUnchanged(t *testing.T) {
	h := newVTEHook(40, 10)
	in := []byte(fmt.Sprintf(claudeTrustFrame, "\x1b[?u"))
	out, err := h.Process(in, hook.DirOut)
	require.NoError(t, err)

	assert.Equal(t, string(in), string(out))
}
