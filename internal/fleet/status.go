package fleet

// Status represents the lifecycle state of a Runner.
type Status int32

const (
	StatusPending Status = iota
	StatusRunning
	StatusDone
	StatusFailed
)
