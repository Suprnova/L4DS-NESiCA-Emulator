package zet

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/url"
	"slices"
	"strconv"
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

// Insert registers a player to a card from the game's key=value text, as in
// "tenpo_id=1,player_name=<urlencoded base64 of the name>,chara_id=3", and answers with the
// new player id.
func (s *Server) Insert(cardID, data string) (string, error) {
	fields := parseFields(data)
	charaID, _ := strconv.ParseInt(fields["chara_id"], 10, 64)

	player, err := s.store.Register(cardID, decodeName(fields["player_name"]), charaID)
	if err != nil {
		return "", err
	}
	return fmt.Sprintf("1\n1\n%d\n", player.ID), nil
}

// parseFields splits comma-separated key=value text.
func parseFields(data string) map[string]string {
	fields := map[string]string{}
	for pair := range strings.SplitSeq(data, ",") {
		if key, value, found := strings.Cut(pair, "="); found {
			fields[key] = value
		}
	}
	return fields
}

// decodeName reverses the game's name encoding (URL-escaped base64 of the UTF-8 name). Text
// that does not decode is returned as it came.
func decodeName(encoded string) string {
	unescaped, err := url.QueryUnescape(encoded)
	if err != nil {
		return encoded
	}
	name, err := base64.StdEncoding.DecodeString(unescaped)
	if err != nil {
		return encoded
	}
	return string(name)
}
