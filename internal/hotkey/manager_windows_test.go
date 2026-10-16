package hotkey

import "testing"

// An obscure combination that no real program is expected to own.
var testHotkey = Hotkey{ModCtrl | ModAlt | ModShift | ModWin, 0x87} // F24

func TestManagerConflictAndRelease(t *testing.T) {
	m1, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	defer m1.Close()
	m2, err := NewManager()
	if err != nil {
		t.Fatal(err)
	}
	defer m2.Close()

	if err := m1.Register(1, testHotkey); err != nil {
		t.Fatalf("m1.Register: %v", err)
	}
	// Re-registering the same id replaces the binding instead of failing.
	if err := m1.Register(1, testHotkey); err != nil {
		t.Fatalf("m1 re-Register: %v", err)
	}
	err = m2.Register(7, testHotkey)
	if !IsTaken(err) {
		t.Fatalf("m2.Register while m1 holds it: err = %v, want taken", err)
	}

	if err := m1.Unregister(1); err != nil {
		t.Fatalf("m1.Unregister: %v", err)
	}
	if err := m2.Register(7, testHotkey); err != nil {
		t.Fatalf("m2.Register after release: %v", err)
	}

	// Close releases everything the manager still holds.
	m2.Close()
	if err := m1.Register(1, testHotkey); err != nil {
		t.Fatalf("m1.Register after m2.Close: %v", err)
	}
	if err := m2.Register(7, testHotkey); err != ErrClosed {
		t.Fatalf("Register after Close: err = %v, want ErrClosed", err)
	}
	if _, ok := <-m2.Events(); ok {
		t.Fatal("Events not closed after Close")
	}
}
