package zet_test

import (
	"strings"
	"testing"

	"l4ds-nesica-emulator/internal/zet"
)

func TestCards_Select_ReturnsARegisteredPlayer(t *testing.T) {
	got := strings.Split(zet.Cards{}.Select("7020392000000001"), "\n")

	if got[0] != "1" || got[2] != "7020392000000001" || got[3] != "1" || got[4] != "Survivor" {
		t.Fatalf("Select() lines = %q", got)
	}
}
