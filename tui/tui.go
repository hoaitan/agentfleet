package tui

import (
	"context"
	"fmt"
	"io"
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
	styleTitle       = lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("#c084fc"))
	styleSummary     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	styleMeta        = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	styleSelID       = lipgloss.NewStyle().Foreground(lipgloss.Color("#c084fc"))
	styleFooter      = lipgloss.NewStyle().Foreground(lipgloss.Color("#4b5563"))
	styleQuitConfirm = lipgloss.NewStyle().Foreground(lipgloss.Color("#f59e0b"))
	styleLog         = lipgloss.NewStyle().Foreground(lipgloss.Color("#4b5563"))
	styleRunning     = lipgloss.NewStyle().Foreground(lipgloss.Color("#4ade80"))
	styleDone        = lipgloss.NewStyle().Foreground(lipgloss.Color("#34d399"))
	styleFailed      = lipgloss.NewStyle().Foreground(lipgloss.Color("#f87171"))
	stylePending     = lipgloss.NewStyle().Foreground(lipgloss.Color("#6b7280"))
	styleDivider     = lipgloss.NewStyle().Foreground(lipgloss.Color("#374151"))
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
	frameCount int // incremented each tick; drives the cursor-anchor trick in View()

	listOffset   int    // first visible visual row in task list
	selectedID   string // task ID of currently selected runner; stable through list reorders
	pendingQuit  bool   // true after first q press; second q confirms quit
	pendingClose bool   // true after first x press; second x confirms close
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
		c := exec.Command("tmux", append([]string{"new-window"}, cmd...)...)
		c.Stdout = io.Discard
		c.Stderr = io.Discard
		c.Start() //nolint:errcheck
		return
	}
	switch os.Getenv("TERM_PROGRAM") {
	case "iTerm.app":
		script := fmt.Sprintf("tell application \"iTerm2\"\ntell current window\ncreate tab with default profile command \"%s\"\nend tell\nend tell", cmdStr)
		c := exec.Command("osascript", "-e", script)
		c.Stdout = io.Discard
		c.Stderr = io.Discard
		c.Start() //nolint:errcheck
	case "Apple_Terminal":
		script := fmt.Sprintf("tell application \"Terminal\"\ndo script \"%s\"\nactivate\nend tell", cmdStr)
		c := exec.Command("osascript", "-e", script)
		c.Stdout = io.Discard
		c.Stderr = io.Discard
		c.Start() //nolint:errcheck
	case "ghostty":
		c := exec.Command("ghostty", append([]string{"-e"}, cmd...)...)
		c.Stdout = io.Discard
		c.Stderr = io.Discard
		c.Start() //nolint:errcheck
	default:
		if os.Getenv("TERM") == "xterm-kitty" {
			c := exec.Command("kitty", cmd...)
			c.Stdout = io.Discard
			c.Stderr = io.Discard
			c.Start() //nolint:errcheck
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
			c := exec.Command(args[0], args[1:]...)
			c.Stdout = io.Discard
			c.Stderr = io.Discard
			c.Start() //nolint:errcheck
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
		m.frameCount++
		if m.cfg.AutoOpen {
			for _, r := range m.fleet.Runners() {
				id := r.Task().ID()
				if r.Status() == agentfleet.StatusRunning && !m.openedTabs[id] {
					m.openedTabs[id] = true
					m.onAttach(id)
				}
			}
		}
		active, done := orderedRunners(m.fleet.Runners(), m.cfg.MaxDoneTasks)
		all := make([]*agentfleet.Runner, 0, len(active)+len(done))
		all = append(all, active...)
		all = append(all, done...)

		// Re-anchor cursor to the same task ID each tick.
		// orderedRunners puts newest first, so adding a new task shifts existing
		// indices — without this, cursor silently points to a different task and
		// Enter attaches the wrong one.
		if m.selectedID != "" {
			for i, r := range all {
				if r.Task().ID() == m.selectedID {
					m.cursor = i
					break
				}
			}
		}
		if total := len(all); total > 0 && m.cursor >= total {
			m.cursor = total - 1
		}
		// Initialise selectedID on first task
		if m.selectedID == "" && m.cursor < len(all) {
			m.selectedID = all[m.cursor].Task().ID()
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
		case tea.KeyEsc:
			m.pendingQuit = false
			m.pendingClose = false
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
		case "q":
			m.pendingClose = false
			if m.pendingQuit {
				return m, tea.Quit
			}
			m.pendingQuit = true
			return m, nil
		case "c":
			m.pendingQuit = false
			m.pendingClose = false
		case "x":
			if !m.pendingClose {
				m.pendingClose = true
				return m, nil
			}
			// Second x: confirmed — fire OnClose and reset state.
			m.pendingClose = false
			if m.cursor < len(all) && m.cfg.OnClose != nil {
				m.cfg.OnClose(all[m.cursor].Task().ID())
			}
		}

		// Sync selectedID so cursor stays on same task through list reorders.
		if m.cursor < len(all) {
			m.selectedID = all[m.cursor].Task().ID()
		}

		// keep cursor visible — each card is ~3 visual rows
		mainH, _ := m.layoutHeights()
		visibleCards := mainH / 3
		if visibleCards < 1 {
			visibleCards = 1
		}
		visRow := m.cursor
		if m.cursor >= len(active) && len(active) > 0 && len(done) > 0 {
			visRow++ // account for section divider row
		}
		if visRow < m.listOffset {
			m.listOffset = visRow
		}
		if visRow >= m.listOffset+visibleCards {
			m.listOffset = visRow - visibleCards + 1
		}
	}
	return m, nil
}

