package hook_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/tan/agentfleet/internal/hook"
)

func TestEmptyChainPassThrough(t *testing.T) {
	out, err := hook.Chain{}.Process([]byte("hello"), hook.DirOut)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), out)
}

func TestChainTransforms(t *testing.T) {
	ch := hook.Chain{
		hook.HookFunc(func(data []byte, dir hook.Dir) ([]byte, error) {
			return append([]byte("!"), data...), nil
		}),
	}
	out, err := ch.Process([]byte("hi"), hook.DirIn)
	require.NoError(t, err)
	assert.Equal(t, []byte("!hi"), out)
}

func TestChainFailOpen(t *testing.T) {
	ch := hook.Chain{
		hook.HookFunc(func(data []byte, dir hook.Dir) ([]byte, error) {
			return nil, errors.New("fail")
		}),
	}
	out, err := ch.Process([]byte("data"), hook.DirOut)
	require.NoError(t, err)
	assert.Equal(t, []byte("data"), out)
}

func TestChainSuppressNil(t *testing.T) {
	ch := hook.Chain{
		hook.HookFunc(func(data []byte, dir hook.Dir) ([]byte, error) {
			return nil, nil
		}),
	}
	out, err := ch.Process([]byte("data"), hook.DirOut)
	require.NoError(t, err)
	assert.Nil(t, out)
}

func TestFileLogger(t *testing.T) {
	var buf strings.Builder
	fl := hook.NewFileLogger(&buf)
	out, err := fl.Process([]byte("hello"), hook.DirOut)
	require.NoError(t, err)
	assert.Equal(t, []byte("hello"), out)
	assert.Contains(t, buf.String(), "[OUT]")
	assert.Contains(t, buf.String(), "hello")
}

func TestLogger(t *testing.T) {
	lg := hook.NewLogger(10)
	out, err := lg.Process([]byte("world"), hook.DirIn)
	require.NoError(t, err)
	assert.Equal(t, []byte("world"), out)
	evt := <-lg.Events
	assert.Equal(t, hook.DirIn, evt.Dir)
	assert.Equal(t, []byte("world"), evt.Data)
}
