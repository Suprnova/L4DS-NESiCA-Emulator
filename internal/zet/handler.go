// Package zet implements the ZET game server: the HTTP endpoints the game reaches through
// the NESYS tunnel, and the card lookups that share its player data.
package zet

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"l4ds-nesica-emulator/internal/game"
	"l4ds-nesica-emulator/internal/store"
)

// Server is the ZET game server.
type Server struct {
	store  *store.Store
	levels *game.LevelTable
	stages []game.Stage
}

// NewServer returns a server that keeps its players in s, derives their levels from levels and
// offers stages in every mode. A nil table makes every player level 1; with no stages the game
// falls back to its own stage table.
func NewServer(s *store.Store, levels *game.LevelTable, stages []game.Stage) *Server {
	return &Server{store: s, levels: levels, stages: stages}
}

// Handler returns the server's HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sendversion", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /version", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /loading_start", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /prestage", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /chapter_start", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /chapter_end", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /result", s.result)
	mux.HandleFunc("POST /close", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /svp", svp)
	mux.HandleFunc("POST /stages", s.stageList)
	return mux
}

// stageKinds are the stage lists the game keeps, one per way of playing: national co-op, store
// co-op, clan co-op and single play.
var stageKinds = []int{1, 2, 3, 4}

// stageList answers the stage list the game fetches at startup: each stage in every kind, with
// its id, kind, "new" flag, name and chapters (id and name). The stage select offers these
// stages, opening each chapter once the one before it is cleared. Every stage is flagged new:
// the game marks an uncleared chapter of a new stage "NEW", and a stage "NEW" while none of its
// chapters is cleared, "CLEAR" once all are, and nothing in between.
func (s *Server) stageList(w http.ResponseWriter, _ *http.Request) {
	lines := []string{"1", "1", "0", strconv.Itoa(len(s.stages) * len(stageKinds))}
	for _, kind := range stageKinds {
		for _, stage := range s.stages {
			lines = append(lines, strconv.Itoa(stage.ID), strconv.Itoa(kind), "1", stage.Name, strconv.Itoa(len(stage.Chapters)))
			for _, chapter := range stage.Chapters {
				lines = append(lines, strconv.Itoa(chapter.ID), chapter.Name)
			}
		}
	}
	w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

// reply answers every request with a fixed body.
func reply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}
}

// svp answers with a success header followed by the player id the game sent, which the game
// uses as its player id from then on.
func svp(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("1\n1\n0\n" + r.PostFormValue("player_id") + "\n"))
}

// result adds the finished chapter to the player's totals and answers with a success header.
// class_point is the change in the player's class points; the game modes it reports are all
// co-op, so it goes to the co-op class.
func (s *Server) result(w http.ResponseWriter, r *http.Request) {
	playerID, _ := strconv.ParseInt(r.PostFormValue("player_id"), 10, 64)
	result := store.Result{
		Zet:            formInt(r, "zet"),
		Exp:            formInt(r, "exp"),
		Score:          formInt(r, "TOTAL_SCORE"),
		CoopClassPoint: formInt(r, "class_point"),
		CoopClassID:    formInt(r, "coop_class_id"),
		VSClassID:      formInt(r, "vs_class_id"),

		StageID:   formInt(r, "stage_id"),
		ChapterID: formInt(r, "chapter_id"),
		Cleared:   formInt(r, "clear_flag") == 1,
	}

	if err := s.store.RecordResult(playerID, result); err != nil {
		slog.Error("record result", "player", playerID, "error", err)
	}
	w.Write([]byte("1\n1\n0\n"))
}

func formInt(r *http.Request, key string) int64 {
	value, _ := strconv.ParseInt(r.PostFormValue(key), 10, 64)
	return value
}
