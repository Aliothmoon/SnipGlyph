package autostart

import (
	"testing"

	"golang.org/x/sys/windows/registry"
)

// scratch points the package at throwaway keys under HKCU and removes them
// afterwards, so tests never touch the real Run key.
func scratch(t *testing.T) {
	t.Helper()
	const root = `Software\SnipGlyphTest`
	oldRun, oldApproved := runKey, approvedKey
	runKey, approvedKey = root+`\Run`, root+`\StartupApproved\Run`
	t.Cleanup(func() {
		runKey, approvedKey = oldRun, oldApproved
		for _, k := range []string{root + `\StartupApproved\Run`, root + `\StartupApproved`, root + `\Run`, root} {
			registry.DeleteKey(registry.CURRENT_USER, k)
		}
	})
}

func TestEnableDisable(t *testing.T) {
	scratch(t)
	const name = "SnipGlyphTest"
	exe := `C:\Program Files\SnipGlyph\snipglyph.exe`

	if on, _, err := Status(name); err != nil || on {
		t.Fatalf("before: on=%v err=%v", on, err)
	}
	if err := Enable(name, exe); err != nil {
		t.Fatal(err)
	}
	on, cmd, err := Status(name)
	if err != nil || !on || cmd != `"C:\Program Files\SnipGlyph\snipglyph.exe"` {
		t.Fatalf("after enable: on=%v cmd=%q err=%v", on, cmd, err)
	}
	if !Points(cmd, `c:\program files\snipglyph\SNIPGLYPH.EXE`) || Points(cmd, `C:\other.exe`) {
		t.Errorf("Points(%q) wrong", cmd)
	}

	// Switched off in Task Manager.
	k, _, err := registry.CreateKey(registry.CURRENT_USER, approvedKey, registry.SET_VALUE)
	if err != nil {
		t.Fatal(err)
	}
	k.SetBinaryValue(name, []byte{3, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0})
	k.Close()
	if on, _, _ := Status(name); on {
		t.Error("an entry disabled in Task Manager reported as enabled")
	}
	// Enabling again clears the switch.
	if err := Enable(name, exe); err != nil {
		t.Fatal(err)
	}
	if on, _, _ := Status(name); !on {
		t.Error("Enable did not clear the Task Manager switch")
	}

	if err := Disable(name); err != nil {
		t.Fatal(err)
	}
	if on, cmd, err := Status(name); err != nil || on || cmd != "" {
		t.Errorf("after disable: on=%v cmd=%q err=%v", on, cmd, err)
	}
	if err := Disable(name); err != nil {
		t.Errorf("disabling twice: %v", err)
	}
}
