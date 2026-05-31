//go:build darwin

package proxy

import "golang.org/x/sys/unix"

func enableOutputProcessing(fd int) {
	cur, err := unix.IoctlGetTermios(fd, unix.TIOCGETA)
	if err != nil {
		return
	}
	cur.Oflag |= unix.OPOST | unix.ONLCR
	_ = unix.IoctlSetTermios(fd, unix.TIOCSETA, cur)
}