// layoutHeights returns the fixed line budgets for the task list and log panel.
// Heights are derived solely from termH, so they never shift mid-session
// (unless the user resizes). This is essential for Bubbletea's diff renderer:
// any change in total line count between frames causes its linesRendered counter
// to diverge from the actual cursor position, producing permanent "phantom" bleed-through.
func (m model) layoutHeights() (mainH, logH int) {
	remaining := m.termH - 2 // 1 header + 1 footer
	if remaining < 1 {
		remaining = 1
	}
	logH = remaining / 3
	if logH < 2 {
		logH = 2 // minimum: 1 divider + 1 content/blank
	}
	mainH = remaining - logH
	if mainH < 1 {
		mainH = 1
	}
	return
}

func (m model) View() string {
	if m.termW == 0 || m.termH == 0 {
		return ""
	}

	active, done := orderedRunners(m.fleet.Runners(), m.cfg.MaxDoneTasks)
	mainH, logH := m.layoutHeights()

	// anchor: embedded \033[H in line 0 moves the actual terminal cursor to (0,0)
	// before Bubbletea writes the header. This self-corrects any linesRendered drift
	// on every tick without a screen clear — the cursor is always at the right row
	// when line 0 is written, so all subsequent \r\n advances land on correct rows.
	//
	// invis: invisible SGR counter that increments every tick. Adding it to every
	// blank/padding line makes those lines appear "changed" to Bubbletea's diff,
	// so they are always written (never skipped). A skipped blank row that contains
	// stale content from a previous frame will never be cleared by the diff alone —
	// only by actually writing to that row. Combined with the anchor, this eliminates
	// all phantom bleed-through without any screen flash.
	anchor := fmt.Sprintf("\033[H\033[%dm\033[m", m.frameCount%256)
	invis := fmt.Sprintf("\033[%dm\033[m", m.frameCount%256)

	header := anchor + renderHeader(m, active, done)
	footer := renderFooter(m, m.termW, invis)

	// When there's no log panel, give the log's share of lines to the task list.
	taskMainH := mainH
	if m.cfg.Log == nil {
		taskMainH = mainH + logH
	}
	taskList := renderTaskList(m, active, done, taskMainH, m.termW, invis)

	var parts []string
	if m.cfg.Log != nil {
		parts = []string{header, taskList, renderLog(m, logH, invis), footer}
	} else {
		parts = []string{header, taskList, footer}
	}

	output := strings.Join(parts, "\n")
	// ensure exactly termH lines so linesRendered stays correct
	if lineCount := strings.Count(output, "\n") + 1; m.termH > lineCount {
		output += strings.Repeat("\n", m.termH-lineCount)
	}
	return output
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

	right := ""
	if m.cfg.TitleRight != nil {
		right = m.cfg.TitleRight()
	}
	rightW := lipgloss.Width(right)

	// 2-space left/right padding: reserve 4 chars for padding, compute layout
	// within the remaining width. A header that wraps corrupts Bubbletea's
	// linesRendered counter, causing phantom bleed-through on every row below.
	const pad = 2
	innerW := m.termW - pad*2

	var line string
	leftFull := title + "  " + styleSummary.Render(summary)
	gap := innerW - lipgloss.Width(leftFull) - rightW
	if gap < 1 {
		// Not enough room with summary — fall back to title-only
		gap = innerW - lipgloss.Width(title) - rightW
		if gap < 1 {
			gap = 1
		}
		if right == "" {
			line = title
		} else {
			line = title + strings.Repeat(" ", gap) + right
		}
	} else if right == "" {
		line = leftFull
	} else {
		line = leftFull + strings.Repeat(" ", gap) + right
	}

	line = strings.Repeat(" ", pad) + line
	if visW := lipgloss.Width(line); m.termW > visW {
		line += strings.Repeat(" ", m.termW-visW)
	}
	return line
}

