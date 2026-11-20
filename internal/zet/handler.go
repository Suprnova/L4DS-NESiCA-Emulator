// Package zet implements the ZET game server: the HTTP endpoints the game reaches through
// the NESYS tunnel, and the card lookups that share its player data.
package zet

import (
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// Server is the ZET game server.
type Server struct {
	store     *store.Store
	levels    *game.LevelTable
	stages    []game.Stage
	classes   []game.VersusClass
	schedule  Schedule
	storeName func(storeID int64) string
	leave     func(playerID int64, roomID uint32)
	now       func() time.Time
	handler   http.Handler
}

// Catalog is what the server offers players besides their own data.
type Catalog struct {
	// Levels derives player levels from experience; nil makes every player level 1.
	Levels *game.LevelTable
	// Stages are offered in every mode; with none, the game falls back to its own stage table.
	Stages []game.Stage
	// VersusClasses are sent as the class table's versus classes. The game needs them to record
	// a versus round's class points: without them it crashes at the end of the round.
	VersusClasses []game.VersusClass
	// Schedule holds the versus schedule and the events; without them versus and the event
	// modes stay unavailable.
	Schedule Schedule
	// StoreName returns the name of the store with the given id, for players' home stores on
	// their cards and in rankings; "" for an unknown store. When nil, no store has a name.
	StoreName func(storeID int64) string
	// Leave is told when a player leaves the matching room they were in; it may be nil.
	Leave func(playerID int64, roomID uint32)
}

// NewServer returns a server that keeps its players in s and offers catalog.
func NewServer(s *store.Store, catalog Catalog) *Server {
	server := &Server{store: s, levels: catalog.Levels, stages: catalog.Stages, classes: catalog.VersusClasses, schedule: catalog.Schedule, storeName: catalog.StoreName, leave: catalog.Leave, now: time.Now}
	if server.storeName == nil {
		server.storeName = func(int64) string { return "" }
	}
	server.handler = server.routes()
	return server
}

// Handler returns the server's HTTP handler, which logs every request with its body.
func (s *Server) Handler() http.Handler { return s.handler }

func (s *Server) routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sendversion", reply(successHeader))
	mux.HandleFunc("POST /version", reply(successHeader))
	mux.HandleFunc("POST /login", s.login)
	mux.HandleFunc("POST /loading_start", stub("play log: mode, rule, stage, chapter, difficulty, svp"))
	mux.HandleFunc("POST /prestage", stub("reply's friend invite rewards"))
	mux.HandleFunc("POST /chapter_start", stub("play log: mode, rule, stage, chapter, difficulty, svp"))
	mux.HandleFunc("POST /chapter_end", stub("play log: mode, rule, stage, chapter, difficulty, svp"))
	mux.HandleFunc("POST /result", s.result)
	mux.HandleFunc("POST /close", stub("ex_svp and zet (leftover SVP converted to ZG), reply's info lines"))
	mux.HandleFunc("POST /svp", svp)
	mux.HandleFunc("POST /stages", s.stageList)
	mux.HandleFunc("POST /versus", s.versusList)
	mux.HandleFunc("POST /event", s.eventList)
	mux.HandleFunc("POST /class_table", s.classTable)
	mux.HandleFunc("POST /select", stub("matching room choice: player_id, room_id"))
	mux.HandleFunc("POST /leave", s.leaveRoom)
	mux.HandleFunc("POST /equip", s.equip)
	mux.HandleFunc("/", unanswered)
	return logRequests(mux)
}

// successHeader is the reply header for a request that succeeded: flag, result 1 and API_ID.
const successHeader = "1\n1\n0\n"

// stub answers with a bare success header where the official server did more, and logs what
// the request carried or the reply lacks, so the gap shows while playing.
func stub(ignored string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		slog.Info("stub reply", "path", r.URL.Path, "ignored", ignored)
		w.Write([]byte(successHeader))
	}
}

// unanswered logs a request the server has no handler for and answers 404, which the game
// treats as a failed request.
func unanswered(w http.ResponseWriter, r *http.Request) {
	slog.Warn("unanswered request", "path", r.URL.Path)
	http.NotFound(w, r)
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
	w.Write([]byte(successHeader + r.PostFormValue("player_id") + "\n"))
}

// result stores the finished chapter with the time it arrived, adds it to the player's totals,
// and answers with a success header. class_point is the change in the player's class points:
// versus points when game_mode, the matching mode, is a versus mode (7 to 10), co-op points
// otherwise. A versus result also counts a versus play, and a win when vs_you_win is 1. A cleared
// chapter counts towards the player's stage progress only when campaign_flag is 1, which the game
// sets for co-op and single play (modes 1 and 5), not for events or versus. A guest's result is
// not kept.
func (s *Server) result(w http.ResponseWriter, r *http.Request) {
	defer w.Write([]byte(successHeader))
	playerID := formInt(r, "player_id")
	if guest(playerID) {
		return
	}

	result := store.Result{
		Played:      s.now(),
		GameMode:    formInt(r, "game_mode"),
		StageID:     formInt(r, "stage_id"),
		ChapterID:   formInt(r, "chapter_id"),
		Cleared:     formInt(r, "clear_flag") == 1 && formInt(r, "campaign_flag") == 1,
		Zet:         formInt(r, "zet"),
		Exp:         formInt(r, "exp"),
		Score:       formInt(r, "TOTAL_SCORE"),
		CoopClassID: formInt(r, "coop_class_id"),
		VSClassID:   formInt(r, "vs_class_id"),
	}
	if result.GameMode >= 7 && result.GameMode <= 10 {
		result.VSClassPoint = formInt(r, "class_point")
		result.VSPlays, result.VSWins = 1, formInt(r, "vs_you_win")
	} else {
		result.CoopClassPoint = formInt(r, "class_point")
	}

	if err := s.store.RecordResult(playerID, result); err != nil {
		slog.Error("record result", "player", playerID, "error", err)
	}
}

// leaveRoom tells the matching master that a player left a room, and answers with a success
// header.
func (s *Server) leaveRoom(w http.ResponseWriter, r *http.Request) {
	if s.leave != nil {
		s.leave(formInt(r, "player_id"), uint32(formInt(r, "room_id")))
	}
	w.Write([]byte(successHeader))
}

// guest reports whether a player id is the one the game uses when no card is in use (-1).
func guest(playerID int64) bool { return playerID < 1 }

func formInt(r *http.Request, key string) int64 {
	value, _ := strconv.ParseInt(r.PostFormValue(key), 10, 64)
	return value
}
