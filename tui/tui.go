package tui

import (
	"context"
	"fmt"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"

	agentfleet "github.com/hoaitan/agentfleet"
)

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
type ctxDoneMsg struct{}

func tickCmd(d time.Duration) tea.Cmd {
	return tea.Tick(d, func(time.Time) tea.Msg { return tickMsg{} })
}

func ctxDoneCmd(ctx context.Context) tea.Cmd {
	return func() tea.Msg {
		<-ctx.Done()
		return ctxDoneMsg{}
	}
}

type model struct {
	fleet    *agentfleet.Fleet
	cfg      agentfleet.TUIConfig
	onAttach func(taskID string)
	ctx      context.Context
	cursor   int
	termW    int
	termH    int
}

// Run starts the Bubbletea TUI and blocks until the user quits or ctx is cancelled.
// onAttach is called when the user presses Enter on a running task.
// If onAttach is nil, the default behaviour opens an iTerm2 tab with the attach binary.
func Run(ctx context.Context, fleet *agentfleet.Fleet, cfg agentfleet.TUIConfig, onAttach func(taskID string)) error {
	if onAttach == nil {
		onAttach = defaultOnAttach
	}
	m := model{fleet: fleet, cfg: cfg, onAttach: onAttach, ctx: ctx}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func defaultOnAttach(taskID string) {
	attachBin, _ := filepath.Abs("./attach")
	script := fmt.Sprintf(`tell application "iTerm2"
	tell current window
		create tab with default profile command "%s %s"
	end tell
end tell`, attachBin, taskID)
	exec.Command("osascript", "-e", script).Start() //nolint:errcheck
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(m.cfg.RefreshRate), ctxDoneCmd(m.ctx))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ctxDoneMsg:
		return m, tea.Quit
	case tickMsg:
		return m, tickCmd(m.cfg.RefreshRate)
	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		return m, nil
	case tea.KeyMsg:
		runners := m.fleet.Runners()
		switch msg.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < len(runners)-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			if len(runners) > 0 && runners[m.cursor].Status() == agentfleet.StatusRunning {
				m.onAttach(runners[m.cursor].Task().ID())
			}
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		}
		switch msg.String() {
		case "j":
			if m.cursor < len(m.fleet.Runners())-1 {
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

func (m model) View() string { return renderListView(m) }

func renderHeader(m model) string {
	runners := m.fleet.Runners()
	var running, done, failed int
	for _, r := range runners {
		switch r.Status() {
		case agentfleet.StatusRunning:
			running++
		case agentfleet.StatusDone:
			done++
		case agentfleet.StatusFailed:
			failed++
		}
	}
	summary := fmt.Sprintf("%d tasks · %d running · %d done", len(runners), running, done)
	if failed > 0 {
		summary += fmt.Sprintf(" · %d failed", failed)
	}
	return styleTitle.Render("◈ agentfleet") + "  " + styleSummary.Render(summary)
}

func renderListView(m model) string {
	runners := m.fleet.Runners()
	var b strings.Builder
	b.WriteString(renderHeader(m) + "\n\n")
	for i, r := range runners {
		b.WriteString(renderCard(r, m.cfg, i == m.cursor) + "\n")
	}
	b.WriteString("\n" + styleFooter.Render("[↑↓ j/k] navigate  [enter] open tab  [q] quit"))
	return b.String()
}

func statusBadge(s agentfleet.Status) string {
	const w = 10
	switch s {
	case agentfleet.StatusRunning:
		return styleRunning.Width(w).Render("● running")
	case agentfleet.StatusDone:
		return styleDone.Width(w).Render("✓ done")
	case agentfleet.StatusFailed:
		return styleFailed.Width(w).Render("✗ failed")
	default:
		return stylePending.Width(w).Render("○ pending")
	}
}

func renderCard(r *agentfleet.Runner, cfg agentfleet.TUIConfig, selected bool) string {
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
	nameMaxW := cfg.CardWidth - cursorW - idW - 2 - badgeW - 1
	if nameMaxW < 8 {
		nameMaxW = 8
	}
	name := truncateVisual(task.Name(), nameMaxW)
	left := cursor + idStyle.Render(task.ID()) + "  " + name
	gap := cfg.CardWidth - lipgloss.Width(left) - badgeW
	if gap < 1 {
		gap = 1
	}

	var lines []string
	lines = append(lines, left+strings.Repeat(" ", gap)+badge)

	if selected {
		elapsed := elapsedStr(r.StartedAt(), r.FinishedAt())
		lines = append(lines, styleMeta.Render("  "+elapsed))

		allLines := r.Lines()
		start := len(allLines) - cfg.PreviewLines
		if start < 0 {
			start = 0
		}
		preview := allLines[start:]
		lines = append(lines, "")
		for i := 0; i < cfg.PreviewLines; i++ {
			if i < len(preview) {
				text := truncateVisual(stripANSI(preview[i]), cfg.CardWidth-4)
				lines = append(lines, styleOutput.Render("  "+text))
			} else {
				lines = append(lines, "")
			}
		}
		return cardSelStyle.Width(cfg.CardWidth).Render(strings.Join(lines, "\n"))
	}

	return cardOtherStyle.Width(cfg.CardWidth).Render(strings.Join(lines, "\n"))
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
