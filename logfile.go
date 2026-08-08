package agentfleet

import (
	"fmt"
	"os"
	"path/filepath"
	"sync"
)

// DefaultLogFileName is used when LogFileConfig.Path is empty.
const DefaultLogFileName = "agentfleet.log"

// Default rotation thresholds used by DefaultConfig.
const (
	DefaultLogMaxBytes = 10 << 20 // 10MB
	DefaultLogBackups  = 5
)

// LogFileConfig controls the on-disk copy of the process log stream.
//
// The library never installs a logger of its own: OpenLogFile hands back an
// io.Writer that the caller tees into whatever handler it already uses, so the
// same lines reach the TUI log panel and the file.
type LogFileConfig struct {
	Enabled  bool   // write the log stream to a file          — default: true
	Path     string // log file path; empty = <cwd>/agentfleet.log
	MaxBytes int64  // rotate once the live file exceeds this  — default: 10MB; <= 0 disables rotation
	Backups  int    // rotated generations kept (.1 … .N)      — default: 5; 0 truncates instead
}

// LogFile is an io.Writer that appends to a file and rotates it Unix-style:
// the live file keeps its name, and older generations shift down through
// <path>.1, <path>.2, … up to Backups before being discarded.
//
// A nil *LogFile is a valid no-op writer, so callers can pass the result of
// OpenLogFile straight to io.MultiWriter without a nil check.
type LogFile struct {
	mu       sync.Mutex
	path     string
	maxBytes int64
	backups  int
	f        *os.File
	size     int64
}

// OpenLogFile opens (creating or appending to) the configured log file.
// It returns a nil *LogFile when cfg.Enabled is false — writes to that value
// are discarded, so disabling the file needs no branching at the call site.
func OpenLogFile(cfg LogFileConfig) (lf *LogFile, err error) {
	if !cfg.Enabled {
		return nil, nil
	}
	path := cfg.Path
	if path == "" {
		path = DefaultLogFileName
	}
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve log file path %q: %w", path, err)
	}
	backups := cfg.Backups
	if backups < 0 {
		backups = 0
	}
	l := &LogFile{path: abs, maxBytes: cfg.MaxBytes, backups: backups}
	if err := l.open(); err != nil {
		return nil, err
	}
	return l, nil
}

// Path returns the absolute path of the live log file, or "" for a nil LogFile.
func (l *LogFile) Path() string {
	if l == nil {
		return ""
	}
	return l.path
}

// Write appends p to the log file, rotating first when the write would push
// the live file past MaxBytes. A single write is never split across two
// generations, so a log line always lands in one file.
func (l *LogFile) Write(p []byte) (n int, err error) {
	if l == nil {
		return len(p), nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return 0, os.ErrClosed
	}
	if l.maxBytes > 0 && l.size > 0 && l.size+int64(len(p)) > l.maxBytes {
		if err := l.rotate(); err != nil {
			return 0, err
		}
	}
	n, err = l.f.Write(p)
	l.size += int64(n)
	return n, err
}

// Close closes the live file. Writes after Close return os.ErrClosed.
func (l *LogFile) Close() error {
	if l == nil {
		return nil
	}
	l.mu.Lock()
	defer l.mu.Unlock()
	if l.f == nil {
		return nil
	}
	err := l.f.Close()
	l.f = nil
	return err
}

// open opens the live file for append and records its current size.
// Callers other than OpenLogFile must hold l.mu.
func (l *LogFile) open() error {
	f, err := os.OpenFile(l.path, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		return fmt.Errorf("open log file %q: %w", l.path, err)
	}
	size := int64(0)
	if st, err := f.Stat(); err == nil {
		size = st.Size()
	}
	l.f, l.size = f, size
	return nil
}

// rotate shifts the log generations down and reopens an empty live file.
// The caller must hold l.mu.
func (l *LogFile) rotate() error {
	if err := l.f.Close(); err != nil {
		return fmt.Errorf("close log file %q: %w", l.path, err)
	}
	l.f = nil

	if l.backups == 0 {
		// No generations kept: start the live file over.
		if err := os.Remove(l.path); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("remove log file %q: %w", l.path, err)
		}
		return l.open()
	}

	// Drop the oldest generation, then shift the rest down: .N-1 -> .N, … , .1 -> .2.
	if err := os.Remove(l.backupPath(l.backups)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("remove log file %q: %w", l.backupPath(l.backups), err)
	}
	for i := l.backups - 1; i >= 1; i-- {
		from, to := l.backupPath(i), l.backupPath(i+1)
		if err := os.Rename(from, to); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("rotate log file %q -> %q: %w", from, to, err)
		}
	}
	if err := os.Rename(l.path, l.backupPath(1)); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("rotate log file %q -> %q: %w", l.path, l.backupPath(1), err)
	}
	return l.open()
}

// backupPath returns the path of the nth rotated generation (<path>.n).
func (l *LogFile) backupPath(n int) string {
	return fmt.Sprintf("%s.%d", l.path, n)
}
