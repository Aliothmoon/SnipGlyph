// Package autostart starts the program at sign-in through the per-user Run
// key, which needs no administrator rights.
package autostart

import (
	"errors"
	"io/fs"
	"strings"

	"golang.org/x/sys/windows/registry"
)

// Keys under HKEY_CURRENT_USER; variables so tests can use scratch keys.
var (
	runKey = `Software\Microsoft\Windows\CurrentVersion\Run`
	// Task Manager's Startup page records its enable/disable switch here,
	// and Windows skips a Run entry this marks as disabled.
	approvedKey = `Software\Microsoft\Windows\CurrentVersion\Explorer\StartupApproved\Run`
)

// Status reports whether name starts at sign-in, and the command it runs.
// An entry switched off in Task Manager counts as disabled.
func Status(name string) (enabled bool, command string, err error) {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if errors.Is(err, fs.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	defer k.Close()
	command, _, err = k.GetStringValue(name)
	if errors.Is(err, fs.ErrNotExist) {
		return false, "", nil
	}
	if err != nil {
		return false, "", err
	}
	return !disabledInTaskManager(name), command, nil
}

// disabledInTaskManager reads the StartupApproved flag: the low bit of the
// first byte is set when the user switched the entry off.
func disabledInTaskManager(name string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, approvedKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	v, _, err := k.GetBinaryValue(name)
	return err == nil && len(v) > 0 && v[0]&1 != 0
}

// Enable makes exe start at sign-in under name, replacing any earlier
// path, and clears a Task Manager switch-off.
func Enable(name, exe string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue(name, Quote(exe)); err != nil {
		return err
	}
	return deleteValue(approvedKey, name)
}

// Disable stops name from starting at sign-in.
func Disable(name string) error {
	if err := deleteValue(runKey, name); err != nil {
		return err
	}
	return deleteValue(approvedKey, name)
}

func deleteValue(key, name string) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, key, registry.SET_VALUE)
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.DeleteValue(name); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	return nil
}

// Quote returns the Run command for exe: the path in double quotes, so
// spaces in it are not taken as argument separators.
func Quote(exe string) string { return `"` + exe + `"` }

// Points reports whether command runs exe (case-insensitively, as Windows
// paths are).
func Points(command, exe string) bool {
	return strings.EqualFold(strings.Trim(strings.TrimSpace(command), `"`), exe)
}
