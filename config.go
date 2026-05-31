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
	Columns      int           // grid columns               — default: 3
	PreviewLines int           // output lines shown in card — default: 3
	CardWidth    int           // card width in chars        — default: 64
	RefreshRate  time.Duration // TUI tick interval          — default: 500ms
}

// AgentConfig controls PTY dimensions.
type AgentConfig struct {
	PTYRows int // default: 24
	PTYCols int // default: 220
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
			Columns:      3,
			PreviewLines: 3,
			CardWidth:    64,
			RefreshRate:  500 * time.Millisecond,
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
