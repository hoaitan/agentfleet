package hook

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
	default: // drop when buffer full — never block the data path
	}
	return data, nil
}
