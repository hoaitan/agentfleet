package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"sync"
	"sync/atomic"

	"github.com/hoaitan/agentfleet/internal/fleet"
)

type taskSession interface {
	SetOutput(io.Writer)
	StdinWriter() io.Writer
	Done() <-chan struct{}
}

var _ taskSession = (*fleet.Runner)(nil)

func serveTask(session taskSession, id string) {
	path := "/tmp/agentfleet-" + id + ".sock"
	os.Remove(path)
	ln, err := net.Listen("unix", path)
	if err != nil {
		fmt.Fprintf(os.Stderr, "socket listen %s: %v\n", path, err)
		return
	}
	defer os.Remove(path)

	var (
		connected  atomic.Bool
		activeMu   sync.Mutex
		activeConn net.Conn
	)

	go func() {
		<-session.Done()
		activeMu.Lock()
		if activeConn != nil {
			activeConn.Close()
		}
		activeMu.Unlock()
		ln.Close()
	}()

	for {
		conn, err := ln.Accept()
		if err != nil {
			return
		}
		if !connected.CompareAndSwap(false, true) {
			conn.Write([]byte("already attached\n")) //nolint:errcheck
			conn.Close()
			continue
		}
		activeMu.Lock()
		activeConn = conn
		activeMu.Unlock()

		go func() {
			defer func() {
				activeMu.Lock()
				activeConn = nil
				activeMu.Unlock()
				session.SetOutput(io.Discard)
				connected.Store(false)
				conn.Close()
			}()
			session.SetOutput(conn)
			io.Copy(session.StdinWriter(), conn) //nolint:errcheck
		}()
	}
}
