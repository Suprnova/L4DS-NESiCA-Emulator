package game_test

import (
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
)

// writeGameFolder writes a game folder with the files LoadFolder reads, and returns its path.
func writeGameFolder(t *testing.T, files map[string]string) string {
	t.Helper()
	dir := t.TempDir()
	for name, text := range files {
		path := filepath.Join(dir, name)
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte(text), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return dir
}

var gameFolder = map[string]string{
	"left4dead2/resource/LevelTable.txt": `"LevelTable" { "Lv1" { "BaseExp" "0" } "Lv2" { "BaseExp" "150" } }`,
	"left4dead2/resource/VersusClassDataTable.txt": `"VersusClassDataTable" { "VersusClassData_0" { "pName" "F" "baseCP" "0" "bonusCP" "30"
		"aScores[0]" "4" "aScores[1]" "3" "aScores[2]" "2" "aScores[3]" "1" "aScores[4]" "0" } }`,
	"left4dead2/resource/l4d360ui_japanese.txt": `"lang" { "Tokens" {
		"L4D360UI_CampaignName_C1" "Old name"
		"L4D360UI_LevelName_COOP_C1M1" "1: Hotel" } }`,
	"update/resource/l4d360ui_tu_japanese.txt": `"lang" { "Tokens" {
		"L4D360UI_CampaignName_C1" "Dead Center" } }`,
	"left4dead2/resource/stage_solo.txt": `"Data" { "c1m1" "_hotel" }`,
}

func TestLoadFolder_GameFolder_ReadsEveryTable(t *testing.T) {
	dir := writeGameFolder(t, gameFolder)

	folder, err := game.LoadFolder(dir)

	stages := []game.Stage{{ID: 1, Name: "Dead Center", Chapters: []game.Chapter{{ID: 1, Name: "1: Hotel"}}}}
	if err != nil || folder.Levels.Level(150) != 2 || len(folder.VersusClasses) != 1 || !reflect.DeepEqual(folder.Stages, stages) {
		t.Fatalf("LoadFolder() = %+v, %v", folder, err)
	}
}

func TestLoadFolder_MalformedTable_NamesTheFile(t *testing.T) {
	files := maps.Clone(gameFolder)
	files["left4dead2/resource/LevelTable.txt"] = `"LevelTable" { }`
	dir := writeGameFolder(t, files)

	_, err := game.LoadFolder(dir)

	if err == nil || !strings.Contains(err.Error(), "LevelTable.txt") {
		t.Fatalf("error = %v, want one naming LevelTable.txt", err)
	}
}

func TestLoadFolder_InstalledGame_ReadsEveryTable(t *testing.T) {
	dir := os.Getenv("L4DS_GAME_DIR")
	if dir == "" {
		t.Skip("L4DS_GAME_DIR is not set")
	}

	folder, err := game.LoadFolder(dir)

	if err != nil || folder.Levels == nil || len(folder.VersusClasses) != 7 || len(folder.Stages) == 0 || folder.Stages[0].Name == "" {
		t.Fatalf("LoadFolder() = %+v, %v", folder, err)
	}
}
