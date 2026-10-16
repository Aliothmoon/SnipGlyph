// Package instance enforces a single running instance per user session.
package instance

import (
	"errors"

	"golang.org/x/sys/windows"
)

// ErrRunning means another instance already holds the lock.
var ErrRunning = errors.New("another instance is already running")

// Acquire takes the session-wide named lock. Call release on exit; the OS
// also drops the lock if the process dies.
func Acquire(name string) (release func(), err error) {
	n, err := windows.UTF16PtrFromString(`Local\` + name)
	if err != nil {
		return nil, err
	}
	h, err := windows.CreateMutex(nil, false, n)
	if errors.Is(err, windows.ERROR_ALREADY_EXISTS) {
		windows.CloseHandle(h)
		return nil, ErrRunning
	}
	if err != nil {
		return nil, err
	}
	return func() { windows.CloseHandle(h) }, nil
}
