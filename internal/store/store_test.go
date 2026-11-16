package store_test

import (
	"database/sql"
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

func TestPlayerByCard_UnknownCard_RegistersADefaultPlayer(t *testing.T) {
	s := openStore(t)

	player, err := s.PlayerByCard("7020392010281502")

	if err != nil || player.ID == 0 || player.Name != store.DefaultName || player.CardID != "7020392010281502" {
		t.Fatalf("PlayerByCard() = %+v, %v", player, err)
	}
}

func TestPlayerByCard_KnownCard_ReturnsTheSamePlayer(t *testing.T) {
	s := openStore(t)
	first, _ := s.PlayerByCard("7020392010281502")

	second, err := s.PlayerByCard("7020392010281502")

	if err != nil || second.ID != first.ID {
		t.Fatalf("PlayerByCard() = %+v, %v; want id %d", second, err, first.ID)
	}
}

func TestRecordResult_AddsToTheTotals(t *testing.T) {
	s := openStore(t)
	player, _ := s.PlayerByCard("7020392010281502")
	s.RecordResult(player.ID, store.Result{Zet: 568, Exp: 222, Score: 195})

	err := s.RecordResult(player.ID, store.Result{Zet: 10, Exp: 5, Score: 1})

	got, _ := s.Player(player.ID)
	if err != nil || got.Zet != 578 || got.Exp != 227 || got.Score != 196 {
		t.Fatalf("player = %+v, err = %v", got, err)
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
	player, _ := s.PlayerByCard("7020392010281502")
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
