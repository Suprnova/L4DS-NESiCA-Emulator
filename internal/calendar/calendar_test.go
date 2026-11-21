package calendar_test

import (
	"slices"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
)

var jst = time.FixedZone("JST", 9*60*60)

func at(month time.Month, day, hour, minute int) time.Time {
	return time.Date(2027, month, day, hour, minute, 0, 0, jst)
}

// weekly is a national co-op list: Thursday 08:00 for a week, from 2027-04-01.
var weekly = calendar.Span{Start: at(4, 1, 8, 0), End: at(4, 8, 8, 0), Every: calendar.Every{Days: 7}}

func TestNextDayTurn_IsTheNext0800(t *testing.T) {
	for now, want := range map[time.Time]time.Time{
		at(4, 1, 7, 59): at(4, 1, 8, 0),
		at(4, 1, 8, 0):  at(4, 2, 8, 0),
		at(4, 1, 23, 0): at(4, 2, 8, 0),
	} {
		if got := calendar.NextDayTurn(now); !got.Equal(want) {
			t.Errorf("NextDayTurn(%s) = %s, want %s", now, got, want)
		}
	}
}

func TestSpan_At_FindsTheRunOfAWeeklySpan(t *testing.T) {
	run, ok := weekly.At(at(5, 20, 12, 0))

	if !ok || run.N != 7 || !run.Start.Equal(at(5, 20, 8, 0)) || !run.End.Equal(at(5, 27, 8, 0)) {
		t.Fatalf("At() = %+v, %v", run, ok)
	}
}

func TestSpan_At_BeforeTheFirstRun_FindsNone(t *testing.T) {
	if run, ok := weekly.At(at(4, 1, 7, 59)); ok {
		t.Fatalf("At() = %+v", run)
	}
}

func TestSpan_Runs_ListsTheRunsOverlappingTheTime(t *testing.T) {
	block := calendar.Span{Start: at(4, 1, 8, 0), End: at(4, 22, 0, 0), Every: calendar.Every{Days: 28}}

	got := block.Runs(at(4, 21, 12, 0), at(5, 1, 8, 0))

	want := []calendar.Run{{N: 0, Start: at(4, 1, 8, 0), End: at(4, 22, 0, 0)}, {N: 1, Start: at(4, 29, 8, 0), End: at(5, 20, 0, 0)}}
	if !slices.EqualFunc(got, want, sameRun) {
		t.Fatalf("Runs() = %+v, want %+v", got, want)
	}
}

func TestSpan_Runs_StopsAtUntil(t *testing.T) {
	span := weekly
	span.Until = at(4, 8, 8, 0)

	got := span.Runs(at(4, 1, 0, 0), at(6, 1, 0, 0))

	if len(got) != 2 || got[1].N != 1 {
		t.Fatalf("Runs() = %+v, want two runs", got)
	}
}

func TestSpan_Runs_YearlySpanComesBackOnItsDate(t *testing.T) {
	goldenWeek := calendar.Span{Start: at(5, 1, 8, 0), End: at(5, 9, 0, 0), Every: calendar.Every{Years: 1}}

	run, ok := goldenWeek.At(time.Date(2031, 5, 3, 12, 0, 0, 0, jst))

	if !ok || run.N != 4 || !run.Start.Equal(time.Date(2031, 5, 1, 8, 0, 0, 0, jst)) {
		t.Fatalf("At() = %+v, %v", run, ok)
	}
}

func TestSpan_ZeroSpan_IsAlwaysOpen(t *testing.T) {
	run, ok := calendar.Span{}.At(at(4, 1, 0, 0))

	if !ok || run != (calendar.Run{}) {
		t.Fatalf("At() = %+v, %v", run, ok)
	}
}

func TestSpan_Check_RefusesBadSpans(t *testing.T) {
	spans := map[string]calendar.Span{
		"ends at its start": {Start: at(4, 1, 8, 0), End: at(4, 1, 8, 0)},
		"runs overlap":      {Start: at(4, 1, 8, 0), End: at(4, 9, 8, 0), Every: calendar.Every{Days: 7}},
		"until before":      {Start: at(4, 1, 8, 0), End: at(4, 2, 8, 0), Every: calendar.Every{Days: 7}, Until: at(3, 1, 0, 0)},
		"days and years":    {Start: at(4, 1, 8, 0), End: at(4, 2, 8, 0), Every: calendar.Every{Days: 7, Years: 1}},
	}

	for name, span := range spans {
		if span.Check() == nil {
			t.Errorf("%s: Check() = nil", name)
		}
	}
	if err := weekly.Check(); err != nil {
		t.Errorf("weekly: Check() = %v", err)
	}
}

func sameRun(a, b calendar.Run) bool {
	return a.N == b.N && a.Start.Equal(b.Start) && a.End.Equal(b.End)
}
