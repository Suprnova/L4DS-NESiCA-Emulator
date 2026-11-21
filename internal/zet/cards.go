package zet

import (
	"encoding/base64"
	"errors"
	"fmt"
	"log/slog"
	"net/url"
	"slices"
	"strconv"
	"strings"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// Select answers a card lookup with the profile of the player registered to the card, with what
// they wear and carry: the avatar items, skill items, title, and weapons set in their slots. An
// unregistered card gets the card server's unregistered status and no profile. changed_equipment
// is 0, so the game reads no latest equipment set.
func (s *Server) Select(cardID string) (wire.CardReply, error) {
	player, err := s.store.PlayerByCard(cardID)
	if errors.Is(err, store.ErrNotRegistered) {
		return wire.CardReply{Status: wire.CardStatusUnregistered, Body: "0\n1\n" + cardID + "\n"}, nil
	}
	if err != nil {
		return wire.CardReply{}, err
	}
	equipment, err := s.store.Equipment(player.ID)
	if err != nil {
		return wire.CardReply{}, err
	}

	lines := []any{
		1, 1, cardID, player.ID, player.Name,
		player.Zet, s.levels.Level(player.Exp), player.Score, "", "",
		player.CoopClassPoint, player.VSClassPoint, player.CoopClassID, player.VSClassID,
		0, "", player.HomeStore, s.storeName(player.HomeStore), player.CharaID,
	}
	lines = append(lines, avatarLines(equipment, player.CharaID)...)
	lines = append(lines, armedLines(equipment, store.SkillItem)...)
	lines = append(lines, titleLines(equipment)...)
	lines = append(lines, armedLines(equipment, store.Weapon)...)
	lines = append(lines, player.Exp, 0)
	lines = append(lines, player.Logins, player.VSPlays, player.VSWins)
	return wire.CardReply{Status: 1, Body: lineBody(lines)}, nil
}

// emptyAvatarLines is the number of lines in an avatar block with nothing worn: a 0 id for each of
// the six slots.
const emptyAvatarLines = 6

// avatarSlots are the card reply's six avatar slots in order (head, tops, bottoms, face
// accessory, upper-body accessory, lower-body accessory): the kind and slot each takes its item
// from, and whether it has a replacement line. client.dll reads them at 0x10273cf0.
var avatarSlots = []struct {
	kind     store.ItemKind
	slot     int64
	replaced bool
}{
	{store.Head, 0, true}, {store.Upper, 2, false}, {store.Lower, 3, false},
	{store.Accessory, 1, true}, {store.Accessory, 2, true}, {store.Accessory, 3, true},
}

// avatarLines lists the card reply's avatar block for a player whose card's chara_id is chara.
// An empty slot is a 0 id. A worn one is its id, resource id, and color index, then for the
// head and the accessories a replacement id. Only a head item has a replacement: the hair its
// survivor wears under it (client.dll 0x102707d0 sets the hat as the head accessory and the
// replacement as the hair), sent as replacement id, resource id, and color index 0.
func avatarLines(equipment []store.Equipped, chara int64) []any {
	worn := map[[2]int64]store.Equipped{}
	for _, item := range equipment {
		worn[[2]int64{int64(item.Kind), item.Slot}] = item
	}
	survivor, _ := game.SurvivorOf(chara)

	var lines []any
	for _, slot := range avatarSlots {
		item, found := worn[[2]int64{int64(slot.kind), slot.slot}]
		if !found {
			lines = append(lines, 0)
			continue
		}
		resource, color := avatarParts(item.ItemID)
		lines = append(lines, item.ItemID, resource, color)
		if !slot.replaced {
			continue
		}
		hair := 0
		if head, ok := game.AvatarAt(game.SiteHead, survivor, resource); ok && slot.kind == store.Head {
			hair = head.Hair[survivor]
		}
		lines = append(lines, hair)
		if hair != 0 {
			lines = append(lines, hair, 0)
		}
	}
	return lines
}

// titleLines lists the card reply's title lines: the id and name of the title worn, or two empty
// lines for none.
func titleLines(equipment []store.Equipped) []any {
	for _, item := range equipment {
		if item.Kind == store.Title {
			return []any{item.ItemID, item.Name}
		}
	}
	return []any{"", ""}
}

// armedLines lists the card reply's block of the slots of kind: the number of slots, then each
// slot's number, item id, name and count, an empty slot's as 0, "", and 0. A weapon adds its
// resource id, which is its item id, and its color index, which is always 0. Every slot is listed,
// as the login opens every slot: the card screen, shown before login, opens as many weapon slots
// as the block lists (client.dll 0x10274c40 keeps the count at +0x928, which 0x1026ef90 compares
// each slot with), and the skill slots it lists (0x10270250).
func armedLines(equipment []store.Equipped, kind store.ItemKind) []any {
	lines := []any{store.Slots}
	for slot := range int64(store.Slots) {
		i := slices.IndexFunc(equipment, func(item store.Equipped) bool { return item.Kind == kind && item.Slot == slot+1 })
		var item store.Equipped
		if i >= 0 {
			item = equipment[i]
		}
		lines = append(lines, slot+1, item.ItemID, item.Name, item.Count)
		if kind == store.Weapon {
			lines = append(lines, item.ItemID, 0)
		}
	}
	return lines
}

// lineBody returns lines as a reply body, one value per line.
func lineBody(lines []any) string {
	var body strings.Builder
	for _, line := range lines {
		fmt.Fprintf(&body, "%v\n", line)
	}
	return body.String()
}

// ReissueTest answers that no reissue is reserved for the card.
func (s *Server) ReissueTest(cardID string) (wire.CardReply, error) {
	slog.Info("stub reply", "command", "CARD_REISSUE_TEST", "ignored", "card reissues: a new card always registers a new player")
	return wire.CardReply{Status: wire.CardStatusNoReissue, Body: "0\n1\n" + cardID + "\n"}, nil
}

// Insert registers a player to a card from the game's key=value text, as in
// "tenpo_id=1,player_name=<urlencoded base64 of the name>,chara_id=3", and answers with the
// new player id. The store the card is registered at becomes the player's home store.
func (s *Server) Insert(cardID, data string) (string, error) {
	fields := parseFields(data)
	charaID, _ := strconv.ParseInt(fields["chara_id"], 10, 64)
	homeStore, _ := strconv.ParseInt(fields["tenpo_id"], 10, 64)

	player, err := s.store.Register(cardID, decodeName(fields["player_name"]), charaID, homeStore)
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
// Other data types are logged and ignored.
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
		slog.Warn("unhandled card update", "card", cardID, "type", dataType, "data", data)
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
