package hotkey_test

import (
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/hotkey"
)

func TestParse_ValidKeys_ReadBackInCanonicalForm(t *testing.T) {
	cases := map[string]string{
		"F5":           "F5",
		"f24":          "F24",
		"a":            "A",
		"7":            "7",
		"num3":         "Num3",
		"shift+ctrl+1": "Ctrl+Shift+1",
		"Win+Alt+F12":  "Alt+Win+F12",
	}
	for text, want := range cases {
		key, err := hotkey.Parse(text)

		if err != nil || key.String() != want {
			t.Errorf("Parse(%q) = %v, %v; want %s", text, key, err, want)
		}
	}
}

func TestParse_InvalidKeys_Fail(t *testing.T) {
	for _, text := range []string{"", "F0", "F25", "F05", "Ctrl+", "Ctrl+Ctrl+A", "Meta+A", "Enter", "Num10", "AB"} {
		_, err := hotkey.Parse(text)

		if err == nil {
			t.Errorf("Parse(%q) succeeded", text)
		}
	}
}
