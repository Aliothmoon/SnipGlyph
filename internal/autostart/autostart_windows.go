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

// Enable makes exe start at sign-in under name (with optional args), replacing
// any earlier path, and clears a Task Manager switch-off.
func Enable(name, exe string, args ...string) error {
	k, _, err := registry.CreateKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if err := k.SetStringValue(name, Command(exe, args...)); err != nil {
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

// Command returns the Run command line for exe and any extra arguments.
func Command(exe string, args ...string) string {
	cmd := Quote(exe)
	if extra := formatArgs(args); extra != "" {
		cmd += " " + extra
	}
	return cmd
}

// Points reports whether command runs exe with the exact args
// (comparing exe case-insensitively, as Windows paths are).
func Points(command, exe string, args ...string) bool {
	s := strings.TrimSpace(command)
	var cmdExe, rest string
	if strings.HasPrefix(s, `"`) {
		end := strings.Index(s[1:], `"`)
		if end < 0 {
			return false
		}
		cmdExe = s[1 : 1+end]
		rest = strings.TrimSpace(s[2+end:])
	} else if len(args) == 0 {
		return strings.EqualFold(s, exe)
	} else {
		cmdExe, rest, _ = strings.Cut(s, " ")
		rest = strings.TrimSpace(rest)
	}
	if !strings.EqualFold(cmdExe, exe) {
		return false
	}
	return rest == formatArgs(args)
}

func formatArgs(args []string) string {
	var parts []string
	for _, a := range args {
		if a == "" {
			continue
		}
		if strings.ContainsAny(a, " \t") {
			parts = append(parts, `"`+a+`"`)
		} else {
			parts = append(parts, a)
		}
	}
	return strings.Join(parts, " ")
}
