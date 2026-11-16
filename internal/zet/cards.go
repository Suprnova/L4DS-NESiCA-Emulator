package zet

import (
	"fmt"
	"strings"
)

// Cards is the card registry. It currently registers every card to the same player.
type Cards struct{}

const (
	playerID   = 1
	playerName = "Survivor"
)

// Select answers a card lookup with the profile of the registered player. Lines after the
// character id (avatar, equipment and history) are left out, which the game reads as empty.
func (Cards) Select(cardID string) string {
	lines := []any{
		1, 1, cardID, playerID, playerName,
		0, 1, 0, "", "", 0, 0, 0, 0,
		0, "", 1, "", 0,
	}

	var body strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&body, "%v\n", line)
	}
	return body.String()
}
