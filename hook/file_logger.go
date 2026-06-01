package hook

import (
	"fmt"
	"io"
	"sync"
)

// FileLogger writes [IN]/[OUT]-prefixed lines to an io.Writer for every chunk.
type FileLogger struct {
	w  io.Writer
	mu sync.Mutex
}

func NewFileLogger(w io.Writer) *FileLogger { return &FileLogger{w: w} }

func (l *FileLogger) Process(data []byte, dir Dir) ([]byte, error) {
	prefix := "[IN]"
	if dir == DirOut {
		prefix = "[OUT]"
	}
	l.mu.Lock()
	fmt.Fprintf(l.w, "%s %s\n", prefix, data) //nolint:errcheck
	l.mu.Unlock()
	return data, nil
}
