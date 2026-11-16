// Package game holds the arcade game's rules, driven by the tables the game ships with.
package game

import (
	"errors"
	"fmt"
	"io"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	token      = regexp.MustCompile(`"([^"]*)"|[{}]|//[^\n]*`)
	levelEntry = regexp.MustCompile(`^Lv(\d+)$`)
)

// LevelTable maps experience to a player level.
type LevelTable struct {
	baseExp []int64
}

// ParseLevelTable reads the game's LevelTable.txt: a KeyValues file with one "LvN" block per
// level holding that level's "BaseExp", the experience at which it starts.
func ParseLevelTable(r io.Reader) (*LevelTable, error) {
	text, err := io.ReadAll(r)
	if err != nil {
		return nil, err
	}

	var tokens []string
	for _, match := range token.FindAllStringSubmatch(string(text), -1) {
		if len(match[0]) > 1 && match[0][:2] == "//" {
			continue
		}
		tokens = append(tokens, match[0])
	}

	baseExp := map[int]int64{}
	for i := 0; i+2 < len(tokens); i++ {
		level := levelEntry.FindStringSubmatch(unquote(tokens[i]))
		if level == nil || tokens[i+1] != "{" {
			continue
		}
		number, _ := strconv.Atoi(level[1])
		for j := i + 2; j+1 < len(tokens) && tokens[j] != "}"; j += 2 {
			if unquote(tokens[j]) == "BaseExp" {
				baseExp[number], err = strconv.ParseInt(unquote(tokens[j+1]), 10, 64)
				if err != nil {
					return nil, fmt.Errorf("level %d: %w", number, err)
				}
			}
		}
	}

	table := &LevelTable{}
	for level := 1; ; level++ {
		exp, found := baseExp[level]
		if !found {
			break
		}
		table.baseExp = append(table.baseExp, exp)
	}
	if len(table.baseExp) == 0 {
		return nil, errors.New("game: level table has no levels")
	}
	if !slices.IsSorted(table.baseExp) {
		return nil, errors.New("game: level table experience is not ascending")
	}
	return table, nil
}

// Level returns the level reached with exp. A nil table has no levels, so everyone is level 1.
func (t *LevelTable) Level(exp int64) int {
	if t == nil {
		return 1
	}
	reached, _ := slices.BinarySearch(t.baseExp, exp+1)
	return max(reached, 1)
}

func unquote(token string) string { return strings.Trim(token, `"`) }
