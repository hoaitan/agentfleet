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

func TestGenerateSourceParsesResponse(t *testing.T) {
	// Simulate Anthropic API response
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		assert.Equal(t, "POST", r.Method)
		assert.Equal(t, "application/json", r.Header.Get("Content-Type"))

		resp := map[string]any{
			"content": []map[string]any{
				{"type": "text", "text": `[{"id":"g1","name":"Generated","command":"claude","steps":[{"delay":2,"command":"hello"}]}]`},
			},
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(resp) //nolint:errcheck
	}))
	defer srv.Close()

	src := source.NewGenerateSource("do something cool", srv.URL, "test-key")
	tasks, err := src.Load()
	require.NoError(t, err)
	require.Len(t, tasks, 1)
	assert.Equal(t, "g1", tasks[0].ID())
	assert.Equal(t, "Generated", tasks[0].Name())
	assert.Equal(t, "claude", tasks[0].Command())

	st := tasks[0].(*source.StepTask)
	require.Len(t, st.Steps(), 1)
	assert.Equal(t, "hello", st.Steps()[0].Command)
	assert.Equal(t, 2.0, st.Steps()[0].Delay)
}

func TestGenerateSourceMissingKey(t *testing.T) {
	src := source.NewGenerateSource("goal", "https://api.anthropic.com/v1/messages", "")
	_, err := src.Load()
	require.Error(t, err)
	assert.Contains(t, err.Error(), "ANTHROPIC_API_KEY")
}
