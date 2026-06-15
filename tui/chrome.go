package tui

import "strings"

// filterAgentChrome removes AI agent TUI shell artifacts from raw PTY output lines.
// Filters full-width dividers, input prompts, permission banners, and processing
// indicators emitted by agent shells (e.g. Claude Code) that add no value in a preview.
func filterAgentChrome(lines []string) []string {
	out := make([]string, 0, len(lines))
	prevBlank := false
	for _, l := range lines {
		s := strings.TrimSpace(stripANSI(l))
		if isChromeLine(s) {
			continue
		}
		blank := s == ""
		if blank && prevBlank {
			continue // collapse consecutive blank lines
		}
		prevBlank = blank
		out = append(out, l)
	}
	// trim leading/trailing blank lines
	for len(out) > 0 && strings.TrimSpace(out[0]) == "" {
		out = out[1:]
	}
	for len(out) > 0 && strings.TrimSpace(out[len(out)-1]) == "" {
		out = out[:len(out)-1]
	}
	return out
}

func isChromeLine(s string) bool {
	if s == "" {
		return false
	}
	// Full-width horizontal rules (─ U+2500, ━ U+2501)
	if strings.Trim(s, "─") == "" || strings.Trim(s, "━") == "" {
		return true
	}
	// Input prompt line
	if s == "❯" || strings.HasPrefix(s, "❯ ") {
		return true
	}
	// Processing / thinking indicator (e.g. "✻ Sautéed for 12s")
	if strings.HasPrefix(s, "✻") {
		return true
	}
	// Permission banner / mode line
	if strings.Contains(s, "bypass permissions") ||
		strings.Contains(s, "shift+tab to cycle") ||
		strings.Contains(s, "← for agents") ||
		strings.Contains(s, "⏵⏵") {
		return true
	}
	return false
}
