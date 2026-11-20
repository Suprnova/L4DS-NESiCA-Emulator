// Package hotkey reads key combinations such as "Ctrl+F5" and, on Windows, calls back when one is
// pressed, whichever program has the focus.
package hotkey

import (
	"fmt"
	"strconv"
	"strings"
)

// Modifier keys, as Windows' RegisterHotKey takes them.
const (
	alt   = 0x1
	ctrl  = 0x2
	shift = 0x4
	win   = 0x8
)

var modifiers = []struct {
	name string
	bit  uint32
}{{"Ctrl", ctrl}, {"Alt", alt}, {"Shift", shift}, {"Win", win}}

// Key is a key combination: one key and the modifiers held with it. The zero Key is no key.
type Key struct {
	modifiers uint32
	// code is the Windows virtual-key code.
	code uint32
}

// Parse reads a key combination: modifiers (Ctrl, Alt, Shift, Win) and one key (A to Z, 0 to 9,
// Num0 to Num9, or F1 to F24), joined by "+", in any case, such as "Ctrl+Shift+1" or "F5".
func Parse(text string) (Key, error) {
	parts := strings.Split(text, "+")
	var key Key
	for _, part := range parts[:len(parts)-1] {
		bit := modifier(part)
		if bit == 0 || key.modifiers&bit != 0 {
			return Key{}, fmt.Errorf("key %q: %q is not a modifier, or is given twice", text, part)
		}
		key.modifiers |= bit
	}
	key.code = code(strings.ToUpper(parts[len(parts)-1]))
	if key.code == 0 {
		return Key{}, fmt.Errorf("key %q: unknown key %q", text, parts[len(parts)-1])
	}
	return key, nil
}

func modifier(name string) uint32 {
	for _, m := range modifiers {
		if strings.EqualFold(name, m.name) {
			return m.bit
		}
	}
	return 0
}

// code returns the virtual-key code of an upper-case key name, or 0 for an unknown one.
func code(name string) uint32 {
	switch {
	case len(name) == 1 && (name[0] >= 'A' && name[0] <= 'Z' || name[0] >= '0' && name[0] <= '9'):
		return uint32(name[0])
	case strings.HasPrefix(name, "NUM") && len(name) == 4 && name[3] >= '0' && name[3] <= '9':
		return 0x60 + uint32(name[3]-'0')
	case strings.HasPrefix(name, "F"):
		if n, err := strconv.Atoi(name[1:]); err == nil && n >= 1 && n <= 24 && name[1] != '0' {
			return 0x70 + uint32(n-1)
		}
	}
	return 0
}

// String returns the key as Parse reads it, with the modifiers in a fixed order.
func (k Key) String() string {
	var b strings.Builder
	for _, m := range modifiers {
		if k.modifiers&m.bit != 0 {
			b.WriteString(m.name + "+")
		}
	}
	switch {
	case k.code >= 0x70:
		fmt.Fprintf(&b, "F%d", k.code-0x70+1)
	case k.code >= 0x60:
		fmt.Fprintf(&b, "Num%d", k.code-0x60)
	default:
		b.WriteByte(byte(k.code))
	}
	return b.String()
}

// UnmarshalText reads a key with [Parse].
func (k *Key) UnmarshalText(text []byte) error {
	key, err := Parse(string(text))
	if err != nil {
		return err
	}
	*k = key
	return nil
}
