package game

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"maps"
	"regexp"
	"slices"
	"strconv"
	"unicode/utf16"
)

var (
	mapName       = regexp.MustCompile(`^c(\d+)m(\d+)$`)
	localizedLine = regexp.MustCompile(`(?m)^\s*"([^"]+)"\s*"((?:[^"\\]|\\.)*)"`)
)

// Stage is a campaign the game can play, with its chapters in order.
type Stage struct {
	ID       int
	Name     string
	Chapters []Chapter
}

// Chapter is one map of a stage.
type Chapter struct {
	ID   int
	Name string
}

// ParseStages reads the game's stage map table (resource/stage_solo.txt, a KeyValues block
// with one "cXmY" key per chapter) and names each stage and chapter from tokens, the game's
// localized text: "L4D360UI_CampaignName_CX" and "L4D360UI_LevelName_COOP_CXMY". Stages and
// chapters are ordered by number.
func ParseStages(r io.Reader, tokens map[string]string) ([]Stage, error) {
	text, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	chapters := map[int][]int{}
	for _, match := range token.FindAllString(string(text), -1) {
		found := mapName.FindStringSubmatch(unquote(match))
		if found == nil {
			continue
		}
		stage, _ := strconv.Atoi(found[1])
		chapter, _ := strconv.Atoi(found[2])
		chapters[stage] = append(chapters[stage], chapter)
	}
	if len(chapters) == 0 {
		return nil, errors.New("game: stage table has no maps")
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
	return stages, nil
}

// ParseTokens reads a localization file, UTF-16 with a byte order mark or UTF-8, and adds its
// tokens to tokens, replacing those already there.
func ParseTokens(r io.Reader, tokens map[string]string) error {
	data, err := io.ReadAll(r)
	if err != nil {
		return err
	}
	if bytes.HasPrefix(data, []byte{0xff, 0xfe}) {
		units := make([]uint16, (len(data)-2)/2)
		binary.Decode(data[2:], binary.LittleEndian, units)
		data = []byte(string(utf16.Decode(units)))
	}

	for _, match := range localizedLine.FindAllStringSubmatch(string(data), -1) {
		tokens[match[1]] = match[2]
	}
	return nil
}
