package zet

import (
	"log/slog"
	"net/http"
	"slices"
	"strconv"
	"strings"

	"l4ds-nesica-emulator/internal/store"
)

// Lines of the login reply that carry no data: the count of each empty group (titles, dailys,
// thrus, eventprizes, comps).
const emptyGroups = 5

// defaultConfig lists the config_values of the login reply: master volume, bgm volume,
// mouse horizontal and vertical reverse, mouse sensitivity and acceleration, key map, then
// the controller, mouse button and radio bindings.
var defaultConfig = []int{
	5, 5, 0, 0, 3, 0, 1,
	0, 0, 0, 0,
	0, 0, 0, 0, 0,
	0, 0, 0, 0, 0, 0, 0, 0, 0,
}

// unusedLoginLines are the lines between the config and the stage progress: force_exit,
// todays_login_count, next_thru_login_bonus_count, zet_lose_amount and zet_lose_time.
const unusedLoginLines = 5

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

// login counts the login and answers with a success header, no bonuses, the default player
// settings and the player's stage progress.
func (s *Server) login(w http.ResponseWriter, r *http.Request) {
	playerID := formInt(r, "player_id")
	if err := s.store.RecordLogin(playerID); err != nil {
		slog.Error("record login", "player", playerID, "error", err)
	}

	lines := []string{"1", "1", "0"}
	for range emptyGroups {
		lines = append(lines, "0")
	}
	for _, value := range defaultConfig {
		lines = append(lines, strconv.Itoa(value))
	}
	for range unusedLoginLines {
		lines = append(lines, "0")
	}

	clears, err := s.store.Clears(playerID)
	if err != nil {
		slog.Error("load clears", "player", playerID, "error", err)
	}
	lines = append(lines, stageLines(clears)...)
	w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}
