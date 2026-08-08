package agentfleet_test

import (
	"strings"
	"testing"

	agentfleet "github.com/hoaitan/agentfleet"
	"github.com/stretchr/testify/assert"
)

func TestLogBuffer_Write_basic(t *testing.T) {
	lb := agentfleet.NewLogBuffer(10)
	lb.Write([]byte("line1\nline2\n")) //nolint:errcheck
	assert.Equal(t, []string{"line1", "line2"}, lb.Lines())
}

func TestLogBuffer_Write_partialThenComplete(t *testing.T) {
	lb := agentfleet.NewLogBuffer(10)
	lb.Write([]byte("hel"))         //nolint:errcheck
	lb.Write([]byte("lo\nworld\n")) //nolint:errcheck
	assert.Equal(t, []string{"hello", "world"}, lb.Lines())
}

func TestLogBuffer_Overflow_dropsOldest(t *testing.T) {
	lb := agentfleet.NewLogBuffer(3)
	lb.Write([]byte("a\nb\nc\nd\n")) //nolint:errcheck
	assert.Equal(t, []string{"b", "c", "d"}, lb.Lines())
}

func TestLogBuffer_Lines_returnsCopy(t *testing.T) {
	lb := agentfleet.NewLogBuffer(10)
	lb.Write([]byte("x\n")) //nolint:errcheck
	lines := lb.Lines()
	lines[0] = "mutated"
	assert.Equal(t, []string{"x"}, lb.Lines()) // original unchanged
}

func TestLogBuffer_ImplementsWriter(t *testing.T) {
	lb := agentfleet.NewLogBuffer(10)
	n, err := lb.Write([]byte("test\n"))
	assert.NoError(t, err)
	assert.Equal(t, 5, n)
}

func TestLogBuffer_ConcurrentWrite(t *testing.T) {
	lb := agentfleet.NewLogBuffer(100)
	done := make(chan struct{})
	for i := 0; i < 10; i++ {
		go func() {
			lb.Write([]byte(strings.Repeat("a", 10) + "\n")) //nolint:errcheck
			done <- struct{}{}
		}()
	}
	for i := 0; i < 10; i++ {
		<-done
	}
	assert.Len(t, lb.Lines(), 10)
}

func TestLogBuffer_DefaultCapacity(t *testing.T) {
	for _, n := range []int{0, -1} {
		lb := agentfleet.NewLogBuffer(n)
		for i := 0; i < 205; i++ {
			lb.Write([]byte("x\n")) //nolint:errcheck
		}
		assert.Len(t, lb.Lines(), 200, "NewLogBuffer(%d) should default to 200", n)
	}
}
