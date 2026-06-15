package tui

import (
	"context"
	"fmt"
	"os"
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
	styleLog     = lipgloss.NewStyle().Foreground(lipgloss.Color("#4b5563"))
	styleRunning = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ade80"))
	styleDone    = lipgloss.NewStyle().Foreground(lipgloss.Color("#34d399"))
	styleFailed  = lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
	stylePending = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	styleSel     = lipgloss.NewStyle().Background(lipgloss.Color("#1e1730"))
	styleDivider = lipgloss.NewStyle().Foreground(lipgloss.Color("#374151"))
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
	fleet      *agentfleet.Fleet
	cfg        agentfleet.TUIConfig
	onAttach   func(taskID string)
	ctx        context.Context
	cursor     int
	termW      int
	termH      int
	openedTabs map[string]bool

	listOffset    int // first visible visual row in task list
	outScrollBack int // 0 = snapped to bottom; N = scrolled N lines up from bottom
}

// Run starts the Bubbletea TUI and blocks until the user quits or ctx is cancelled.
// onAttach is called when the user presses Enter on a running task.
// If onAttach is nil, the default behaviour opens an iTerm2 tab with the attach binary.
func Run(ctx context.Context, fleet *agentfleet.Fleet, cfg agentfleet.TUIConfig, onAttach func(taskID string)) error {
	if onAttach == nil {
		onAttach = defaultOnAttach
	}
	m := model{fleet: fleet, cfg: cfg, onAttach: onAttach, ctx: ctx, openedTabs: make(map[string]bool)}
	_, err := tea.NewProgram(m, tea.WithAltScreen()).Run()
	return err
}

func defaultOnAttach(taskID string) {
	attachBin, _ := filepath.Abs("./attach")
	OpenInTerminal(attachBin, taskID)
}

// OpenInTerminal opens a new terminal tab/window running the given command.
// Each element of cmd is a separate argument (e.g. OpenInTerminal("retask", "sandbox", "attach", id)).
func OpenInTerminal(cmd ...string) {
	if len(cmd) == 0 {
		return
	}
	cmdStr := strings.Join(cmd, " ")
	if os.Getenv("TMUX") != "" {
		exec.Command("tmux", append([]string{"new-window"}, cmd...)...).Start() //nolint:errcheck
		return
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app":
		script := fmt.Sprintf("tell application \"iTerm2\"\ntell current window\ncreate tab with default profile command \"%s\"\nend tell\nend tell", cmdStr)
		exec.Command("osascript", "-e", script).Start() //nolint:errcheck
	case "Apple_Terminal":
		script := fmt.Sprintf("tell application \"Terminal\"\ndo script \"%s\"\nactivate\nend tell", cmdStr)
		exec.Command("osascript", "-e", script).Start() //nolint:errcheck
	case "ghostty":
		exec.Command("ghostty", append([]string{"-e"}, cmd...)...).Start() //nolint:errcheck
	default:
		if os.Getenv("TERM") == "xterm-kitty" {
			exec.Command("kitty", cmd...).Start() //nolint:errcheck
			return
		}
		openLinuxTerminal(cmd...)
	}
}

func openLinuxTerminal(cmd ...string) {
	cmdStr := strings.Join(cmd, " ")
	candidates := [][]string{
		append([]string{"gnome-terminal", "--"}, cmd...),
		append([]string{"xterm", "-e"}, cmd...),
		append([]string{"alacritty", "-e"}, cmd...),
		append([]string{"konsole", "-e"}, cmd...),
		{"xfce4-terminal", "-e", cmdStr},
	}
	for _, args := range candidates {
		if _, err := exec.LookPath(args[0]); err == nil {
			exec.Command(args[0], args[1:]...).Start() //nolint:errcheck
			return
		}
	}
}

func (m model) Init() tea.Cmd {
	return tea.Batch(tickCmd(m.cfg.RefreshRate), ctxDoneCmd(m.ctx))
}

