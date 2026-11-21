package zet_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

var jst = time.FixedZone("JST", 9*60*60)

func on(month time.Month, day, hour int) time.Time {
	return time.Date(2027, month, day, hour, 0, 0, 0, jst)
}

func scheduleServer(t *testing.T, now time.Time, schedule zet.Schedule) *zet.Server {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	server := zet.NewServer(s, zet.Catalog{Schedule: schedule})
	server.SetClock(func() time.Time { return now })
	return server
}

// fortnightly is a versus unit's span: two weeks from Thursday 2027-04-01 08:00, every four weeks.
var fortnightly = calendar.Span{Start: on(4, 1, 8), End: on(4, 15, 8), Every: calendar.Every{Days: 28}}

func versusUnit(when calendar.Span) zet.VersusUnit {
	return zet.VersusUnit{ID: 3, StageID: 2, ChapterID: 4, Difficulty: 2, Cost: 300, TakeWeapon: true, GameMode: "versus", When: when}
}

func TestHandler_Versus_ListsTheRunOpenNow(t *testing.T) {
	server := scheduleServer(t, on(5, 3, 12), zet.Schedule{Versus: []zet.VersusUnit{versusUnit(fortnightly)}})

	got := strings.Split(postTo(server, "/versus", "t"), "\n")

	want := []string{"1", "1", "0", "1", "3", "2", "4", "2", "300", "1", "versus", fmt.Sprint(on(4, 29, 8).Unix()), fmt.Sprint(on(5, 13, 8).Unix()), ""}
	if !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

func TestHandler_Versus_RunStartingBeforeTheDayTurn_IsListed(t *testing.T) {
	server := scheduleServer(t, on(4, 29, 7), zet.Schedule{Versus: []zet.VersusUnit{versusUnit(fortnightly)}})

	got := strings.Split(postTo(server, "/versus", "t"), "\n")

	if got[3] != "1" || got[11] != fmt.Sprint(on(4, 29, 8).Unix()) {
		t.Fatalf("lines = %q", got)
	}
}

func TestHandler_Versus_RunStartingAfterTheDayTurn_IsNotListedYet(t *testing.T) {
	server := scheduleServer(t, on(4, 20, 12), zet.Schedule{Versus: []zet.VersusUnit{versusUnit(fortnightly)}})

	if got := postTo(server, "/versus", "t"); got != "1\n1\n0\n0\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Event_AlwaysOpen_IsListedWithZeroTimes(t *testing.T) {
	server := scheduleServer(t, on(4, 1, 12), zet.Schedule{Events: []zet.Event{{
		ID: 7, Type: zet.EventStore, StageID: 1, ChapterID: 2, Difficulty: 3, SpecialMode: 1, Cost: 0,
		TakeDisposable: true, GameMode: "mutation2", Name: "Event", Detail: "a|b",
		MinLevel: 1, MaxLevel: 99, MinClassID: 1, MaxClassID: 37,
	}}})

	got := strings.Split(postTo(server, "/event", "tenpo_id=1"), "\n")

	want := []string{"1", "1", "0", "1",
		"7", "2", "1", "2", "3", "1", "0", "0", "1", "0", "mutation2", "Event", "a|b",
		"1", "99", "1", "37", "0", "0", "0", "0", ""}
	if !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

// stageKindsOf returns, for each stage kind in a stage list reply, the stage ids it lists.
func stageKindsOf(body string) map[string][]string {
	lines := strings.Split(body, "\n")
	kinds := map[string][]string{}
	for i := 4; i+4 < len(lines); {
		chapters := 0
		fmt.Sscan(lines[i+4], &chapters)
		kinds[lines[i+1]] = append(kinds[lines[i+1]], lines[i])
		i += 5 + 2*chapters
	}
	return kinds
}

func coopServer(t *testing.T, now time.Time) *zet.Server {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	stages := []game.Stage{{ID: 1, Name: "A"}, {ID: 2, Name: "B", Chapters: []game.Chapter{{ID: 1, Name: "B1"}}}, {ID: 3, Name: "C"}}
	weekly := calendar.Span{Start: on(4, 1, 8), End: on(4, 8, 8), Every: calendar.Every{Days: 14}}
	server := zet.NewServer(s, zet.Catalog{Stages: stages, Schedule: zet.Schedule{CoopStages: []zet.StageList{{When: weekly, Stages: []int{3, 2, 9}}}}})
	server.SetClock(func() time.Time { return now })
	return server
}

func TestHandler_Stages_OpenCoopList_LimitsOnlyNationalCoop(t *testing.T) {
	got := stageKindsOf(postTo(coopServer(t, on(4, 2, 12)), "/stages", "t"))

	every := []string{"1", "2", "3"}
	if !slices.Equal(got["1"], []string{"3", "2"}) || !slices.Equal(got["2"], every) || !slices.Equal(got["3"], every) || !slices.Equal(got["4"], every) {
		t.Fatalf("stages by kind = %v", got)
	}
}

func TestHandler_Stages_NoCoopListOpen_OffersEveryStage(t *testing.T) {
	got := stageKindsOf(postTo(coopServer(t, on(4, 9, 12)), "/stages", "t"))

	if !slices.Equal(got["1"], []string{"1", "2", "3"}) {
		t.Fatalf("national co-op stages = %v", got["1"])
	}
}

func TestHandler_Versus_EmptySchedule_ListsNothing(t *testing.T) {
	got := post(t, "/versus", "t")

	if got != "1\n1\n0\n0\n" {
		t.Fatalf("body = %q", got)
	}
}
