package agentfleet

import (
	"bytes"
	"sync"
)

// LogBuffer is a thread-safe line-oriented ring buffer that implements io.Writer.
// Pass it to slog.NewTextHandler to capture log output; read Lines() in the TUI.
type LogBuffer struct {
	mu    sync.RWMutex
	lines []string
	max   int
	acc   []byte
}

// NewLogBuffer returns a LogBuffer keeping at most maxLines lines.
func NewLogBuffer(maxLines int) *LogBuffer {
	if maxLines <= 0 {
		maxLines = 200
	}
	return &LogBuffer{max: maxLines}
}

// Write implements io.Writer. Lines are split on '\n'; incomplete lines are
// buffered until the next Write that completes them.
func (b *LogBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.acc = append(b.acc, p...)
	for {
		idx := bytes.IndexByte(b.acc, '\n')
		if idx < 0 {
			break
		}
		line := string(b.acc[:idx])
		b.acc = b.acc[idx+1:]
		if len(b.lines) >= b.max {
			b.lines = b.lines[1:]
		}
		b.lines = append(b.lines, line)
	}
	return len(p), nil
}

// Lines returns a snapshot of buffered lines, oldest first.
func (b *LogBuffer) Lines() []string {
	b.mu.RLock()
	defer b.mu.RUnlock()
	out := make([]string, len(b.lines))
	copy(out, b.lines)
	return out
}
