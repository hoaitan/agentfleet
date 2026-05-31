package source_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/hoaitan/agentfleet/source"
)

func TestHTTPSource(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		json.NewEncoder(w).Encode([]*source.StepTask{ //nolint:errcheck
			{TaskID: "t1", TaskName: "Task One", Cmd: "claude", TaskSteps: []source.Step{{Delay: 1, Command: "hi"}}},
			{TaskID: "t2", TaskName: "Task Two", Cmd: "codex", TaskSteps: nil},
		})
	}))
	defer srv.Close()

	src := &source.HTTPSource{URL: srv.URL}
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 2)
	assert.Equal(t, "t1", tasks[0].ID())
	assert.Equal(t, "Task One", tasks[0].Name())
	assert.Equal(t, "claude", tasks[0].Command())

	st := tasks[0].(*source.StepTask)
	assert.Len(t, st.Steps(), 1)
	assert.Equal(t, "t2", tasks[1].ID())
}

func TestHTTPSourceInvalidURL(t *testing.T) {
	src := &source.HTTPSource{URL: "http://localhost:0/tasks"}
	_, err := src.Load()
	require.Error(t, err)
}
