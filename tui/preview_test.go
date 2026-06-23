package tui

import (
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPreviewLines_CapsAtN(t *testing.T) {
	in := make([]string, 50)
	for i := range in {
		in[i] = fmt.Sprintf("line%d", i)
	}
	out := previewLines(in, 5)
	assert.Len(t, out, 5, "never more than n rows regardless of emulator height")
	assert.Equal(t, "line45", out[0])
	assert.Equal(t, "line49", out[4])
}

func TestPreviewLines_FewerThanN(t *testing.T) {
	assert.Len(t, previewLines([]string{"a", "b"}, 5), 2)
	assert.Empty(t, previewLines(nil, 5))
}
