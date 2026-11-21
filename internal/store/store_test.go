package store_test

import (
	"database/sql"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	_ "modernc.org/sqlite"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

func openStore(t *testing.T) *store.Store {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

const card = "7020392000000100"

func register(t *testing.T, s *store.Store) store.Player {
	t.Helper()
	player, err := s.Register(card, "Mika", 3, 1)
	if err != nil {
		t.Fatal(err)
	}
	return player
}

func TestPlayerByCard_UnknownCard_ReturnsErrNotRegistered(t *testing.T) {
	s := openStore(t)

	_, err := s.PlayerByCard(card)

	if !errors.Is(err, store.ErrNotRegistered) {
		t.Fatalf("error = %v, want ErrNotRegistered", err)
	}
}

func TestPlayer_UnknownID_ReturnsErrNoPlayer(t *testing.T) {
	s := openStore(t)

	_, err := s.Player(42)

	if !errors.Is(err, store.ErrNoPlayer) {
		t.Fatalf("error = %v, want ErrNoPlayer", err)
	}
}

func TestRecordLogin_UnknownID_ReturnsErrNoPlayer(t *testing.T) {
	s := openStore(t)

	err := s.RecordLogin(42, time.Now())

	if !errors.Is(err, store.ErrNoPlayer) {
		t.Fatalf("error = %v, want ErrNoPlayer", err)
	}
}

func TestRegister_NewCard_CreatesThePlayer(t *testing.T) {
	s := openStore(t)

	player, err := s.Register(card, "Mika", 3, 2)

	found, _ := s.PlayerByCard(card)
	if err != nil || player.ID == 0 || found.ID != player.ID || found.Name != "Mika" || found.CharaID != 3 || found.HomeStore != 2 {
		t.Fatalf("Register() = %+v, %v; found %+v", player, err, found)
	}
}

func TestRegister_RegisteredCard_ReturnsErrAlreadyRegistered(t *testing.T) {
	s := openStore(t)
	register(t, s)

	_, err := s.Register(card, "Other", 1, 1)

	if !errors.Is(err, store.ErrAlreadyRegistered) {
		t.Fatalf("error = %v, want ErrAlreadyRegistered", err)
	}
}

func TestRecordResult_AddsToTheTotals(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.RecordResult(player.ID, store.Result{Zet: 568, Exp: 222, Score: 195})

	err := s.RecordResult(player.ID, store.Result{Zet: 10, Exp: 5, Score: 1})

	got, _ := s.Player(player.ID)
	if err != nil || got.Zet != 578 || got.Exp != 227 || got.Score != 196 {
		t.Fatalf("player = %+v, err = %v", got, err)
	}
}

func TestRegister_NewPlayer_StartsInTheLowestClasses(t *testing.T) {
	s := openStore(t)

	player := register(t, s)

	if player.CoopClassID != 1 || player.VSClassID != 1 || player.CoopClassPoint != 0 || player.VSClassPoint != 0 {
		t.Fatalf("player = %+v", player)
	}
}

func TestRecordResult_NoClasses_KeepsTheStoredClasses(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.RecordResult(player.ID, store.Result{CoopClassID: 4, VSClassID: 2})

	s.RecordResult(player.ID, store.Result{Zet: 10})

	got, _ := s.Player(player.ID)
	if got.CoopClassID != 4 || got.VSClassID != 2 {
		t.Fatalf("player = %+v", got)
	}
}

func TestRegister_NewPlayer_HasTheDefaultSettings(t *testing.T) {
	s := openStore(t)

	player := register(t, s)

	if player.Settings != store.DefaultSettings {
		t.Fatalf("settings = %+v, want %+v", player.Settings, store.DefaultSettings)
	}
}

func TestUpdateSettings_ReplacesTheSettings(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	settings := store.Settings{MasterVolume: 2, BGMVolume: 7, MouseHorizontalReverse: 1, MouseVerticalReverse: 1, MouseSensitivity: 9, MouseAcceleration: 1, KeyMap: 2}

	err := s.UpdateSettings(card, settings)

	got, _ := s.Player(player.ID)
	if err != nil || got.Settings != settings {
		t.Fatalf("settings = %+v, err = %v", got.Settings, err)
	}
}

func TestRename_ChangesTheName(t *testing.T) {
	s := openStore(t)
	player := register(t, s)

	err := s.Rename(card, "Survivor")

	got, _ := s.Player(player.ID)
	if err != nil || got.Name != "Survivor" {
		t.Fatalf("name = %q, err = %v", got.Name, err)
	}
}

func TestUpdateSettings_UnknownCard_ReturnsAnError(t *testing.T) {
	s := openStore(t)

	err := s.UpdateSettings(card, store.Settings{})

	if err == nil {
		t.Fatal("UpdateSettings() error = nil")
	}
}

func TestRecordResult_UnknownPlayer_ReturnsAnError(t *testing.T) {
	s := openStore(t)

	err := s.RecordResult(99, store.Result{Zet: 1})

	if err == nil {
		t.Fatal("RecordResult() error = nil")
	}
}

func TestRecordLogin_CountsEachLogin(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.RecordLogin(player.ID, day1)

	err := s.RecordLogin(player.ID, day1)

	got, _ := s.Player(player.ID)
	if err != nil || got.Logins != 2 {
		t.Fatalf("logins = %d, err = %v", got.Logins, err)
	}
}

func TestOpen_DatabaseFromAnEarlierSchema_IsMigrated(t *testing.T) {
	path := t.TempDir() + "/old.db"
	old, _ := sql.Open("sqlite", path)
	old.Exec(`CREATE TABLE players (id INTEGER PRIMARY KEY, card_id TEXT NOT NULL UNIQUE, name TEXT NOT NULL, zet INTEGER NOT NULL DEFAULT 0, exp INTEGER NOT NULL DEFAULT 0, score INTEGER NOT NULL DEFAULT 0)`)
	old.Exec(`INSERT INTO players (card_id, name, zet) VALUES ('7020392000000100', 'Old', 7)`)
	old.Close()

	s, err := store.Open(path)

	player, _ := s.PlayerByCard("7020392000000100")
	if err != nil || player.Name != "Old" || player.Zet != 7 || player.Logins != 0 {
		t.Fatalf("player = %+v, err = %v", player, err)
	}
	s.Close()
}

func TestOpen_DatabaseWithCardsOnPlayers_MovesThemToTheirOwnTable(t *testing.T) {
	path := t.TempDir() + "/cards.db"
	s, _ := store.Open(path)
	player, _ := s.Register(card, "Mika", 3, 1)
	s.RecordResult(player.ID, store.Result{StageID: 1, ChapterID: 1, Cleared: true})
	s.Close()
	old, _ := sql.Open("sqlite", path)
	old.Exec(`DROP TABLE login_days; DROP TABLE queued_grants; DROP TABLE grant_deliveries; DROP TABLE owned_items; DROP TABLE equipment; DROP TABLE results; DROP TABLE cards; ALTER TABLE players ADD COLUMN card_id TEXT; UPDATE players SET card_id = '` + card + `'; PRAGMA user_version = 17`)
	old.Close()

	s, err := store.Open(path)

	found, _ := s.PlayerByCard(card)
	clears, _ := s.Clears(player.ID)
	if err != nil || found.ID != player.ID || found.Name != "Mika" || len(clears) != 1 {
		t.Fatalf("player = %+v, clears = %v, err = %v", found, clears, err)
	}
	s.Close()
}

func TestRecordResult_StoresTheResult(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	played := time.Date(2026, 11, 17, 21, 30, 15, 0, time.UTC)
	result := store.Result{Played: played, GameMode: 7, StageID: 1, ChapterID: 1, Score: 900, VSClassPoint: -10, VSPlays: 1, VSWins: 1}

	err := s.RecordResult(player.ID, result)

	results, _ := s.Results(player.ID)
	if err != nil || len(results) != 1 || !results[0].Played.Equal(played) || results[0].GameMode != 7 || results[0].Score != 900 || results[0].VSClassPoint != -10 {
		t.Fatalf("Results() = %+v, err = %v", results, err)
	}
}

func TestRecordResult_ClearedChapter_IsListedOnce(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.RecordResult(player.ID, store.Result{StageID: 1, ChapterID: 2, Cleared: true})
	s.RecordResult(player.ID, store.Result{StageID: 1, ChapterID: 1, Cleared: true})
	s.RecordResult(player.ID, store.Result{StageID: 1, ChapterID: 2, Cleared: true})
	s.RecordResult(player.ID, store.Result{StageID: 2, ChapterID: 1, Cleared: false})

	clears, err := s.Clears(player.ID)

	want := []store.Clear{{StageID: 1, ChapterID: 1}, {StageID: 1, ChapterID: 2}}
	if err != nil || !slices.Equal(clears, want) {
		t.Fatalf("Clears() = %v, %v; want %v", clears, err, want)
	}
}

var monthStart = time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC)

func TestTopScorersSince_TotalsEachPlayersScoresInThePeriod(t *testing.T) {
	s := openStore(t)
	low, _ := s.Register("7020392000000001", "Low", 1, 1)
	high, _ := s.Register("7020392000000002", "High", 1, 1)
	s.Register("7020392000000003", "Unplayed", 1, 1)
	s.RecordResult(low.ID, store.Result{Played: monthStart, Score: 50})
	s.RecordResult(high.ID, store.Result{Played: monthStart.Add(time.Hour), Score: 40})
	s.RecordResult(high.ID, store.Result{Played: monthStart.Add(2 * time.Hour), Score: 30})

	got, err := s.TopScorersSince(monthStart, 0, 10)

	if err != nil || len(got) != 2 || got[0].Player.ID != high.ID || got[0].Score != 70 || got[1].Player.ID != low.ID || got[1].Score != 50 {
		t.Fatalf("TopScorersSince() = %+v, %v", got, err)
	}
}

func TestTopScorersSince_LeavesOutEarlierAndVersusResults(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.RecordResult(player.ID, store.Result{Played: monthStart.Add(-time.Second), Score: 1000})
	s.RecordResult(player.ID, store.Result{Played: monthStart, Score: 500, VSPlays: 1})
	s.RecordResult(player.ID, store.Result{Played: monthStart, Score: 20})

	got, _ := s.TopScorersSince(monthStart, 0, 10)

	if len(got) != 1 || got[0].Score != 20 {
		t.Fatalf("TopScorersSince() = %+v, want a score of 20", got)
	}
}

func TestTopScorersSince_StopsAtTheLimit(t *testing.T) {
	s := openStore(t)
	for i := range 3 {
		player, _ := s.Register("702039200000000"+strconv.Itoa(i+1), "Player", 1, 1)
		s.RecordResult(player.ID, store.Result{Played: monthStart, Score: 10})
	}

	got, _ := s.TopScorersSince(monthStart, 0, 2)

	if len(got) != 2 {
		t.Fatalf("TopScorersSince() returned %d players, want 2", len(got))
	}
}

func TestTopVersusPlayers_OrdersVersusPlayersByVersusClassPoints(t *testing.T) {
	s := openStore(t)
	low, _ := s.Register("7020392000000001", "Low", 1, 1)
	high, _ := s.Register("7020392000000002", "High", 1, 1)
	coop, _ := s.Register("7020392000000003", "Coop", 1, 1)
	s.RecordResult(low.ID, store.Result{VSPlays: 1, VSClassPoint: 5})
	s.RecordResult(high.ID, store.Result{VSPlays: 2, VSClassPoint: 40})
	s.RecordResult(coop.ID, store.Result{Score: 100})

	got, err := s.TopVersusPlayers(0, 10)

	if err != nil || len(got) != 2 || got[0].ID != high.ID || got[1].ID != low.ID {
		t.Fatalf("TopVersusPlayers() = %+v, %v", got, err)
	}
}

func TestTopScorersSince_WithAHomeStore_KeepsThatStoresPlayers(t *testing.T) {
	s := openStore(t)
	here, _ := s.Register("7020392000000001", "Here", 1, 2)
	elsewhere, _ := s.Register("7020392000000002", "Elsewhere", 1, 1)
	s.RecordResult(here.ID, store.Result{Played: monthStart, Score: 10})
	s.RecordResult(elsewhere.ID, store.Result{Played: monthStart, Score: 90})

	got, err := s.TopScorersSince(monthStart, 2, 10)

	if err != nil || len(got) != 1 || got[0].Player.ID != here.ID {
		t.Fatalf("TopScorersSince() = %+v, %v; want only %d", got, err, here.ID)
	}
}

func TestTopVersusPlayers_WithAHomeStore_KeepsThatStoresPlayers(t *testing.T) {
	s := openStore(t)
	here, _ := s.Register("7020392000000001", "Here", 1, 2)
	elsewhere, _ := s.Register("7020392000000002", "Elsewhere", 1, 1)
	s.RecordResult(here.ID, store.Result{VSPlays: 1, VSClassPoint: 5})
	s.RecordResult(elsewhere.ID, store.Result{VSPlays: 1, VSClassPoint: 40})

	got, err := s.TopVersusPlayers(2, 10)

	if err != nil || len(got) != 1 || got[0].ID != here.ID {
		t.Fatalf("TopVersusPlayers() = %+v, %v; want only %d", got, err, here.ID)
	}
}
