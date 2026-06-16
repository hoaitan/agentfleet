package tui

import (
	"strings"
	"unicode"
)

// FilterAgentChrome is the exported form of filterAgentChrome for use by callers
// that compose their own FilterLines pipeline.
func FilterAgentChrome(lines []string) []string { return filterAgentChrome(lines) }

// StripANSI is the exported form of stripANSI for use by callers.
func StripANSI(s string) string { return stripANSI(s) }

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
	// Full-width horizontal rules: em-dash (─ U+2500, ━ U+2501) or ASCII hyphens (---)
	if strings.Trim(s, "─") == "" || strings.Trim(s, "━") == "" {
		return true
	}
	if strings.Trim(s, "-") == "" && len(s) >= 3 {
		return true
	}
	// Input prompt line
	if s == "❯" || strings.HasPrefix(s, "❯ ") {
		return true
	}
	// Processing / thinking indicators (various Unicode symbols used by agent shells)
	if strings.HasPrefix(s, "✻") || strings.HasPrefix(s, "⭑") ||
		strings.HasPrefix(s, "✦") || strings.HasPrefix(s, "⭐") {
		return true
	}
	// Token/time counter standalone: "(16s·+842tokens)" or "(2s · ↓1 tokens)"
	if strings.HasPrefix(s, "(") && strings.HasSuffix(s, "tokens)") {
		return true
	}
	// Active tool spinner: "● Analyzing… (18s · ↓5 tokens)"
	// The ● prefix + token suffix is the Claude Code live-update status line.
	if strings.HasPrefix(s, "●") && strings.Contains(s, "tokens)") {
		return true
	}
	// Completed tool timing: "• Sautéed for 19s" — single verb + duration
	if strings.HasPrefix(s, "• ") && isToolTiming(strings.TrimPrefix(s, "• ")) {
		return true
	}
	// Tip banners emitted inline with spinners: "| Tip: ..."
	if strings.Contains(s, "| Tip:") {
		return true
	}
	// Permission banner / mode line
	if strings.Contains(s, "bypass permissions") ||
		strings.Contains(s, "shift+tab to cycle") ||
		strings.Contains(s, "← for agents") ||
		strings.Contains(s, "⏵⏵") {
		return true
	}
	// Bare thinking verb: single capitalized word ending with "..." or "…"
	// e.g. "Galloping...", "Analyzing…", "Loading..."
	if isThinkingVerb(s) {
		return true
	}
	return false
}

// isToolTiming returns true for "Sautéed for 19s" — a single capitalised verb
// followed by "for <duration>" emitted by Claude Code after a tool completes.
func isToolTiming(s string) bool {
	parts := strings.Fields(s)
	if len(parts) < 3 {
		return false
	}
	runes := []rune(parts[0])
	return unicode.IsUpper(runes[0]) &&
		parts[1] == "for" &&
		len(parts[2]) > 1 && parts[2][len(parts[2])-1] == 's'
}

// isThinkingVerb returns true for lines like "Galloping..." — a single
// CamelCase word followed by an ASCII or Unicode ellipsis. These are emitted
// by agent shells as progress indicators and add no value in a preview.
func isThinkingVerb(s string) bool {
	suffix := ""
	switch {
	case strings.HasSuffix(s, "..."):
		suffix = "..."
	case strings.HasSuffix(s, "…"):
		suffix = "…"
	default:
		return false
	}
	word := strings.TrimSuffix(s, suffix)
	if len(word) == 0 || strings.ContainsAny(word, " \t") {
		return false // multi-word line — probably real content
	}
	runes := []rune(word)
	return unicode.IsUpper(runes[0])
}
