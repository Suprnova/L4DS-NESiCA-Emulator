package zet

import (
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// emptyGroups is the number of groups the login reply sends empty, each as a count of 0: titles,
// dailys, thrus, and eventprizes. comps follows them.
const emptyGroups = 4

// defaultBindings is the number of config_values after the settings: the controller, mouse
// button and radio bindings, which the cabinet never changes. 0 keeps the game's defaults.
const defaultBindings = 18

// configLines lists the config_values of the login reply: the player's settings (master
// volume, bgm volume, mouse horizontal and vertical reverse, mouse sensitivity and
// acceleration, key map), then the bindings.
func configLines(settings store.Settings) []string {
	values := []int64{
		settings.MasterVolume, settings.BGMVolume, settings.MouseHorizontalReverse, settings.MouseVerticalReverse,
		settings.MouseSensitivity, settings.MouseAcceleration, settings.KeyMap,
	}
	lines := slices.Repeat([]string{"0"}, len(values)+defaultBindings)
	for i, value := range values {
		lines[i] = strconv.FormatInt(value, 10)
	}
	return lines
}

// unusedLoginLines are the lines between the config and the stage progress: force_exit,
// todays_login_count, next_thru_login_bonus_count and zet_lose_amount. The game reads
// zet_lose_time only when the amount is above zero.
const unusedLoginLines = 4

// stageLines lists the stage_infos group: the number of stages, then for each stage its id,
// cleared flag, highest reached chapter, remaining title count and chapter count, followed by
// each chapter's id, cleared flag and remaining title count.
func stageLines(clears []store.Clear) []string {
	var stages []int64
	chapters := map[int64][]int64{}
	for _, clear := range clears {
		if _, seen := chapters[clear.StageID]; !seen {
			stages = append(stages, clear.StageID)
		}
		chapters[clear.StageID] = append(chapters[clear.StageID], clear.ChapterID)
	}

	lines := []string{strconv.Itoa(len(stages))}
	for _, stage := range stages {
		cleared := chapters[stage]
		lines = append(lines, strconv.FormatInt(stage, 10), "1", strconv.FormatInt(slices.Max(cleared), 10), "0", strconv.Itoa(len(cleared)))
		for _, chapter := range cleared {
			lines = append(lines, strconv.FormatInt(chapter, 10), "1", "0")
		}
	}
	return lines
}

// membershipLines are the lines after the stage progress: release_difficulty 0, paid_membersihp
// 1, which lets the reward pop-up set an item into a free slot, and the daily_quest flag 0.
var membershipLines = []string{"0", "1", "0"}

// slotLines are item_slot and weapon_slot, which open every skill item and weapon slot.
var slotLines = []string{strconv.Itoa(store.Slots), strconv.Itoa(store.Slots)}

// delivered is a gift given at this login: a queued gift's id or a grant's, and what it gave.
type delivered struct {
	id   int64
	gift store.Gift
}

// giftGroupLines lists the comps or period_bonus group: the number of gifts, then for each its
// id, name, and ZG, and its item block (id, category, name, count, resource id, color index,
// replace, and rare). A weapon's resource id is its id, an avatar item's resource id and color
// are the ones its id carries, and everything else has 0 for both. Replace and rare are 0: a
// nonzero rare on an avatar item makes the pop-up take the next entry as its partner (GameUI
// 0x10058850).
func giftGroupLines(gifts []delivered) []string {
	lines := []string{strconv.Itoa(len(gifts))}
	for _, d := range gifts {
		item := d.gift.Item
		resource, color := 0, 0
		switch {
		case item.Kind == store.Weapon:
			resource = int(item.ItemID)
		case item.Kind.Wearable():
			resource, color = avatarParts(item.ItemID)
		}
		lines = append(lines, strconv.FormatInt(d.id, 10), d.gift.Name, strconv.FormatInt(d.gift.Zet, 10),
			strconv.FormatInt(item.ItemID, 10), strconv.FormatInt(int64(item.Kind), 10), item.Name,
			strconv.FormatInt(item.Count, 10), strconv.Itoa(resource), strconv.Itoa(color), "0", "0")
	}
	return lines
}

// deliverQueued gives a player the gifts queued for them, and returns them.
func (s *Server) deliverQueued(playerID int64) []delivered {
	gifts, err := s.store.DeliverQueued(playerID, s.now())
	if err != nil {
		slog.Error("deliver queued gifts", "player", playerID, "error", err)
		return nil
	}
	var given []delivered
	for _, g := range gifts {
		slog.Info("queued gift delivered", "player", playerID, "gift", g.ID, "name", g.Name)
		given = append(given, delivered{g.ID, g.Gift})
	}
	return given
}

// deliverGrants gives a player, whose card's chara_id is chara, the grants open now that they have
// not had, and returns them. An avatar item their survivor cannot wear is left out of its grant.
func (s *Server) deliverGrants(playerID, chara int64) []delivered {
	now := s.now()
	var given []delivered
	for _, g := range s.schedule.Grants {
		if !g.Open(now) {
			continue
		}
		gift, fits := g.For(chara)
		ok, err := s.store.Deliver(playerID, int64(g.ID), now, gift)
		if err != nil {
			slog.Error("deliver grant", "player", playerID, "grant", g.ID, "error", err)
			continue
		}
		if !ok {
			continue
		}
		slog.Info("grant delivered", "player", playerID, "grant", g.ID, "name", g.Name)
		if !fits {
			slog.Warn("grant's avatar item does not fit the player's survivor; left out", "player", playerID, "grant", g.ID, "item", g.Avatar.Name)
		}
		given = append(given, delivered{int64(g.ID), gift})
	}
	return given
}

// login counts the login, delivers the gifts queued for the player and the open grants they have
// not had, and answers with a success header, the queued gifts (comps), the player's settings,
// their stage progress, the grants (period_bonus), and every slot open. A guest gets the default
// settings, no progress, and no gifts.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	playerID := formInt(r, "player_id")
	settings := store.DefaultSettings
	var clears []store.Clear
	var queued, grants []delivered
	if !guest(playerID) {
		if err := s.store.RecordLogin(playerID); err != nil {
			slog.Error("record login", "player", playerID, "error", err)
		}
		player, err := s.store.Player(playerID)
		if err == nil {
			settings = player.Settings
		}
		if clears, err = s.store.Clears(playerID); err != nil {
			slog.Error("load clears", "player", playerID, "error", err)
		}
		queued = s.deliverQueued(playerID)
		grants = s.deliverGrants(playerID, player.CharaID)
	}

	lines := []string{"1", "1", "0"}
	for range emptyGroups {
		lines = append(lines, "0")
	}
	lines = append(lines, giftGroupLines(queued)...)
	lines = append(lines, configLines(settings)...)
	for range unusedLoginLines {
		lines = append(lines, "0")
	}

	lines = append(lines, stageLines(clears)...)
	lines = append(lines, membershipLines...)
	lines = append(lines, giftGroupLines(grants)...)
	lines = append(lines, slotLines...)
	w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}
