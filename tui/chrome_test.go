package tui

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestIsChromeLine(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"────────────────────────────────────────────────────────────", true},
		{"━━━━━━━━━━━━━━━━━━━━━━━━", true},
		{"❯", true},
		{"❯ some text", true},
		{"✻ Sautéed for 12s", true},
		{"  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents", true},
		{"bypass permissions", true},
		{"", false},
		{"Analyzing codebase structure...", false},
		{"Writing unit tests for auth.go", false},
		{"Step 3 of 5 complete", false},
		{"─ some label ─", false}, // not a full-width divider (contains non-─ chars)
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, isChromeLine(c.in))
		})
	}
}

func TestFilterAgentChrome(t *testing.T) {
	input := []string{
		"✻ Sautéed for 12s",
		"",
		"────────────────────────────────────────────────────────────────────",
		"❯",
		"────────────────────────────────────────────────────────────────────",
		"  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents",
		"",
		"Analyzing codebase...",
		"Writing tests for auth.go",
		"",
		"",
		"Step 3 of 5 complete",
	}
	got := filterAgentChrome(input)
	assert.Equal(t, []string{
		"Analyzing codebase...",
		"Writing tests for auth.go",
		"",
		"Step 3 of 5 complete",
	}, got)
}

func TestFilterAgentChromeEmpty(t *testing.T) {
	assert.Empty(t, filterAgentChrome(nil))
	assert.Empty(t, filterAgentChrome([]string{}))
}

func TestFilterAgentChromeAllChrome(t *testing.T) {
	input := []string{
		"────────────────────────────────────",
		"❯",
		"────────────────────────────────────",
		"  ⏵⏵ bypass permissions on",
	}
	assert.Empty(t, filterAgentChrome(input))
}