func (m model) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case ctxDoneMsg:
		return m, tea.Quit

	case tickMsg:
		if m.cfg.AutoOpen {
			for _, r := range m.fleet.Runners() {
				id := r.Task().ID()
				if r.Status() == agentfleet.StatusRunning && !m.openedTabs[id] {
					m.openedTabs[id] = true
					m.onAttach(id)
				}
			}
		}
		// clamp cursor if tasks disappeared
		active, done := orderedRunners(m.fleet.Runners(), m.cfg.MaxDoneTasks)
		if total := len(active) + len(done); total > 0 && m.cursor >= total {
			m.cursor = total - 1
		}
		return m, tickCmd(m.cfg.RefreshRate)

	case tea.WindowSizeMsg:
		m.termW = msg.Width
		m.termH = msg.Height
		return m, nil

	case tea.KeyMsg:
		active, done := orderedRunners(m.fleet.Runners(), m.cfg.MaxDoneTasks)
		all := make([]*agentfleet.Runner, 0, len(active)+len(done))
		all = append(all, active...)
		all = append(all, done...)
		total := len(all)
		prevCursor := m.cursor

		switch msg.Type {
		case tea.KeyUp:
			if m.cursor > 0 {
				m.cursor--
			}
		case tea.KeyDown:
			if m.cursor < total-1 {
				m.cursor++
			}
		case tea.KeyEnter:
			if m.cursor < len(all) && all[m.cursor].Status() == agentfleet.StatusRunning {
				m.onAttach(all[m.cursor].Task().ID())
			}
			return m, nil
		case tea.KeyCtrlC:
			return m, tea.Quit
		}

		switch msg.String() {
		case "j":
			if m.cursor < total-1 {
				m.cursor++
			}
		case "k":
			if m.cursor > 0 {
				m.cursor--
			}
		case "u":
			m.outScrollBack++
		case "d":
			if m.outScrollBack > 0 {
				m.outScrollBack--
			}
		case "q":
			return m, tea.Quit
		}

		// reset right panel scroll on task change
		if m.cursor != prevCursor {
			m.outScrollBack = 0
		}

		// clamp outScrollBack so d-key is always immediately responsive
		if m.cursor < len(all) {
			lines := all[m.cursor].Lines()
			if maxBack := len(lines) - m.mainHeight(); maxBack >= 0 {
				if m.outScrollBack > maxBack {
					m.outScrollBack = maxBack
				}
			} else {
				m.outScrollBack = 0
			}
		}

		// keep cursor visible in the list
		mainH := m.mainHeight()
		visRow := m.cursor
		if m.cursor >= len(active) && len(active) > 0 && len(done) > 0 {
			visRow++ // account for divider row
		}
		if visRow < m.listOffset {
			m.listOffset = visRow
		}
		if visRow >= m.listOffset+mainH {
			m.listOffset = visRow - mainH + 1
		}
	}
	return m, nil
}

// mainHeight returns usable height for the left/right panels.
func (m model) mainHeight() int {
	h := m.termH - 2 // subtract header + footer
	if m.cfg.Log != nil {
		h -= m.termH / 3
	}
	if h < 1 {
		h = 1
	}
	return h
}

func (m model) View() string {
	if m.termW == 0 || m.termH == 0 {
		return ""
	}

	active, done := orderedRunners(m.fleet.Runners(), m.cfg.MaxDoneTasks)
	all := make([]*agentfleet.Runner, 0, len(active)+len(done))
	all = append(all, active...)
	all = append(all, done...)

	mainH := m.mainHeight()
	leftW := m.termW / 2
	rightW := m.termW - leftW

	var selRunner *agentfleet.Runner
	if m.cursor < len(all) {
		selRunner = all[m.cursor]
	}

	header := renderHeader(m, active, done)
	left := renderLeft(m, active, done, mainH, leftW)
	right := renderRight(m, selRunner, mainH, rightW)
	main := lipgloss.JoinHorizontal(lipgloss.Top, left, right)

	footer := styleFooter.Render("[↑↓ j/k] navigate  [u/d] scroll output  [enter] attach  [q] quit")

	parts := []string{header, main}
	if m.cfg.Log != nil {
		parts = append(parts, renderLog(m))
	}
	parts = append(parts, footer)
	return strings.Join(parts, "\n")
}

