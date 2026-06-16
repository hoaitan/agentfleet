package agentfleet

import (
	"os"
	"time"

	"golang.org/x/term"
)

// Config holds all configuration for a fleet run.
type Config struct {
	Fleet FleetConfig
	TUI   TUIConfig
	Agent AgentConfig
}

// FleetConfig controls task scheduling and I/O paths.
type FleetConfig struct {
	MaxConcurrent  int    // max tasks running in parallel — default: 9
	RingBufferSize int    // output lines kept per runner   — default: 200
	SocketDir      string // Unix socket dir; empty = no socket server — default: /tmp
	LogDir         string // session log dir; empty = no log file    — default: /tmp
}

// TUIConfig controls the Bubbletea dashboard appearance.
type TUIConfig struct {
	Title        func() string           // left side of header; nil = "◈ agentfleet"
	TitleRight   func() string           // right side of header, right-aligned (e.g. connection status)
	RefreshRate  time.Duration           // TUI tick interval          — default: 500ms
	AutoOpen     bool                    // auto-open a tab for each task when it starts — default: true
	MaxDoneTasks int                     // done/failed tasks kept in list; 0 = no limit — default: 10
	Log          *LogBuffer              // nil = no log panel
	OnClose      func(taskID string)     // called when user presses x on a selected task; nil = no-op
	FilterLines  func([]string) []string // pre-process runner output before preview; nil = default chrome filter
}

// AgentConfig controls PTY dimensions and environment.
type AgentConfig struct {
	PTYRows int      // default: 24
	PTYCols int      // default: 220
	Env     []string // extra env vars (KEY=VALUE); appended to os.Environ() for child process only
}

// DefaultConfig returns sensible production defaults.
func DefaultConfig() Config {
	return Config{
		Fleet: FleetConfig{
			MaxConcurrent:  9,
			RingBufferSize: 200,
			SocketDir:      "/tmp",
			LogDir:         "/tmp",
		},
		TUI: TUIConfig{
			RefreshRate:  500 * time.Millisecond,
			AutoOpen:     true,
			MaxDoneTasks: 10,
		},
		Agent: AgentConfig{PTYRows: 24, PTYCols: 220},
	}
}

// AgentConfigFromTerminal reads actual terminal dimensions.
// Falls back to DefaultConfig().Agent when stdout is not a TTY.
func AgentConfigFromTerminal() AgentConfig {
	cols, rows, err := term.GetSize(int(os.Stdout.Fd()))
	if err != nil || rows <= 0 || cols <= 0 {
		return DefaultConfig().Agent
	}
	return AgentConfig{PTYRows: rows, PTYCols: cols}
}
