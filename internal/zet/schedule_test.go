package zet_test

import (
	"slices"
	"strings"
	"testing"
	"time"

	"l4ds-nesica-emulator/internal/store"
	"l4ds-nesica-emulator/internal/zet"
)

func scheduleServer(t *testing.T, schedule zet.Schedule) *zet.Server {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return zet.NewServer(s, zet.Catalog{Schedule: schedule})
}

func TestHandler_Versus_ListsEachUnit(t *testing.T) {
	start, end := time.Unix(1700000000, 0), time.Unix(1800000000, 0)
	server := scheduleServer(t, zet.Schedule{Versus: []zet.VersusUnit{{
		ID: 3, StageID: 2, ChapterID: 4, Difficulty: 2, Cost: 300, TakeWeapon: true, GameMode: "versus", Start: start, End: end,
	}}})

	got := strings.Split(postTo(server, "/versus", "t"), "\n")

	want := []string{"1", "1", "0", "1", "3", "2", "4", "2", "300", "1", "versus", "1700000000", "1800000000", ""}
	if !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

func TestHandler_Event_ListsEachEventWithZeroTimesAsZero(t *testing.T) {
	server := scheduleServer(t, zet.Schedule{Events: []zet.Event{{
		ID: 7, Type: zet.EventStore, StageID: 1, ChapterID: 2, Difficulty: 3, SpecialMode: 1, Cost: 0,
		TakeDisposable: true, GameMode: "mutation2", Name: "Event", Detail: "a|b",
		MinLevel: 1, MaxLevel: 99, MinClassID: 1, MaxClassID: 37, Close: time.Unix(1800000000, 0),
	}}})

	got := strings.Split(postTo(server, "/event", "tenpo_id=1"), "\n")

	want := []string{"1", "1", "0", "1",
		"7", "2", "1", "2", "3", "1", "0", "0", "1", "0", "mutation2", "Event", "a|b",
		"1", "99", "1", "37", "0", "1800000000", "0", "0", ""}
	if !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

func TestHandler_Versus_EmptySchedule_ListsNothing(t *testing.T) {
	got := post(t, "/versus", "t")

	if got != "1\n1\n0\n0\n" {
		t.Fatalf("body = %q", got)
	}
}
