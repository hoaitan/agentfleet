package source

import (
	"encoding/json"
	"fmt"
	"net/http"
	"time"

	agentfleet "github.com/hoaitan/agentfleet"
)

var httpClient = &http.Client{Timeout: 30 * time.Second}

// HTTPSource loads tasks from a JSON HTTP endpoint.
// The endpoint must return a JSON array of StepTask-compatible objects.
type HTTPSource struct {
	URL string
}

func (s *HTTPSource) Load() ([]agentfleet.Task, error) {
	resp, err := httpClient.Get(s.URL)
	if err != nil {
		return nil, fmt.Errorf("fetch %s: %w", s.URL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("server returned %d", resp.StatusCode)
	}

	var raw []*StepTask
	if err := json.NewDecoder(resp.Body).Decode(&raw); err != nil {
		return nil, fmt.Errorf("decode response: %w", err)
	}

	tasks := make([]agentfleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
