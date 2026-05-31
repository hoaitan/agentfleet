package source

import "github.com/tan/agentfleet/internal/fleet"

// Source loads a list of tasks from some external system.
// Implement this interface to add custom task sources.
type Source interface {
	Load() ([]fleet.Task, error)
}
