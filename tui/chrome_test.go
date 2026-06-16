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
		{"---", true},
		{"----------", true},
		{"❯", true},
		{"❯ some text", true},
		{"✻ Sautéed for 12s", true},
		{"⭑Garnishing…(16s·+842tokens)", true},
		{"✦ thinking", true},
		{"(16s·+842tokens)", true},
		{"(2s · ↓1 tokens)", true},
		{"  ⏵⏵ bypass permissions on (shift+tab to cycle) · ← for agents", true},
		{"bypass permissions", true},
		{"", false},
		{"--", false}, // too short to be a divider
		{"Analyzing codebase structure...", false},
		{"Writing unit tests for auth.go", false},
		{"Step 3 of 5 complete", false},
		{"─ some label ─", false},          // not a full-width divider (contains non-─ chars)
		{"(cost 0.5 tokens saved)", false}, // doesn't end with "tokens)"
		{"Galloping...", true},
		{"Analyzing…", true},
		{"Loading...", true},
		{"Analyzing codebase...", false}, // multi-word
		{"running...", false},            // lowercase first letter
		// Active tool spinner (● prefix + token suffix)
		{"● Scampering… (18s · ↓23 tokens)", true},
		{"● Analyzing… (5s · +100 tokens)", true},
		{"● Running tool (2s · tokens)", true},
		{"● I'll use brainstorming to understand", false}, // real content, no tokens)
		// Completed tool timing (• Verb for Ns)
		{"• Sautéed for 19s", true},
		{"• Galloping for 3s", true},
		{"• I'll use brainstorming to understand what kind of story", false}, // real content
		// Tip banners
		{"● Scampering… (18s · ↓23 tokens) | Tip: Send messages to Claude", true},
		{"| Tip: You can press Escape to interrupt", true},
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
