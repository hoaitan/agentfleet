package tui

import agentfleet "github.com/hoaitan/agentfleet"

// orderedRunners splits a fleet snapshot into:
//
//	active — running+pending, newest first (last in fleet = first in result)
//	done   — done+failed, newest first, capped at maxDone (0 = no limit)
func orderedRunners(runners []*agentfleet.Runner, maxDone int) (active, done []*agentfleet.Runner) {
	for i := len(runners) - 1; i >= 0; i-- {
		r := runners[i]
		switch r.Status() {
		case agentfleet.StatusRunning, agentfleet.StatusPending:
			active = append(active, r)
		default:
			done = append(done, r)
		}
	}
	if maxDone > 0 && len(done) > maxDone {
		done = done[:maxDone]
	}
	return
}
