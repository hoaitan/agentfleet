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

func (f HookFunc) Process(data []byte, dir Dir) ([]byte, error) {
	return f(data, dir)
}

// Chain is an ordered list of Hooks applied in sequence.
// On error the offending hook is skipped (fail-open).
// Returning nil from any hook suppresses the remaining chain.
type Chain []Hook

func (c Chain) Process(data []byte, dir Dir) ([]byte, error) {
	for _, h := range c {
		out, err := h.Process(data, dir)
		if err != nil {
			continue // fail-open
		}
		if out == nil {
			return nil, nil // suppressed
		}
		data = out
	}
	return data, nil
}
