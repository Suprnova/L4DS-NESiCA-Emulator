package zet_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

func rankingServer(t *testing.T) (*zet.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	server := zet.NewServer(s, zet.Catalog{StoreName: "L4DS Emulator"})
	server.SetClock(func() time.Time { return now })
	return server, s
}

// now is the rankings' current time, in the middle of a month.
var now = time.Date(2026, 11, 17, 12, 0, 0, 0, time.Local)

func TestServer_Ranking_NoPlayers_IsEmpty(t *testing.T) {
	server, _ := rankingServer(t)

	got, err := server.Ranking(zet.RankingNationalCoop)

	if err != nil || got != "1\n1\n0\n" {
		t.Fatalf("Ranking() = %q, %v", got, err)
	}
}

func TestServer_CoopRanking_ListsPlayersByScore(t *testing.T) {
	server, s := rankingServer(t)
	low, _ := s.Register("7020392000000001", "Low", 0)
	high, _ := s.Register("7020392000000002", "High", 0)
	s.RecordResult(low.ID, store.Result{Played: now, Score: 10, CoopClassID: 2})
	s.RecordResult(high.ID, store.Result{Played: now, Score: 90})

	got, _ := server.Ranking(zet.RankingStoreCoop)

	want := []string{"1", "1", "2",
		"1", "2", "90", "", "High", "1", "1", "", "L4DS Emulator", "", "0",
		"2", "1", "10", "", "Low", "1", "2", "", "L4DS Emulator", "", "0", ""}
	if lines := strings.Split(got, "\n"); !slices.Equal(lines, want) {
		t.Fatalf("Ranking() lines = %q, want %q", lines, want)
	}
}

func TestServer_Ranking_PlayerWithACharacter_HasEmptyAvatarSlots(t *testing.T) {
	server, s := rankingServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 3)
	s.RecordResult(player.ID, store.Result{Played: now, Score: 10})

	got, _ := server.Ranking(zet.RankingNationalCoop)

	lines := strings.Split(got, "\n")
	if len(lines) != 3+11+6+1 || lines[13] != "3" || !slices.Equal(lines[14:20], []string{"0", "0", "0", "0", "0", "0"}) {
		t.Fatalf("Ranking() lines = %q", lines)
	}
}

func TestServer_VersusRanking_ListsVersusPlayersByVersusClassPoints(t *testing.T) {
	server, s := rankingServer(t)
	versus, _ := s.Register("7020392000000001", "Versus", 0)
	coop, _ := s.Register("7020392000000002", "Coop", 0)
	s.RecordResult(versus.ID, store.Result{VSPlays: 1, VSClassPoint: 25, VSClassID: 3, Score: 5})
	s.RecordResult(coop.ID, store.Result{Score: 90})

	got, _ := server.Ranking(zet.RankingNationalVersus)

	lines := strings.Split(got, "\n")
	if lines[2] != "1" || lines[4] != "1" || lines[5] != "25" || lines[7] != "Versus" || lines[9] != "3" {
		t.Fatalf("Ranking() lines = %q", lines)
	}
}

func TestServer_CoopRanking_TotalsThisMonthsScores(t *testing.T) {
	server, s := rankingServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	lastMonth := time.Date(2026, 10, 31, 23, 59, 59, 0, time.Local)
	s.RecordResult(player.ID, store.Result{Played: lastMonth, Score: 1000})
	s.RecordResult(player.ID, store.Result{Played: now.AddDate(0, 0, -16), Score: 30})
	s.RecordResult(player.ID, store.Result{Played: now, Score: 40})

	got, _ := server.Ranking(zet.RankingNationalCoop)

	if lines := strings.Split(got, "\n"); lines[2] != "1" || lines[5] != "70" {
		t.Fatalf("Ranking() lines = %q, want one entry scoring 70", lines)
	}
}
