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
	"l4ds-nesica-emulator/internal/wire"
)

// emptyAvatarLines is the number of lines in an avatar block with nothing equipped: the id of
// each of the six slots (head, tops, bottoms and the three accessories). The game reads a
// slot's resource id, color index and replacement only when its id is not 0.
const emptyAvatarLines = 6

// Select answers a card lookup with the profile of the player registered to the card. An
// unregistered card gets the card server's unregistered status and no profile. The avatar,
// skill item, title and weapon blocks are empty, so the game uses its defaults, and
// changed_equipment is 0, so the game reads no latest equipment set.
func (s *Server) Select(cardID string) (wire.CardReply, error) {
	player, err := s.store.PlayerByCard(cardID)
	if errors.Is(err, store.ErrNotRegistered) {
		return wire.CardReply{Status: wire.CardStatusUnregistered, Body: "0\n1\n" + cardID + "\n"}, nil
	}
	if err != nil {
		return wire.CardReply{}, err
	}

	lines := []any{
		1, 1, cardID, player.ID, player.Name,
		player.Zet, s.levels.Level(player.Exp), player.Score, "", "",
		player.CoopClassPoint, player.VSClassPoint, player.CoopClassID, player.VSClassID,
		0, "", 1, "", player.CharaID,
	}
	lines = append(lines, slices.Repeat([]any{0}, emptyAvatarLines)...)
	lines = append(lines, 0, "", "", 0, player.Exp, 0)
	lines = append(lines, player.Logins, 0, 0)

	var body strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&body, "%v\n", line)
	}
	return wire.CardReply{Status: 1, Body: body.String()}, nil
}

// ReissueTest answers that no reissue is reserved for the card.
func (s *Server) ReissueTest(cardID string) (wire.CardReply, error) {
	return wire.CardReply{Status: wire.CardStatusNoReissue, Body: "0\n1\n" + cardID + "\n"}, nil
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

// Data types of a card update.
const (
	updateName     = 1 // "request_flag=0,player_name=<name encoded as in Insert>"
	updateSettings = 3 // "volume=,volume_bgm=,mouse_acce=,mouse_horizon=,mouse_vertical=,mouse_sens=,key_type="
)

// Update stores what the game writes to a card: a new player name or the player's settings.
// Other data types are ignored.
func (s *Server) Update(cardID string, dataType uint32, data string) error {
	fields := parseFields(data)
	switch dataType {
	case updateName:
		return s.store.Rename(cardID, decodeName(fields["player_name"]))
	case updateSettings:
		number := func(key string) int64 {
			value, _ := strconv.ParseInt(fields[key], 10, 64)
			return value
		}
		return s.store.UpdateSettings(cardID, store.Settings{
			MasterVolume:           number("volume"),
			BGMVolume:              number("volume_bgm"),
			MouseHorizontalReverse: number("mouse_horizon"),
			MouseVerticalReverse:   number("mouse_vertical"),
			MouseSensitivity:       number("mouse_sens"),
			MouseAcceleration:      number("mouse_acce"),
			KeyMap:                 number("key_type"),
		})
	default:
		return nil
	}
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
