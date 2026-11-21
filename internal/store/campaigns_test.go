package store_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

func TestDeliverStep_GivesEachStepOnce(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	step := store.Step{Kind: "ladder", ID: 1, Run: 0, Step: 2}

	first, errFirst := s.DeliverStep(player.ID, step, day1, store.Gift{Name: "Day 3", Zet: 100})
	again, errAgain := s.DeliverStep(player.ID, step, day1, store.Gift{Name: "Day 3", Zet: 100})

	updated, _ := s.Player(player.ID)
	if !first || again || errFirst != nil || errAgain != nil || updated.Zet != 100 {
		t.Fatalf("DeliverStep() = %v, %v then %v, %v; zet %d", first, errFirst, again, errAgain, updated.Zet)
	}
}

func TestDeliverStep_KindsHaveTheirOwnIDs(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.DeliverStep(player.ID, store.Step{Kind: "daily", ID: 1}, day1, store.Gift{Name: "A", Zet: 100})

	ok, err := s.DeliverStep(player.ID, store.Step{Kind: "ladder", ID: 1}, day1, store.Gift{Name: "B", Zet: 100})

	if !ok || err != nil {
		t.Fatalf("DeliverStep() = %v, %v", ok, err)
	}
}

func TestCountResults_CountsWhatTheFilterPicks(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	at := func(hours int) time.Time { return day1.Add(time.Duration(hours) * time.Hour) }
	s.RecordResult(player.ID, store.Result{Played: at(1), GameMode: 7})
	s.RecordResult(player.ID, store.Result{Played: at(2), GameMode: 1, StageID: 12, ChapterID: 1, Cleared: true})
	s.RecordResult(player.ID, store.Result{Played: at(3), GameMode: 1, StageID: 12, ChapterID: 2})
	s.RecordResult(player.ID, store.Result{Played: at(30), GameMode: 7})

	versus, _ := s.CountResults(player.ID, day1, day2, store.ResultFilter{Modes: []int64{7, 8, 9, 10}})
	clears, _ := s.CountResults(player.ID, day1, day2, store.ResultFilter{Modes: []int64{1}, StageID: 12, Cleared: true})
	all, err := s.CountResults(player.ID, day1, day2, store.ResultFilter{})

	if err != nil || versus != 1 || clears != 1 || all != 3 {
		t.Fatalf("CountResults() = %d versus, %d clears, %d all, %v", versus, clears, all, err)
	}
}

func TestStartTimes_ListsEachStartOnceOfResultsBeforeTheEnd(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	at := func(hours int) time.Time { return day1.Add(time.Duration(hours) * time.Hour) }
	s.RecordResult(player.ID, store.Result{Played: at(3), Started: at(2), GameMode: 2, EventID: 4})
	s.RecordResult(player.ID, store.Result{Played: at(1), GameMode: 2, EventID: 4})
	s.RecordResult(player.ID, store.Result{Played: at(1), GameMode: 2, EventID: 4})
	s.RecordResult(player.ID, store.Result{Played: at(3), GameMode: 2, EventID: 5})
	s.RecordResult(player.ID, store.Result{Played: at(30), Started: at(5), GameMode: 2, EventID: 4})

	times, err := s.StartTimes(player.ID, day2, store.ResultFilter{EventID: 4})

	if err != nil || len(times) != 2 || !times[0].Equal(at(1)) || !times[1].Equal(at(2)) {
		t.Fatalf("StartTimes() = %v, %v", times, err)
	}
}

func TestTotalScores_RanksPlayersByTheirTotalOfChaptersStartedInTheRange(t *testing.T) {
	s := openStore(t)
	first := register(t, s)
	second, _ := s.Register("7020392000000002", "Second", 0, 1)
	third, _ := s.Register("7020392000000003", "Third", 0, 1)
	event := store.ResultFilter{Modes: []int64{2}, EventID: 4}
	s.RecordResult(first.ID, store.Result{Played: day2.Add(time.Minute), Started: day2.Add(-time.Minute), GameMode: 2, EventID: 4, Score: 300})
	s.RecordResult(second.ID, store.Result{Played: day1, GameMode: 2, EventID: 4, Score: 200})
	s.RecordResult(second.ID, store.Result{Played: day1, GameMode: 2, EventID: 4, Score: 200})
	s.RecordResult(third.ID, store.Result{Played: day1, GameMode: 1, Score: 900})
	s.RecordResult(third.ID, store.Result{Played: day2, GameMode: 2, EventID: 4, Score: 900})

	totals, err := s.TotalScores(day1, day2, event)

	want := []store.Total{{second.ID, 400}, {first.ID, 300}}
	if err != nil || !slices.Equal(totals, want) {
		t.Fatalf("TotalScores() = %v, %v; want %v", totals, err, want)
	}
}
