package game

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"maps"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"unicode/utf16"
)

var (
	token         = regexp.MustCompile(`"([^"]*)"|[{}]|//[^\n]*`)
	levelEntry    = regexp.MustCompile(`^Lv(\d+)$`)
	mapName       = regexp.MustCompile(`^c(\d+)m(\d+)$`)
	localizedLine = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*"((?:[^"\\]|\\.)*)"`)
)

func TestTables_MatchInstalledGame(t *testing.T) {
	dir := os.Getenv("L4DS_GAME_DIR")
	if dir == "" {
		t.Skip("L4DS_GAME_DIR is not set")
	}
	resource := filepath.Join(dir, "left4dead2", "resource")
	tokens := map[string]string{}
	for _, name := range []string{
		filepath.Join(resource, "l4d360ui_japanese.txt"),
		filepath.Join(dir, "update", "resource", "l4d360ui_tu_japanese.txt"),
	} {
		addTokens(readText(t, name), tokens)
	}

	levels := parseLevels(readText(t, filepath.Join(resource, "LevelTable.txt")))
	classes := parseVersusClasses(t, readText(t, filepath.Join(resource, "VersusClassDataTable.txt")))
	stages := parseStages(readText(t, filepath.Join(resource, "stage_solo.txt")), tokens)

	if !slices.Equal(levels, Levels.baseExp) {
		t.Error("Levels differ from LevelTable.txt")
	}
	if !reflect.DeepEqual(classes, VersusClasses) {
		t.Errorf("VersusClasses differ from VersusClassDataTable.txt: %+v", classes)
	}
	if !reflect.DeepEqual(stages, Stages) {
		t.Errorf("Stages differ from stage_solo.txt and the stage names: %+v", stages)
	}
}

func TestLevelTable_Level_ReturnsTheHighestLevelStarted(t *testing.T) {
	for exp, want := range map[int64]int{0: 1, 149: 1, 150: 2, 319: 2, 320: 3, 99999999: 999} {
		if got := Levels.Level(exp); got != want {
			t.Errorf("Level(%d) = %d, want %d", exp, got, want)
		}
	}
}

func TestLevelTable_NilTable_EveryoneIsLevelOne(t *testing.T) {
	var levels *LevelTable

	got := levels.Level(5000)

	if got != 1 {
		t.Fatalf("Level() = %d, want 1", got)
	}
}

// readText reads a game file: UTF-16 with a byte order mark, or UTF-8.
func readText(t *testing.T, path string) string {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) {
		units := make([]uint16, (len(data)-2)/2)
		binary.Decode(data[2:], binary.LittleEndian, units)
		return string(utf16.Decode(units))
	}
	return string(data)
}

// keyValueBlocks reads the innermost blocks of a KeyValues file, by name: each "name" { ... }
// whose body holds only "key" "value" pairs.
func keyValueBlocks(text string) map[string]map[string]string {
	var tokens []string
	for _, match := range token.FindAllString(text, -1) {
		if !strings.HasPrefix(match, "//") {
			tokens = append(tokens, strings.Trim(match, `"`))
		}
	}

	blocks := map[string]map[string]string{}
	for i := 0; i+1 < len(tokens); i++ {
		if tokens[i+1] != "{" || tokens[i] == "{" || tokens[i] == "}" {
			continue
		}
		block := map[string]string{}
		j := i + 2
		for ; j+1 < len(tokens) && tokens[j] != "}" && tokens[j] != "{" && tokens[j+1] != "{"; j += 2 {
			block[tokens[j]] = tokens[j+1]
		}
		if j < len(tokens) && tokens[j] == "}" {
			blocks[tokens[i]] = block
		}
	}
	return blocks
}

// parseLevels reads LevelTable.txt: one "LvN" block per level with the "BaseExp" it starts at.
func parseLevels(text string) []int64 {
	baseExp := map[int]int64{}
	for name, block := range keyValueBlocks(text) {
		if level := levelEntry.FindStringSubmatch(name); level != nil {
			number, _ := strconv.Atoi(level[1])
			baseExp[number], _ = strconv.ParseInt(block["BaseExp"], 10, 64)
		}
	}
	levels := make([]int64, len(baseExp))
	for number, exp := range baseExp {
		levels[number-1] = exp
	}
	return levels
}

// parseVersusClasses reads VersusClassDataTable.txt: one "VersusClassData_N" block per class.
func parseVersusClasses(t *testing.T, text string) []VersusClass {
	blocks := keyValueBlocks(text)
	var classes []VersusClass
	for n := 0; ; n++ {
		block, found := blocks[fmt.Sprintf("VersusClassData_%d", n)]
		if !found {
			return classes
		}
		class := VersusClass{Name: block["pName"]}
		numbers := map[string]*int{"baseCP": &class.BaseCP, "bonusCP": &class.BonusCP}
		for i := range class.Scores {
			numbers[fmt.Sprintf("aScores[%d]", i)] = &class.Scores[i]
		}
		for key, value := range numbers {
			var err error
			if *value, err = strconv.Atoi(block[key]); err != nil {
				t.Fatalf("versus class %d %s: %v", n, key, err)
			}
		}
		classes = append(classes, class)
	}
}

// parseStages reads stage_solo.txt, one "cXmY" key per chapter, and names the stages and
// chapters from tokens.
func parseStages(text string, tokens map[string]string) []Stage {
	chapters := map[int][]int{}
	for _, match := range token.FindAllString(text, -1) {
		if found := mapName.FindStringSubmatch(strings.Trim(match, `"`)); found != nil {
			stage, _ := strconv.Atoi(found[1])
			chapter, _ := strconv.Atoi(found[2])
			chapters[stage] = append(chapters[stage], chapter)
		}
	}

	var stages []Stage
	for _, id := range slices.Sorted(maps.Keys(chapters)) {
		stage := Stage{ID: id, Name: tokens[fmt.Sprintf("L4D360UI_CampaignName_C%d", id)]}
		slices.Sort(chapters[id])
		for _, chapter := range slices.Compact(chapters[id]) {
			stage.Chapters = append(stage.Chapters, Chapter{
				ID:   chapter,
				Name: tokens[fmt.Sprintf("L4D360UI_LevelName_COOP_C%dM%d", id, chapter)],
			})
		}
		stages = append(stages, stage)
	}
	return stages
}

// addTokens adds a localization file's tokens to tokens, replacing those already there.
func addTokens(text string, tokens map[string]string) {
	for _, match := range localizedLine.FindAllStringSubmatch(text, -1) {
		tokens[match[1]] = match[2]
	}
}
