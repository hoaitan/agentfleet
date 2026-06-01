package main

import (
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"golang.org/x/term"
)

func main() {
	if len(os.Args) != 2 {
		fmt.Fprintln(os.Stderr, "usage: attach <task-id>")
		os.Exit(1)
	}
	if err := attach(os.Args[1]); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func attach(taskID string) error {
	// Retry for up to 3 seconds — socket may not be ready when tab opens.
	var (
		conn net.Conn
		err  error
	)
	for i := 0; i < 30; i++ {
		conn, err = net.Dial("unix", "/tmp/agentfleet-"+taskID+".sock")
		if err == nil {
			break
		}
		time.Sleep(100 * time.Millisecond)
	}
	if err != nil {
		return fmt.Errorf("connect to task %q: %w", taskID, err)
	}
	defer conn.Close()

	oldState, err := term.MakeRaw(int(os.Stdin.Fd()))
	if err != nil {
		return fmt.Errorf("raw mode: %w", err)
	}
	defer term.Restore(int(os.Stdin.Fd()), oldState) //nolint:errcheck

	go io.Copy(conn, os.Stdin)   //nolint:errcheck
	io.Copy(os.Stdout, conn)     //nolint:errcheck
	fmt.Print("\r\n[session ended]\r\n")
	return nil
}
