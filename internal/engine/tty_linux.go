//go:build linux

package engine

import "golang.org/x/sys/unix"

// flushInput discards unread terminal input (TCIFLUSH) so stray bytes from the
// TUI->claude handoff do not get consumed by claude's resume gate.
func flushInput(fd uintptr) {
	_ = unix.IoctlSetInt(int(fd), unix.TCFLSH, unix.TCIFLUSH)
}
