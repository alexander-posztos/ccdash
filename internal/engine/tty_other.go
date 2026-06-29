//go:build !darwin && !linux

package engine

// flushInput is a no-op on platforms without a known tty input-flush ioctl.
func flushInput(fd uintptr) {}
