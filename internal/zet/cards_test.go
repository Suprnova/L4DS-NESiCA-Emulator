package zet_test

import (
	"strings"
	"testing"

	"l4ds-nesica-emulator/internal/store"
	"l4ds-nesica-emulator/internal/zet"
)

func newServer(t *testing.T) (*zet.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return zet.NewServer(s), s
}

func TestServer_Select_ReturnsTheStoredPlayer(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	s.RecordResult(player.ID, store.Result{Zet: 568, Score: 195})

	body, err := server.Select("7020392000000001")

	lines := strings.Split(body, "\n")
	if err != nil || lines[0] != "1" || lines[2] != "7020392000000001" || lines[3] != "1" || lines[4] != "Survivor" || lines[5] != "568" || lines[7] != "195" {
		t.Fatalf("Select() = %q, %v", body, err)
	}
}

func TestServer_Select_ReportsExperienceAndLoginsAfterTheEmptyBlocks(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	s.RecordResult(player.ID, store.Result{Exp: 222})
	s.RecordLogin(player.ID)

	body, _ := server.Select("7020392000000001")

	lines := strings.Split(body, "\n")
	if lines[53] != "222" || lines[87] != "1" {
		t.Fatalf("experience line = %q, login_num line = %q", lines[53], lines[87])
	}
}

func TestServer_Select_UnregisteredCard_ReportsPlayerIdZero(t *testing.T) {
	server, _ := newServer(t)

	body, err := server.Select("7020392000000009")

	if err != nil || body != "1\n1\n7020392000000009\n0\n" {
		t.Fatalf("Select() = %q, %v", body, err)
	}
}
