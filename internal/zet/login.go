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
// 1, which lets the reward pop-up set a weapon or skill item into a free slot, and the daily_quest
// flag 0.
var membershipLines = []string{"0", "1", "0"}

// slotLines are item_slot and weapon_slot, which open every skill item and weapon slot.
var slotLines = []string{strconv.Itoa(store.Slots), strconv.Itoa(store.Slots)}

// giftLines lists one entry of the comps or period_bonus group: its id, name, and ZG, and its
// item block (id, category, name, count, resource id, color index, replace, and rare). A weapon's
// resource id is its id; everything else has 0.
func giftLines(id int64, gift store.Gift) []string {
	resource := int64(0)
	if gift.Item.Kind == store.Weapon {
		resource = gift.Item.ItemID
	}
	return []string{strconv.FormatInt(id, 10), gift.Name, strconv.FormatInt(gift.Zet, 10),
		strconv.FormatInt(gift.Item.ItemID, 10), strconv.FormatInt(int64(gift.Item.Kind), 10), gift.Item.Name,
		strconv.FormatInt(gift.Item.Count, 10), strconv.FormatInt(resource, 10), "0", "0", "0"}
}

// compLines lists the comps group: the number of gifts queued for the player, then each one.
func compLines(gifts []store.QueuedGift) []string {
	lines := []string{strconv.Itoa(len(gifts))}
	for _, g := range gifts {
		lines = append(lines, giftLines(g.ID, g.Gift)...)
	}
	return lines
}

// periodBonusLines lists the period_bonus group: the number of grants, then each one.
func periodBonusLines(grants []Grant) []string {
	lines := []string{strconv.Itoa(len(grants))}
	for _, g := range grants {
		lines = append(lines, giftLines(int64(g.ID), g.Gift)...)
	}
	return lines
}

// deliverQueued gives a player the gifts queued for them, and returns them.
func (s *Server) deliverQueued(playerID int64) []store.QueuedGift {
	gifts, err := s.store.DeliverQueued(playerID, s.now())
	if err != nil {
		slog.Error("deliver queued gifts", "player", playerID, "error", err)
		return nil
	}
	for _, g := range gifts {
		slog.Info("queued gift delivered", "player", playerID, "gift", g.ID, "name", g.Name)
	}
	return gifts
}

// deliverGrants gives a player the grants open now that they have not had, and returns them.
func (s *Server) deliverGrants(playerID int64) []Grant {
	now := s.now()
	var delivered []Grant
	for _, g := range s.schedule.Grants {
		if !g.Open(now) {
			continue
		}
		ok, err := s.store.Deliver(playerID, int64(g.ID), now, g.Gift)
		if err != nil {
			slog.Error("deliver grant", "player", playerID, "grant", g.ID, "error", err)
			continue
		}
		if ok {
			slog.Info("grant delivered", "player", playerID, "grant", g.ID, "name", g.Name)
			delivered = append(delivered, g)
		}
	}
	return delivered
}

// login counts the login, delivers the gifts queued for the player and the open grants they have
// not had, and answers with a success header, the queued gifts (comps), the player's settings,
// their stage progress, the grants (period_bonus), and every slot open. A guest gets the default
// settings, no progress, and no gifts.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	playerID := formInt(r, "player_id")
	settings := store.DefaultSettings
	var clears []store.Clear
	var queued []store.QueuedGift
	var grants []Grant
	if !guest(playerID) {
		if err := s.store.RecordLogin(playerID); err != nil {
			slog.Error("record login", "player", playerID, "error", err)
		}
		if player, err := s.store.Player(playerID); err == nil {
			settings = player.Settings
		}
		var err error
		if clears, err = s.store.Clears(playerID); err != nil {
			slog.Error("load clears", "player", playerID, "error", err)
		}
		queued = s.deliverQueued(playerID)
		grants = s.deliverGrants(playerID)
	}

	lines := []string{"1", "1", "0"}
	for range emptyGroups {
		lines = append(lines, "0")
	}
	lines = append(lines, compLines(queued)...)
	lines = append(lines, configLines(settings)...)
	for range unusedLoginLines {
		lines = append(lines, "0")
	}

	lines = append(lines, stageLines(clears)...)
	lines = append(lines, membershipLines...)
	lines = append(lines, periodBonusLines(grants)...)
	lines = append(lines, slotLines...)
	w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}
