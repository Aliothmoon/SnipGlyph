package hotkey

import "testing"

func TestParse(t *testing.T) {
	tests := []struct {
		in   string
		want Hotkey
		str  string
	}{
		{"Win+Alt+C", Hotkey{ModWin | ModAlt, 'C'}, "Win+Alt+C"},
		{"alt + win + shift + c", Hotkey{ModWin | ModAlt | ModShift, 'C'}, "Win+Alt+Shift+C"},
		{"Control+Alt+9", Hotkey{ModCtrl | ModAlt, '9'}, "Ctrl+Alt+9"},
		{"Ctrl+F12", Hotkey{ModCtrl, 0x7B}, "Ctrl+F12"},
		{"F1", Hotkey{0, 0x70}, "F1"},
		{"Shift+F3", Hotkey{ModShift, 0x72}, "Shift+F3"},
		{"printscreen", Hotkey{0, 0x2C}, "PrintScreen"},
		{"Alt+`", Hotkey{ModAlt, 0xC0}, "Alt+`"},
		{"Win+Alt+Space", Hotkey{ModWin | ModAlt, 0x20}, "Win+Alt+Space"},
	}
	for _, tt := range tests {
		got, err := Parse(tt.in)
		if err != nil {
			t.Errorf("Parse(%q) error: %v", tt.in, err)
			continue
		}
		if got != tt.want {
			t.Errorf("Parse(%q) = %+v, want %+v", tt.in, got, tt.want)
		}
		if s := got.String(); s != tt.str {
			t.Errorf("Parse(%q).String() = %q, want %q", tt.in, s, tt.str)
		}
	}
}

func TestParseErrors(t *testing.T) {
	for _, in := range []string{
		"",
		"C",         // bare ordinary key
		"Shift+A",   // just a capital A
		"Win+",      // missing key
		"Win+Alt",   // modifier as key
		"Hyper+C",   // unknown modifier
		"Win+Win+C", // duplicate modifier
		"Win+Alt+F25",
		"Win+Alt+F01",
		"Win+Alt+Foo",
	} {
		if hk, err := Parse(in); err == nil {
			t.Errorf("Parse(%q) = %v, want error", in, hk)
		}
	}
}
