package agentfleet

import (
	"context"
	"sync"
)

// Fleet is a thread-safe, dynamic registry of Runners.
// Managers add Runners via Add(); the TUI reads via Runners().
// Fleet enforces MaxConcurrent: Add() blocks until a slot is available.
type Fleet struct {
	mu      sync.RWMutex
	runners []*Runner
	sem     chan struct{}
	wg      sync.WaitGroup
	cfg     FleetConfig
}

func NewFleet(cfg FleetConfig) *Fleet {
	max := cfg.MaxConcurrent
	if max <= 0 {
		max = 9
	}
	return &Fleet{
		cfg: cfg,
		sem: make(chan struct{}, max),
	}
}

// Add registers a Runner with the Fleet and blocks until a concurrency slot is available.
// Returns ctx.Err() if the context is cancelled while waiting.
// The Runner must already be Start()-ed before calling Add().
func (f *Fleet) Add(ctx context.Context, r *Runner) error {
	select {
	case f.sem <- struct{}{}:
	case <-ctx.Done():
		return ctx.Err()
	}
	f.wg.Add(1)
	f.mu.Lock()
	f.runners = append(f.runners, r)
	f.mu.Unlock()
	go func() {
		<-r.Done()
		<-f.sem
		f.wg.Done()
	}()
	return nil
}

// Runners returns a snapshot of all registered Runners.
func (f *Fleet) Runners() []*Runner {
	f.mu.RLock()
	defer f.mu.RUnlock()
	out := make([]*Runner, len(f.runners))
	copy(out, f.runners)
	return out
}

// Remove immediately drops the runner with the given taskID from the fleet list.
// The runner's Done() goroutine still handles the semaphore release when the
// PTY exits — callers should stop the runner separately before or after calling Remove.
func (f *Fleet) Remove(taskID string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	for i, r := range f.runners {
		if r.Task().ID() == taskID {
			f.runners = append(f.runners[:i], f.runners[i+1:]...)
			return
		}
	}
}

// Wait blocks until all Runners have completed or ctx is cancelled.
func (f *Fleet) Wait(ctx context.Context) error {
	done := make(chan struct{})
	go func() {
		f.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}
