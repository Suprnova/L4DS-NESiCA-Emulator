package zet_test

import (
	"testing"

	"l4ds-nesica-emulator/internal/zet"
)

func TestCards_Select_ReportsTheCardAsUnregistered(t *testing.T) {
	got := zet.Cards{}.Select("7020392000000001")

	if got != "1\n1\n7020392000000001\n0\n" {
		t.Fatalf("Select() = %q", got)
	}
}
