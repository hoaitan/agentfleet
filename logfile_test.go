package agentfleet_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	agentfleet "github.com/hoaitan/agentfleet"
)

func TestOpenLogFileDisabledReturnsNilNoOpWriter(t *testing.T) {
	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{Enabled: false})
	require.NoError(t, err)
	require.Nil(t, lf)

	// A nil *LogFile stays usable so callers need no nil check.
	n, err := lf.Write([]byte("dropped"))
	require.NoError(t, err)
	assert.Equal(t, len("dropped"), n)
	assert.Equal(t, "", lf.Path())
	assert.NoError(t, lf.Close())
}

func TestOpenLogFileResolvesRelativePath(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{Enabled: true, Path: "retask.log"})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lf.Close() })

	assert.True(t, filepath.IsAbs(lf.Path()), "Path() must be absolute, got %q", lf.Path())
	assert.Equal(t, "retask.log", filepath.Base(lf.Path()))
}

func TestOpenLogFileDefaultsToAgentfleetLog(t *testing.T) {
	t.Chdir(t.TempDir())

	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{Enabled: true})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lf.Close() })

	assert.Equal(t, agentfleet.DefaultLogFileName, filepath.Base(lf.Path()))
}

func TestLogFileAppendsAcrossOpens(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retask.log")

	first, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{Enabled: true, Path: path})
	require.NoError(t, err)
	_, err = first.Write([]byte("one\n"))
	require.NoError(t, err)
	require.NoError(t, first.Close())

	second, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{Enabled: true, Path: path})
	require.NoError(t, err)
	_, err = second.Write([]byte("two\n"))
	require.NoError(t, err)
	require.NoError(t, second.Close())

	assert.Equal(t, "one\ntwo\n", readFile(t, path))
}

func TestLogFileRotatesAtMaxBytes(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retask.log")
	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{
		Enabled: true, Path: path, MaxBytes: 10, Backups: 2,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lf.Close() })

	// Each write is 6 bytes, so every second write trips the 10-byte threshold.
	for _, line := range []string{"aaaaa\n", "bbbbb\n", "ccccc\n"} {
		_, err := lf.Write([]byte(line))
		require.NoError(t, err)
	}

	assert.Equal(t, "ccccc\n", readFile(t, path), "live file holds the newest write")
	assert.Equal(t, "bbbbb\n", readFile(t, path+".1"), ".1 holds the previous generation")
	assert.Equal(t, "aaaaa\n", readFile(t, path+".2"), ".2 holds the oldest kept generation")
}

func TestLogFileDiscardsGenerationsBeyondBackups(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retask.log")
	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{
		Enabled: true, Path: path, MaxBytes: 4, Backups: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lf.Close() })

	for _, line := range []string{"aaaaa\n", "bbbbb\n", "ccccc\n"} {
		_, err := lf.Write([]byte(line))
		require.NoError(t, err)
	}

	assert.Equal(t, "ccccc\n", readFile(t, path))
	assert.Equal(t, "bbbbb\n", readFile(t, path+".1"))
	assert.NoFileExists(t, path+".2", "only Backups generations are kept")
}

func TestLogFileZeroBackupsTruncates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retask.log")
	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{
		Enabled: true, Path: path, MaxBytes: 4, Backups: 0,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lf.Close() })

	_, err = lf.Write([]byte("aaaaa\n"))
	require.NoError(t, err)
	_, err = lf.Write([]byte("bbbbb\n"))
	require.NoError(t, err)

	assert.Equal(t, "bbbbb\n", readFile(t, path))
	assert.NoFileExists(t, path+".1")
}

func TestLogFileZeroMaxBytesNeverRotates(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retask.log")
	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{
		Enabled: true, Path: path, MaxBytes: 0, Backups: 3,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lf.Close() })

	_, err = lf.Write([]byte(strings.Repeat("x", 4096)))
	require.NoError(t, err)
	_, err = lf.Write([]byte(strings.Repeat("y", 4096)))
	require.NoError(t, err)

	assert.Len(t, readFile(t, path), 8192)
	assert.NoFileExists(t, path+".1")
}

func TestLogFileWriteIsNeverSplitAcrossGenerations(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retask.log")
	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{
		Enabled: true, Path: path, MaxBytes: 8, Backups: 1,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lf.Close() })

	_, err = lf.Write([]byte("short\n"))
	require.NoError(t, err)
	long := strings.Repeat("z", 64) + "\n"
	n, err := lf.Write([]byte(long))
	require.NoError(t, err)
	assert.Equal(t, len(long), n)

	// An oversized line rotates first, then lands whole in the new live file.
	assert.Equal(t, long, readFile(t, path))
	assert.Equal(t, "short\n", readFile(t, path+".1"))
}

func TestLogFileWriteAfterCloseFails(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retask.log")
	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{Enabled: true, Path: path})
	require.NoError(t, err)
	require.NoError(t, lf.Close())
	require.NoError(t, lf.Close(), "Close is idempotent")

	_, err = lf.Write([]byte("nope"))
	assert.ErrorIs(t, err, os.ErrClosed)
}

func TestLogFileConcurrentWrites(t *testing.T) {
	path := filepath.Join(t.TempDir(), "retask.log")
	lf, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{
		Enabled: true, Path: path, MaxBytes: 64, Backups: 3,
	})
	require.NoError(t, err)
	t.Cleanup(func() { _ = lf.Close() })

	done := make(chan struct{})
	for i := 0; i < 8; i++ {
		go func() {
			defer func() { done <- struct{}{} }()
			for j := 0; j < 50; j++ {
				_, _ = lf.Write([]byte("concurrent line\n"))
			}
		}()
	}
	for i := 0; i < 8; i++ {
		<-done
	}
}

func TestOpenLogFileUnwritablePath(t *testing.T) {
	_, err := agentfleet.OpenLogFile(agentfleet.LogFileConfig{
		Enabled: true, Path: filepath.Join(t.TempDir(), "missing-dir", "retask.log"),
	})
	require.Error(t, err)
}

func TestDefaultConfigEnablesLogFileAndPathDisplay(t *testing.T) {
	cfg := agentfleet.DefaultConfig()
	assert.True(t, cfg.LogFile.Enabled)
	assert.Equal(t, int64(agentfleet.DefaultLogMaxBytes), cfg.LogFile.MaxBytes)
	assert.Equal(t, agentfleet.DefaultLogBackups, cfg.LogFile.Backups)
	assert.True(t, cfg.TUI.ShowLogPath)
}

func readFile(t *testing.T, path string) string {
	t.Helper()
	b, err := os.ReadFile(path)
	require.NoError(t, err)
	return string(b)
}
