//go:build darwin

package engine

import "golang.org/x/sys/unix"

// flushInput discards unread terminal input so stray bytes from the TUI->claude
// handoff do not get consumed by claude's large-session resume gate. On BSD and
// macOS this is TIOCFLUSH with FREAD (0x1) to flush only the input queue.
func flushInput(fd uintptr) {
	_ = unix.IoctlSetPointerInt(int(fd), unix.TIOCFLUSH, 0x1)
}
