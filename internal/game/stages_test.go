package game_test

import (
	"bytes"
	"encoding/binary"
	"reflect"
	"strings"
	"testing"
	"unicode/utf16"

	"l4ds-nesica-emulator/internal/game"
)

func TestParseStages_OrdersStagesAndChaptersByNumber(t *testing.T) {
	table := `"Data" { "c10m1" "_caves" "c2m2" "_fairgrounds" "c2m1" "_highway" // c9m1
	}`
	tokens := map[string]string{
		"L4D360UI_CampaignName_C2":          "Dark Carnival",
		"L4D360UI_LevelName_COOP_C2M1":      "1: Highway",
		"L4D360UI_LevelName_COOP_C10M1":     "1: The Turnpike",
		"L4D360UI_CampaignName_C10":         "Death Toll",
		"[english]L4D360UI_CampaignName_C2": "unused",
	}

	stages, err := game.ParseStages(strings.NewReader(table), tokens)

	want := []game.Stage{
		{ID: 2, Name: "Dark Carnival", Chapters: []game.Chapter{{ID: 1, Name: "1: Highway"}, {ID: 2}}},
		{ID: 10, Name: "Death Toll", Chapters: []game.Chapter{{ID: 1, Name: "1: The Turnpike"}}},
	}
	if err != nil || !reflect.DeepEqual(stages, want) {
		t.Fatalf("ParseStages() = %+v, %v", stages, err)
	}
}

func TestParseStages_NoMaps_ReturnsAnError(t *testing.T) {
	_, err := game.ParseStages(strings.NewReader(`"Data" { }`), nil)

	if err == nil {
		t.Fatal("ParseStages() error = nil")
	}
}

func TestParseTokens_Utf16File_ReplacesEarlierTokens(t *testing.T) {
	text := "\"lang\"\r\n{\r\n\"Tokens\"\r\n{\r\n\t\"L4D360UI_CampaignName_C1\"\t\"デッド・センター\"\r\n\t\"Quoted\"\t\"say \\\"hi\\\"\"\r\n\"Adjacent\"\"no gap\"\r\n}\r\n}\r\n"
	var file bytes.Buffer
	file.Write([]byte{0xff, 0xfe})
	binary.Write(&file, binary.LittleEndian, utf16.Encode([]rune(text)))
	tokens := map[string]string{"L4D360UI_CampaignName_C1": "old", "Other": "kept"}

	err := game.ParseTokens(&file, tokens)

	want := map[string]string{"L4D360UI_CampaignName_C1": "デッド・センター", "Quoted": `say \"hi\"`, "Adjacent": "no gap", "Other": "kept"}
	if err != nil || !reflect.DeepEqual(tokens, want) {
		t.Fatalf("tokens = %q, %v", tokens, err)
	}
}
