package game_test

import (
	"strings"
	"testing"

	"l4ds-nesica-emulator/internal/game"
)

const table = `// exported from a spreadsheet
"LevelTable"
{
	"MaxLv"	"3"

	"Lv1"
	{
		"BaseExp"	"0"
		"LvUpExp"	"150"
	}

	"Lv2"
	{
		"BaseExp"	"150"
		"LvUpExp"	"170"
	}

	"Lv3"
	{
		"BaseExp"	"320"
		"LvUpExp"	"190"
	}
}`

func TestLevelTable_Level_ReturnsTheHighestLevelStarted(t *testing.T) {
	levels, err := game.ParseLevelTable(strings.NewReader(table))
	if err != nil {
		t.Fatal(err)
	}

	for exp, want := range map[int64]int{0: 1, 149: 1, 150: 2, 319: 2, 320: 3, 99999: 3} {
		if got := levels.Level(exp); got != want {
			t.Errorf("Level(%d) = %d, want %d", exp, got, want)
		}
	}
}

func TestLevelTable_NilTable_EveryoneIsLevelOne(t *testing.T) {
	var levels *game.LevelTable

	got := levels.Level(5000)

	if got != 1 {
		t.Fatalf("Level() = %d, want 1", got)
	}
}

func TestParseLevelTable_NoLevels_ReturnsAnError(t *testing.T) {
	_, err := game.ParseLevelTable(strings.NewReader(`"LevelTable" { "MaxLv" "999" }`))

	if err == nil {
		t.Fatal("ParseLevelTable() error = nil")
	}
}
