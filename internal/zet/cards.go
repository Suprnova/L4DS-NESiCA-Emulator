package zet

import (
	"errors"
	"fmt"
	"slices"
	"strings"

	"l4ds-nesica-emulator/internal/store"
)

// avatarLines is the number of lines in an avatar block: ten slots of id, resource id and
// color index.
const avatarLines = 30

// Select answers a card lookup with the profile of the player registered to the card. An
// unregistered card gets only the header and a player id of 0. The avatar, title and equipment
// blocks are empty, so the game uses its defaults.
func (s *Server) Select(cardID string) (string, error) {
	player, err := s.store.PlayerByCard(cardID)
	if errors.Is(err, store.ErrNotRegistered) {
		return fmt.Sprintf("1\n1\n%s\n0\n", cardID), nil
	}
	if err != nil {
		return "", err
	}

	lines := []any{
		1, 1, cardID, player.ID, player.Name,
		player.Zet, 1, player.Score, "", "", 0, 0, 0, 0,
		0, "", 1, "", player.CharaID,
	}
	lines = append(lines, slices.Repeat([]any{""}, avatarLines)...)
	lines = append(lines, 0, "", "", 0, player.Exp, 0)
	lines = append(lines, slices.Repeat([]any{""}, avatarLines)...)
	lines = append(lines, 0, 0, player.Logins, 0, 0)

	var body strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&body, "%v\n", line)
	}
	return body.String(), nil
}
