// Package hotkey parses hotkey strings such as "Win+Alt+C" and registers
// them as system-wide hotkeys.
package hotkey

import (
	"fmt"
	"strings"
)

// Modifiers is a bit set using the MOD_* values of RegisterHotKey.
type Modifiers uint32

const (
	ModAlt   Modifiers = 0x1
	ModCtrl  Modifiers = 0x2
	ModShift Modifiers = 0x4
	ModWin   Modifiers = 0x8
)

// Hotkey is a modifier set plus one virtual-key code.
type Hotkey struct {
	Mods Modifiers
	Key  uint32 // VK_* code
}

// modifierOrder is the canonical order used by String.
var modifierOrder = []struct {
	mod  Modifiers
	name string
}{
	{ModWin, "Win"},
	{ModCtrl, "Ctrl"},
	{ModAlt, "Alt"},
	{ModShift, "Shift"},
}

var modifierAliases = map[string]Modifiers{
	"win":     ModWin,
	"windows": ModWin,
	"ctrl":    ModCtrl,
	"control": ModCtrl,
	"alt":     ModAlt,
	"shift":   ModShift,
}

// keyNames maps canonical key names to virtual-key codes. Letters and digits
// are handled separately.
var keyNames = map[string]uint32{
	"Space":       0x20,
	"PageUp":      0x21,
	"PageDown":    0x22,
	"End":         0x23,
	"Home":        0x24,
	"Left":        0x25,
	"Up":          0x26,
	"Right":       0x27,
	"Down":        0x28,
	"PrintScreen": 0x2C,
	"Insert":      0x2D,
	"Delete":      0x2E,
	"Pause":       0x13,
	";":           0xBA,
	"=":           0xBB,
	",":           0xBC,
	"-":           0xBD,
	".":           0xBE,
	"/":           0xBF,
	"`":           0xC0,
	"[":           0xDB,
	"\\":          0xDC,
	"]":           0xDD,
	"'":           0xDE,
}

const (
	vkF1  = 0x70
	vkF24 = 0x87
)

// Parse parses strings like "Win+Alt+Shift+C". Modifier names are
// case-insensitive and may appear in any order. A hotkey without Win, Ctrl
// or Alt is only accepted for function keys, PrintScreen and Pause, so that
// a typo can never swallow an ordinary key system-wide (Shift+A is just a
// capital A).
func Parse(s string) (Hotkey, error) {
	parts := strings.Split(s, "+")
	var hk Hotkey
	for i, raw := range parts {
		p := strings.TrimSpace(raw)
		if p == "" {
			return Hotkey{}, fmt.Errorf("hotkey %q: empty part", s)
		}
		if i < len(parts)-1 {
			m, ok := modifierAliases[strings.ToLower(p)]
			if !ok {
				return Hotkey{}, fmt.Errorf("hotkey %q: unknown modifier %q", s, p)
			}
			if hk.Mods&m != 0 {
				return Hotkey{}, fmt.Errorf("hotkey %q: duplicate modifier %q", s, p)
			}
			hk.Mods |= m
			continue
		}
		vk, ok := parseKey(p)
		if !ok {
			return Hotkey{}, fmt.Errorf("hotkey %q: unknown key %q", s, p)
		}
		hk.Key = vk
	}
	if hk.Mods&^ModShift == 0 && !bareKeyAllowed(hk.Key) {
		return Hotkey{}, fmt.Errorf("hotkey %q: needs Win, Ctrl or Alt", s)
	}
	return hk, nil
}

// String returns the canonical form, e.g. "Win+Alt+Shift+C".
func (h Hotkey) String() string {
	return h.Prefix() + keyName(h.Key)
}

// Prefix returns the modifiers in canonical form, each followed by "+",
// e.g. "Win+Alt+".
func (h Hotkey) Prefix() string {
	var b strings.Builder
	for _, m := range modifierOrder {
		if h.Mods&m.mod != 0 {
			b.WriteString(m.name)
			b.WriteByte('+')
		}
	}
	return b.String()
}

func parseKey(p string) (uint32, bool) {
	if len(p) == 1 {
		c := p[0]
		switch {
		case c >= 'a' && c <= 'z':
			return uint32(c - 'a' + 'A'), true
		case c >= 'A' && c <= 'Z', c >= '0' && c <= '9':
			return uint32(c), true
		}
	}
	if (p[0] == 'F' || p[0] == 'f') && len(p) <= 3 {
		var n int
		if _, err := fmt.Sscanf(p[1:], "%d", &n); err == nil && n >= 1 && n <= 24 && fmt.Sprint(n) == p[1:] {
			return uint32(vkF1 + n - 1), true
		}
	}
	for name, vk := range keyNames {
		if strings.EqualFold(name, p) {
			return vk, true
		}
	}
	return 0, false
}

func keyName(vk uint32) string {
	switch {
	case vk >= 'A' && vk <= 'Z', vk >= '0' && vk <= '9':
		return string(rune(vk))
	case vk >= vkF1 && vk <= vkF24:
		return fmt.Sprintf("F%d", vk-vkF1+1)
	}
	for name, v := range keyNames {
		if v == vk {
			return name
		}
	}
	return fmt.Sprintf("VK_0x%02X", vk)
}

func bareKeyAllowed(vk uint32) bool {
	return (vk >= vkF1 && vk <= vkF24) || vk == keyNames["PrintScreen"] || vk == keyNames["Pause"]
}
