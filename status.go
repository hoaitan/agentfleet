package agentfleet

// Status represents the lifecycle state of a Runner.
type Status int32

const (
	StatusPending Status = iota
	StatusRunning
	StatusDone
	StatusFailed
)

func (s Status) String() string {
	switch s {
	case StatusPending:
		return "pending"
	case StatusRunning:
		return "running"
	case StatusDone:
		return "done"
	case StatusFailed:
		return "failed"
	default:
		return "pending"
	}
}
