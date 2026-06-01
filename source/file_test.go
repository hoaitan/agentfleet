package source_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hoaitan/agentfleet/source"
)

func TestFileSourceJSON(t *testing.T) {
	f, _ := os.CreateTemp("", "*.json")
	f.WriteString(`[
		{"id":"t1","name":"JSON Task","command":"claude","steps":[{"delay":1,"command":"hello"}]}
	]`)
	f.Close()
	defer os.Remove(f.Name())

	src := &source.FileSource{Path: f.Name()}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "t1", tasks[0].ID())
	assert.Equal(t, "JSON Task", tasks[0].Name())

	st, ok := tasks[0].(*source.StepTask)
	require.True(t, ok, "expected *source.StepTask")
	require.Len(t, st.Steps(), 1)
	assert.Equal(t, "hello", st.Steps()[0].Command)
	assert.Equal(t, 1.0, st.Steps()[0].Delay)
}

func TestFileSourceYAML(t *testing.T) {
	f, _ := os.CreateTemp("", "*.yaml")
	f.WriteString(`- id: y1
  name: YAML Task
  command: codex
  steps:
    - delay: 2
      command: "summarize this"
`)
	f.Close()
	defer os.Remove(f.Name())

	src := &source.FileSource{Path: f.Name()}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "y1", tasks[0].ID())
	assert.Equal(t, "codex", tasks[0].Command())

	st := tasks[0].(*source.StepTask)
	assert.Equal(t, "summarize this", st.Steps()[0].Command)
	assert.Equal(t, 2.0, st.Steps()[0].Delay)
}

func TestFileSourceMissing(t *testing.T) {
	src := &source.FileSource{Path: "/tmp/no-such-file-agentfleet.json"}
	_, err := src.Load()
	require.Error(t, err)
}
