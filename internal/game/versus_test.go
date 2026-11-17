package game_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
)

const versusTable = `// exported from VersusClassDataTable.xlsm
"VersusClassDataTable"
{
	"VersusClassData_0"
	{
		"eClss"			"0"
		"pName"			"F"
		"baseCP"		"0"
		"bonusCP"		"30"
		"aScores[0]"	"2000"
		"aScores[1]"	"1500"
		"aScores[2]"	"1000"
		"aScores[3]"	"500"
		"aScores[4]"	"0"
	}
	"VersusClassData_1"
	{
		"eClss"			"1"
		"pName"			"E"
		"baseCP"		"400"
		"bonusCP"		"25"
		"aScores[0]"	"2000"
		"aScores[1]"	"1500"
		"aScores[2]"	"1000"
		"aScores[3]"	"500"
		"aScores[4]"	"0"
	}
	"RatingCP"
	{
		"RatingCP[0]"	"50"
	}
}
`

func TestParseVersusClasses_ReadsEachClassInOrder(t *testing.T) {
	got, err := game.ParseVersusClasses(strings.NewReader(versusTable))

	want := []game.VersusClass{
		{Name: "F", BaseCP: 0, BonusCP: 30, Scores: [5]int{2000, 1500, 1000, 500, 0}},
		{Name: "E", BaseCP: 400, BonusCP: 25, Scores: [5]int{2000, 1500, 1000, 500, 0}},
	}
	if err != nil || len(got) != 2 || got[0] != want[0] || got[1] != want[1] {
		t.Fatalf("ParseVersusClasses() = %+v, %v", got, err)
	}
}

func TestParseVersusClasses_NoClasses_IsAnError(t *testing.T) {
	_, err := game.ParseVersusClasses(strings.NewReader(`"VersusClassDataTable" { }`))

	if err == nil {
		t.Fatal("ParseVersusClasses() err = nil")
	}
}

func TestParseVersusClasses_InstalledGame_HasSevenClassesFromFToS(t *testing.T) {
	dir := os.Getenv("L4DS_GAME_DIR")
	if dir == "" {
		t.Skip("L4DS_GAME_DIR is not set")
	}
	file, err := os.Open(filepath.Join(dir, "left4dead2", "resource", "VersusClassDataTable.txt"))
	if err != nil {
		t.Fatal(err)
	}
	defer file.Close()

	got, err := game.ParseVersusClasses(file)

	if err != nil || len(got) != 7 || got[0].Name != "F" || got[6].Name != "S" || got[6].BaseCP != 2950 {
		t.Fatalf("ParseVersusClasses() = %+v, %v", got, err)
	}
}
