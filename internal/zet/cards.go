package zet

import (
	"fmt"
	"strings"
)

// Select answers a card lookup with the profile of the player registered to the card,
// registering a new player for an unknown card. Lines after the character id (avatar,
// equipment and history) are left out, which the game reads as empty.
func (s *Server) Select(cardID string) (string, error) {
	player, err := s.store.PlayerByCard(cardID)
	if err != nil {
		return "", err
	}

	lines := []any{
		1, 1, cardID, player.ID, player.Name,
		player.Zet, 1, player.Score, "", "", 0, 0, 0, 0,
		0, "", 1, "", 0,
	}

	var body strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&body, "%v\n", line)
	}
	return body.String(), nil
}
