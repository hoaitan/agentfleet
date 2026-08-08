package agentfleet

import (
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
