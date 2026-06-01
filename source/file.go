package source

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"

	agentfleet "github.com/hoaitan/agentfleet"
)

// FileSource loads tasks from a local JSON or YAML file.
type FileSource struct {
	Path string
}

func (s *FileSource) Load() ([]agentfleet.Task, error) {
	data, err := os.ReadFile(s.Path)
	if err != nil {
		return nil, fmt.Errorf("read %s: %w", s.Path, err)
	}

	var raw []*StepTask
	ext := strings.ToLower(filepath.Ext(s.Path))
	switch ext {
	case ".yaml", ".yml":
		if err := yaml.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse yaml %s: %w", s.Path, err)
		}
	default:
		if err := json.Unmarshal(data, &raw); err != nil {
			return nil, fmt.Errorf("parse json %s: %w", s.Path, err)
		}
	}

	tasks := make([]agentfleet.Task, len(raw))
	for i, t := range raw {
		tasks[i] = t
	}
	return tasks, nil
}
