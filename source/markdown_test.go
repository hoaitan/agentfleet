package source_test

import (
	"os"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hoaitan/agentfleet/source"
)

func writeTempFile(t *testing.T, content string) string {
	t.Helper()
	f, err := os.CreateTemp("", "agentfleet-*.md")
	require.NoError(t, err)
	_, err = f.WriteString(content)
	require.NoError(t, err)
	require.NoError(t, f.Close())
	t.Cleanup(func() { os.Remove(f.Name()) })
	return f.Name()
}

func TestMarkdownSourceSingleTask(t *testing.T) {
	path := writeTempFile(t, `## Task: Say Hello
command: claude

- delay: 2, inject: "Hello world"
- delay: 5, inject: "/exit"
`)
	src := &source.MarkdownSource{Path: path}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "say-hello", tasks[0].ID())
	assert.Equal(t, "Say Hello", tasks[0].Name())
	assert.Equal(t, "claude", tasks[0].Command())

	st := tasks[0].(*source.StepTask)
	require.Len(t, st.Steps(), 2)
	assert.Equal(t, 2.0, st.Steps()[0].Delay)
	assert.Equal(t, "Hello world", st.Steps()[0].Command)
	assert.Equal(t, 5.0, st.Steps()[1].Delay)
	assert.Equal(t, "/exit", st.Steps()[1].Command)
}

func TestMarkdownSourceMultipleTasks(t *testing.T) {
	path := writeTempFile(t, `## Task: First Task
command: claude

- delay: 1, inject: "hello"

## Task: Second Task
command: codex

- delay: 2, inject: "world"
- delay: 3, inject: ""
`)
	src := &source.MarkdownSource{Path: path}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "first-task", tasks[0].ID())
	assert.Equal(t, "codex", tasks[1].Command())

	st := tasks[1].(*source.StepTask)
	assert.Equal(t, "", st.Steps()[1].Command) // empty = stop agent
}

func TestMarkdownSourceMissingFile(t *testing.T) {
	src := &source.MarkdownSource{Path: "/tmp/does-not-exist-agentfleet.md"}
	_, err := src.Load()
	require.Error(t, err)
}
