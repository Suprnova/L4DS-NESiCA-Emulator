package store_test

import (
	"database/sql"
	"errors"
	"slices"
	"testing"

	_ "modernc.org/sqlite"

	"l4ds-nesica-emulator/internal/store"
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

const card = "7020392010281502"

func register(t *testing.T, s *store.Store) store.Player {
	t.Helper()
	player, err := s.Register(card, "Nova", 3)
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

func TestRegister_NewCard_CreatesThePlayer(t *testing.T) {
	s := openStore(t)

	player, err := s.Register(card, "Nova", 3)

	found, _ := s.PlayerByCard(card)
	if err != nil || player.ID == 0 || found.ID != player.ID || found.Name != "Nova" || found.CharaID != 3 {
		t.Fatalf("Register() = %+v, %v; found %+v", player, err, found)
	}
}

func TestRegister_RegisteredCard_ReturnsErrAlreadyRegistered(t *testing.T) {
	s := openStore(t)
	register(t, s)

	_, err := s.Register(card, "Other", 1)

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
	s.RecordLogin(player.ID)

	err := s.RecordLogin(player.ID)

	got, _ := s.Player(player.ID)
	if err != nil || got.Logins != 2 {
		t.Fatalf("logins = %d, err = %v", got.Logins, err)
	}
}

func TestOpen_DatabaseFromAnEarlierSchema_IsMigrated(t *testing.T) {
	path := t.TempDir() + "/old.db"
	old, _ := sql.Open("sqlite", path)
	old.Exec(`CREATE TABLE players (id INTEGER PRIMARY KEY, card_id TEXT NOT NULL UNIQUE, name TEXT NOT NULL, zet INTEGER NOT NULL DEFAULT 0, exp INTEGER NOT NULL DEFAULT 0, score INTEGER NOT NULL DEFAULT 0)`)
	old.Exec(`INSERT INTO players (card_id, name, zet) VALUES ('7020392010281502', 'Old', 7)`)
	old.Close()

	s, err := store.Open(path)

	player, _ := s.PlayerByCard("7020392010281502")
	if err != nil || player.Name != "Old" || player.Zet != 7 || player.Logins != 0 {
		t.Fatalf("player = %+v, err = %v", player, err)
	}
	s.Close()
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