func renderHeader(m model, active, done []*agentfleet.Runner) string {
	var running int
	for _, r := range active {
		if r.Status() == agentfleet.StatusRunning {
			running++
		}
	}
	total := len(active) + len(done)
	summary := fmt.Sprintf("%d tasks · %d running · %d done", total, running, len(done))

	title := styleTitle.Render("◈ agentfleet")
	if m.cfg.Title != nil {
		title = m.cfg.Title()
	}
	return title + "  " + styleSummary.Render(summary)
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

// renderLeft renders the task list left panel.
func renderLeft(m model, active, done []*agentfleet.Runner, mainH, w int) string {
	type row struct {
		runner  *agentfleet.Runner
		idx     int // index into active+done
		divider bool
	}

	var rows []row
	for i, r := range active {
		rows = append(rows, row{runner: r, idx: i})
	}
	if len(active) > 0 && len(done) > 0 {
		rows = append(rows, row{divider: true})
	}
	for i, r := range done {
		rows = append(rows, row{runner: r, idx: len(active) + i})
	}

	offset := m.listOffset
	if offset > len(rows) {
		offset = len(rows)
	}
	visible := rows[offset:]
	if len(visible) > mainH {
		visible = visible[:mainH]
	}

	lines := make([]string, 0, mainH)
	for _, row := range visible {
		if row.divider {
			lines = append(lines, styleDivider.Width(w).Render(strings.Repeat("─", w)))
		} else {
			lines = append(lines, renderRow(row.runner, row.idx == m.cursor, w))
		}
	}
	for len(lines) < mainH {
		lines = append(lines, strings.Repeat(" ", w))
	}
	return strings.Join(lines, "\n")
}

// renderRow renders a single task row in the left panel.
func renderRow(r *agentfleet.Runner, selected bool, w int) string {
	cursor := "  "
	idStyle := styleMeta
	if selected {
		cursor = "▶ "
		idStyle = styleSelID
	}

	task := r.Task()
	badge := statusBadge(r.Status())

	elapsed := ""
	if r.Status() == agentfleet.StatusRunning {
		d := time.Since(r.StartedAt()).Round(time.Second)
		elapsed = fmt.Sprintf("%02d:%02d", int(d.Minutes()), int(d.Seconds())%60)
	}
	elapsedCol := styleMeta.Width(5).Render(elapsed)

	idStr := idStyle.Render(task.ID())
	cursorW := lipgloss.Width(cursor)
	idW := lipgloss.Width(idStr)
	badgeW := lipgloss.Width(badge)
	elapsedW := lipgloss.Width(elapsedCol)
	nameMaxW := w - cursorW - idW - 2 - badgeW - 1 - elapsedW - 1
	if nameMaxW < 4 {
		nameMaxW = 4
	}
	name := truncateVisual(task.Name(), nameMaxW)

	left := cursor + idStr + "  " + name
	right := badge + " " + elapsedCol
	gap := w - lipgloss.Width(left) - lipgloss.Width(right)
	if gap < 1 {
		gap = 1
	}

	rowStr := left + strings.Repeat(" ", gap) + right
	if selected {
		return styleSel.Width(w).Render(rowStr)
	}
	return lipgloss.NewStyle().Width(w).Render(rowStr)
}

// renderRight renders the output panel for the selected task.
func renderRight(m model, r *agentfleet.Runner, mainH, w int) string {
	var rawLines []string
	if r != nil {
		rawLines = r.Lines()
	}

	lines := make([]string, len(rawLines))
	for i, l := range rawLines {
		lines[i] = truncateVisual(stripANSI(l), w)
	}

	total := len(lines)
	// cap scrollback to valid range
	maxBack := total - mainH
	if maxBack < 0 {
		maxBack = 0
	}
	scrollBack := m.outScrollBack
	if scrollBack > maxBack {
		scrollBack = maxBack
	}
	start := total - mainH - scrollBack
	if start < 0 {
		start = 0
	}
	end := start + mainH
	if end > total {
		end = total
	}

	visible := make([]string, mainH)
	if total > 0 {
		copy(visible, lines[start:end])
	}

	rowStyle := styleOutput.Width(w)
	rendered := make([]string, mainH)
	for i, l := range visible {
		rendered[i] = rowStyle.Render(l)
	}
	return strings.Join(rendered, "\n")
}

// renderLog renders the bottom log panel.
func renderLog(m model) string {
	logH := m.termH / 3
	if logH < 2 {
		logH = 2
	}
	w := m.termW

	allLines := m.cfg.Log.Lines()
	total := len(allLines)
	contentH := logH - 1
	start := total - contentH
	if start < 0 {
		start = 0
	}

	divider := styleDivider.Width(w).Render(strings.Repeat("─", w))
	rows := []string{divider}
	for _, l := range allLines[start:] {
		rows = append(rows, styleLog.Width(w).Render(truncateVisual(l, w)))
	}
	for len(rows) < logH {
		rows = append(rows, strings.Repeat(" ", w))
	}
	return strings.Join(rows[:logH], "\n")
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