var spinnerFrames = []string{"◐", "◓", "◑", "◒"}

func statusBadge(s agentfleet.Status, frame int) string {
	const w = 10
	switch s {
	case agentfleet.StatusRunning:
		sp := spinnerFrames[frame%len(spinnerFrames)]
		return styleRunning.Width(w).Render(sp + " running")
	case agentfleet.StatusDone:
		return styleDone.Width(w).Render("✓ done")
	case agentfleet.StatusFailed:
		return styleFailed.Width(w).Render("✗ failed")
	default:
		return stylePending.Width(w).Render("○ pending")
	}
}

// renderTaskList renders the full-width task list as cards.
func renderTaskList(m model, active, done []*agentfleet.Runner, mainH, w int, invis string) string {
	type row struct {
		runner  *agentfleet.Runner
		idx     int
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

	const previewN = 5

	filter := m.cfg.FilterLines
	if filter == nil {
		filter = filterAgentChrome
	}

	lines := make([]string, 0, mainH)
	for _, row := range rows[offset:] {
		if len(lines) >= mainH {
			break
		}
		if row.divider {
			label := " done "
			dashW := w - len([]rune(label)) - 2
			if dashW < 0 {
				dashW = 0
			}
			lines = append(lines, invis+styleDivider.Render("─"+label+strings.Repeat("─", dashW)+"─"))
			continue
		}

		selected := row.idx == m.cursor
		var preview []string
		if selected {
			filtered := filter(row.runner.Lines())
			start := len(filtered) - previewN
			if start < 0 {
				start = 0
			}
			for _, l := range filtered[start:] {
				preview = append(preview, stripANSI(l))
			}
		}

		for _, cl := range strings.Split(renderCard(row.runner, selected, w, preview, m.frameCount), "\n") {
			if len(lines) >= mainH {
				break
			}
			lines = append(lines, invis+cl)
		}
	}
	padLine := invis + strings.Repeat(" ", w)
	for len(lines) < mainH {
		lines = append(lines, padLine)
	}
	return strings.Join(lines, "\n")
}

// renderCard renders a task as a boxed card.
// Non-selected: 3 lines (top border + content + bottom border).
// Selected: 3 + len(preview) lines.
func renderCard(r *agentfleet.Runner, selected bool, w int, preview []string, frameCount int) string {
	innerW := w - 2 // RoundedBorder uses 1 char on each side

	badge := statusBadge(r.Status(), frameCount)
	elapsed := ""
	if r.Status() == agentfleet.StatusRunning {
		d := time.Since(r.StartedAt()).Round(time.Second)
		elapsed = fmt.Sprintf("%02d:%02d", int(d.Minutes()), int(d.Seconds())%60)
	}

	task := r.Task()
	cursor := "  "
	idStyle := styleMeta
	if selected {
		cursor = "▶ "
		idStyle = styleSelID
	}

	idStr := idStyle.Render(shortID(task.ID()))
	elapsedStr := styleMeta.Width(5).Render(elapsed)
	rightStr := idStr + "  " + elapsedStr
	leftPrefix := cursor + badge + "  "

	nameMaxW := innerW - lipgloss.Width(leftPrefix) - lipgloss.Width(rightStr) - 1
	if nameMaxW < 4 {
		nameMaxW = 4
	}
	name := truncateVisual(task.Name(), nameMaxW)
	leftStr := leftPrefix + name
	gap := innerW - lipgloss.Width(leftStr) - lipgloss.Width(rightStr)
	if gap < 1 {
		gap = 1
	}

	var sb strings.Builder
	sb.WriteString(leftStr + strings.Repeat(" ", gap) + rightStr)
	for _, l := range preview {
		sb.WriteString("\n  ")
		sb.WriteString(styleLog.Render(truncateVisual(l, innerW-2)))
	}

	borderColor := lipgloss.Color("#374151")
	if selected {
		borderColor = lipgloss.Color("#7c3aed")
	}

	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(borderColor).
		Width(innerW).
		Render(sb.String())
}

func shortID(id string) string {
	if len(id) > 8 {
		return id[:8]
	}
	return id
}

// renderLog renders the bottom log panel at a fixed logH height.
// Long lines are wrapped rather than truncated. The most recent segments
// fill the content area; blank rows pad the rest so Bubbletea's diff always
// writes every line.
func renderLog(m model, logH int, invis string) string {
	w := m.termW
	contentH := logH - 1 // one row reserved for the divider

	// Wrap every source line into display segments, collect all.
	var segs []string
	for _, l := range m.cfg.Log.Lines() {
		segs = append(segs, wrapLine(l, w)...)
	}
	// Take the most recent contentH segments.
	start := len(segs) - contentH
	if start < 0 {
		start = 0
	}

	label := " Logs "
	dashW := w - len([]rune(label)) - 2
	if dashW < 0 {
		dashW = 0
	}
	divider := styleDivider.Render("─" + label + strings.Repeat("─", dashW) + "─")
	rows := []string{invis + divider}
	for _, seg := range segs[start:] {
		if len(rows) >= logH {
			break
		}
		rows = append(rows, invis+styleLog.Width(w).Render(seg))
	}
	padLine := invis + strings.Repeat(" ", w)
	for len(rows) < logH {
		rows = append(rows, padLine)
	}
	return strings.Join(rows, "\n")
}

// wrapLine splits s into visual segments each at most maxW display columns wide.
func wrapLine(s string, maxW int) []string {
	if maxW <= 0 {
		return []string{""}
	}
	s = stripANSI(s)
	if lipgloss.Width(s) <= maxW {
		return []string{s}
	}
	var out []string
	runes := []rune(s)
	for len(runes) > 0 {
		w := 0
		cut := len(runes)
		for i, ch := range runes {
			cw := lipgloss.Width(string(ch))
			if w+cw > maxW {
				cut = i
				break
			}
			w += cw
		}
		out = append(out, string(runes[:cut]))
		runes = runes[cut:]
	}
	return out
}

func renderFooter(m model, w int, invis string) string {
	var content string
	switch {
	case m.pendingQuit:
		content = styleQuitConfirm.Render("Press q again to quit · c to cancel")
	case m.pendingClose:
		content = styleQuitConfirm.Render("Press x again to close session · c to cancel")
	default:
		content = styleFooter.Render("↑↓ / jk  navigate · enter  attach session · x  close selected session · q  quit")
	}
	visW := lipgloss.Width(content)
	if w > visW {
		content += strings.Repeat(" ", w-visW)
	}
	return invis + content
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
