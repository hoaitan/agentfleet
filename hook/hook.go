package hook

// Dir indicates byte flow direction.
type Dir uint8

const (
	DirIn  Dir = iota // user / wrapper → agent
	DirOut            // agent → display
)

// Hook processes a byte slice in a given direction.
// Returning nil bytes suppresses the data.
// Returning a non-nil error causes the chain to skip this hook (fail-open).
type Hook interface {
	Process(data []byte, dir Dir) ([]byte, error)
}

// HookFunc adapts a function to the Hook interface.
type HookFunc func([]byte, Dir) ([]byte, error)

func (f HookFunc) Process(data []byte, dir Dir) ([]byte, error) { return f(data, dir) }

// Chain is an ordered list of Hooks applied in sequence.
type Chain []Hook

func (c Chain) Process(data []byte, dir Dir) ([]byte, error) {
	for _, h := range c {
		out, err := h.Process(data, dir)
		if err != nil {
			continue
		}
		if out == nil {
			return nil, nil
		}
		data = out
	}
	return data, nil
}

// Event is one intercepted byte slice snapshot.
type Event struct {
	Dir  Dir
	Data []byte
}

// Logger records intercepted bytes to a non-blocking channel.
type Logger struct {
	Events chan Event
}

func NewLogger(bufSize int) *Logger {
	return &Logger{Events: make(chan Event, bufSize)}
}

func (l *Logger) Process(data []byte, dir Dir) ([]byte, error) {
	cp := make([]byte, len(data))
	copy(cp, data)
	select {
	case l.Events <- Event{Dir: dir, Data: cp}:
	default:
	}
	return data, nil
}
