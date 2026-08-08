package tui

import (
	"strings"
	"testing"

	"github.com/charmbracelet/lipgloss"
	"github.com/stretchr/testify/assert"

	agentfleet "github.com/hoaitan/agentfleet"
)

func TestLogLabelWithoutPath(t *testing.T) {
	cfg := agentfleet.TUIConfig{ShowLogPath: true}
	assert.Equal(t, " Logs ", logLabel(cfg, 120))
}

func TestLogLabelPathDisplayDisabled(t *testing.T) {
	cfg := agentfleet.TUIConfig{ShowLogPath: false, LogPath: "/work/retask.log"}
	assert.Equal(t, " Logs ", logLabel(cfg, 120))
}

func TestLogLabelShowsPath(t *testing.T) {
	cfg := agentfleet.TUIConfig{ShowLogPath: true, LogPath: "/work/session-1/retask.log"}
	assert.Equal(t, " Logs (/work/session-1/retask.log) ", logLabel(cfg, 120))
}

func TestLogLabelElidesLongPathFromTheLeft(t *testing.T) {
	path := "/very/deeply/nested/workspace/session-abc/retask.log"
	cfg := agentfleet.TUIConfig{ShowLogPath: true, LogPath: path}

	label := logLabel(cfg, 40)

	assert.LessOrEqual(t, lipgloss.Width(label), 38, "label must leave room for both dashes")
	assert.True(t, strings.HasPrefix(label, " Logs (…"), "elided from the left, got %q", label)
	assert.True(t, strings.HasSuffix(label, "retask.log) "), "keeps the file name, got %q", label)
}

func TestLogLabelDropsPathOnNarrowTerminal(t *testing.T) {
	cfg := agentfleet.TUIConfig{ShowLogPath: true, LogPath: "/work/session-1/retask.log"}
	assert.Equal(t, " Logs ", logLabel(cfg, 16))
}

func TestElideLeftKeepsShortStrings(t *testing.T) {
	assert.Equal(t, "retask.log", elideLeft("retask.log", 20))
	assert.Equal(t, "retask.log", elideLeft("retask.log", 10))
}

func TestElideLeftFitsWidth(t *testing.T) {
	got := elideLeft("/a/b/c/retask.log", 12)
	assert.Equal(t, 12, lipgloss.Width(got))
	assert.Equal(t, "…/retask.log", got)
}

func TestRenderLogDividerFitsTerminalWidth(t *testing.T) {
	buf := agentfleet.NewLogBuffer(10)
	_, _ = buf.Write([]byte("hello\n"))
	m := model{
		termW: 60,
		cfg: agentfleet.TUIConfig{
			Log:         buf,
			LogPath:     "/very/deeply/nested/workspace/session-abc/retask.log",
			ShowLogPath: true,
		},
	}

	rows := strings.Split(renderLog(m, 4, ""), "\n")

	assert.Len(t, rows, 4)
	assert.Equal(t, 60, lipgloss.Width(rows[0]), "divider spans exactly the terminal width")
	assert.Contains(t, rows[0], "retask.log")
}
