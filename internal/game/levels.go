// Package game holds the arcade game's rules and the tables build 1.26 ships with.
package game

import "slices"

// LevelTable maps experience to a player level.
type LevelTable struct {
	baseExp []int64
}

// Level returns the level reached with exp. A nil table has no levels, so everyone is level 1.
func (t *LevelTable) Level(exp int64) int {
	if t == nil {
		return 1
	}
	reached, _ := slices.BinarySearch(t.baseExp, exp+1)
	return max(reached, 1)
}
