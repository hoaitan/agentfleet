package agentfleet

import "context"

// Manager is the orchestration strategy extension point.
// Implementations load or stream tasks into a Fleet via Add().
type Manager interface {
	Run(ctx context.Context, fleet *Fleet) error
}
