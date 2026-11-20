package zet

import (
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// Lines of the login reply that carry no data: the count of each empty group (titles, dailys,
// thrus, eventprizes, comps).
const emptyGroups = 5

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

// slotLines are the lines after the stage progress: release_difficulty, paid_membersihp, the
// daily_quest flag and the period_bonus count, all 0, then item_slot and weapon_slot, which open
// every skill item and weapon slot.
var slotLines = []string{"0", "0", "0", "0", strconv.Itoa(store.Slots), strconv.Itoa(store.Slots)}

// login counts the login and answers with a success header, no bonuses, the player's settings,
// their stage progress, and every slot open. A guest gets the default settings and no progress.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	playerID := formInt(r, "player_id")
	settings := store.DefaultSettings
	var clears []store.Clear
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
	}

	lines := []string{"1", "1", "0"}
	for range emptyGroups {
		lines = append(lines, "0")
	}
	lines = append(lines, configLines(settings)...)
	for range unusedLoginLines {
		lines = append(lines, "0")
	}

	lines = append(lines, stageLines(clears)...)
	lines = append(lines, slotLines...)
	w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}
