package main

import (
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	"github.com/tan/agentfleet/internal/fleet"
)

const previewLines = 3
const cardWidth = 64

var ansiRe = regexp.MustCompile(
	`\x1b(?:` +
		`\][^\x07\x1b]*(?:\x07|\x1b\\)` +
		`|[@-Z\\-_]` +
		`|\[[0-?]*[ -/]*[@-~]` +
		`|[PX^_][^\x1b]*\x1b\\` +
		`)`,
)

func stripANSI(s string) string {
	s = ansiRe.ReplaceAllString(s, "")
	var b strings.Builder
	for _, r := range s {
		if r >= 0x20 || r == '\t' {
			b.WriteRune(r)
		}
	}
	return b.String()
}

var (
	styleTitle   = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c084fc"))
	styleSummary = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	styleMeta    = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	styleSelID   = lipgloss.NewStyle().Foreground(lipgloss.Color("#c084fc"))
	styleOutput  = lipgloss.NewStyle().Foreground(lipgloss.Color("#d1d5db"))
	styleFooter  = lipgloss.NewStyle().Foreground(lipgloss.Color("#4b5563"))
	styleRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ade80"))
	styleDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("#34d399"))
	styleFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
	stylePending = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))

	cardSelStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#7c3aed")).
			Background(lipgloss.Color("#1e1730"))

	cardOtherStyle = lipgloss.NewStyle().
			Border(lipgloss.RoundedBorder()).
			BorderForeground(lipgloss.Color("#374151")).
			Background(lipgloss.Color("#1a1826"))
)

type tickMsg struct{}

func tickCmd() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(time.Time) tea.Msg { return tickMsg{} })
}

// openTabFn is a package-level var so tests can replace it without invoking osascript.
var openTabFn = func(taskID string) {
	attachBin, _ := filepath.Abs("./attach")
	script := fmt.Sprintf(`tell application "iTerm2"
	tell current window
		create tab with default profile command "%s %s"
	end tell
end tell`, attachBin, taskID)
	exec.Command("osascript", "-e", script).Start() //nolint:errcheck
}

// Model is the Bubbletea model for the agentfleet TUI.
type Model struct {
	runners []*fleet.Runner
	cursor  int
	termW   int
	termH   int
}

func newModel(runners []*fleet.Runner) Model {
	return Model{runners: runners}
}

func (m Model) Init() tea.Cmd { return tickCmd() }

func (m Model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tickMsg:
		return m, tickCmd()
	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		return m, nil
	case tea.KeyMsg:
		switch msg.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(m.runners)-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			if len(m.runners) > 0 && m.runners[m.cursor].Status() == fleet.StatusRunning {
				openTabFn(m.runners[m.cursor].Task().ID())
			}
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
		switch msg.String() {
		case "j":
			if m.cursor < len(m.runners)-1 {
				m.cursor++
			}
		case "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "q":
			return m, tea.Quit
		}
	}
	return m, nil
}

func (m Model) View() string { return renderListView(m) }

func renderHeader(m Model) string {
	var running, done, failed int
	for _, r := range m.runners {
		switch r.Status() {
		case fleet.StatusRunning:
			running++
		case fleet.StatusDone:
			done++
		case fleet.StatusFailed:
			failed++
		}
	}
	summary := fmt.Sprintf("%d tasks · %d running · %d done", len(m.runners), running, done)
	if failed > 0 {
		summary += fmt.Sprintf(" · %d failed", failed)
	}
	return styleTitle.Render("◈ agentfleet") + "  " + styleSummary.Render(summary)
}

func renderListView(m Model) string {
	var b strings.Builder
	b.WriteString(renderHeader(m) + "\n\n")
	for i, r := range m.runners {
		b.WriteString(renderCard(r, i == m.cursor) + "\n")
	}
	b.WriteString("\n" + styleFooter.Render("[↑↓ j/k] navigate  [enter] open tab  [q] quit"))
	return b.String()
}

func statusBadge(s fleet.Status) string {
	const w = 10
	switch s {
	case fleet.StatusRunning:
		return styleRunning.Width(w).Render("● running")
	case fleet.StatusDone:
		return styleDone.Width(w).Render("✓ done")
	case fleet.StatusFailed:
		return styleFailed.Width(w).Render("✗ failed")
	default:
		return stylePending.Width(w).Render("○ pending")
	}
}

func renderCard(r *fleet.Runner, selected bool) string {
	cursor := "  "
	idStyle := styleMeta
	if selected {
		cursor = "▶ "
		idStyle = styleSelID
	}

	badge := statusBadge(r.Status())
	task := r.Task()
	cursorW := lipgloss.Width(cursor)
	idW := lipgloss.Width(idStyle.Render(task.ID()))
	badgeW := lipgloss.Width(badge)
	nameMaxW := cardWidth - cursorW - idW - 2 - badgeW - 1
	if nameMaxW < 8 {
		nameMaxW = 8
	}
	name := truncateVisual(task.Name(), nameMaxW)
	left := cursor + idStyle.Render(task.ID()) + "  " + name
	gap := cardWidth - lipgloss.Width(left) - badgeW
	if gap < 1 {
		gap = 1
	}

	var lines []string
	lines = append(lines, left+strings.Repeat(" ", gap)+badge)

	if selected {
		elapsed := elapsedStr(r.StartedAt(), r.FinishedAt())
		meta := elapsed
		if n := len(task.Steps()); n > 0 {
			if meta != "" {
				meta += " · "
			}
			meta += fmt.Sprintf("%d steps", n)
		}
		lines = append(lines, styleMeta.Render("  "+meta))

		allLines := r.Lines()
		start := len(allLines) - previewLines
		if start < 0 {
			start = 0
		}
		preview := allLines[start:]
		lines = append(lines, "")
		for i := 0; i < previewLines; i++ {
			if i < len(preview) {
				text := truncateVisual(stripANSI(preview[i]), cardWidth-4)
				lines = append(lines, styleOutput.Render("  "+text))
			} else {
				lines = append(lines, "")
			}
		}
		return cardSelStyle.Width(cardWidth).Render(strings.Join(lines, "\n"))
	}

	return cardOtherStyle.Width(cardWidth).Render(strings.Join(lines, "\n"))
}

func elapsedStr(start, end time.Time) string {
	if start.IsZero() {
		return ""
	}
	if end.IsZero() {
		end = time.Now()
	}
	d := end.Sub(start).Round(time.Second)
	return fmt.Sprintf("%02d:%02d elapsed", int(d.Minutes()), int(d.Seconds())%60)
}

func truncateVisual(s string, maxW int) string {
	w := 0
	runes := []rune(s)
	for i, ch := range runes {
		cw := lipgloss.Width(string(ch))
		if w+cw > maxW {
			if w+1 <= maxW {
				return string(runes[:i]) + "…"
			}
			return string(runes[:i])
		}
		w += cw
	}
	return s
}
