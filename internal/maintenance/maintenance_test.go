package maintenance_test

import (
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/maintenance"
)

var jst = time.FixedZone("JST", 9*60*60)

func at(day, hour, minute int) time.Time { return time.Date(2026, 11, day, hour, minute, 0, 0, jst) }

// official is the official service's daily window, 06:00 to 08:00, warned 30 minutes ahead.
var official = maintenance.Schedule{Daily: &maintenance.Daily{Start: 6 * time.Hour, End: 8 * time.Hour}, Warn: 30 * time.Minute}

func TestAt_DuringTheDailyWindow_IsActive(t *testing.T) {
	got := official.At(at(20, 7, 0))

	if got != (maintenance.Status{Phase: maintenance.Active}) {
		t.Fatalf("At() = %+v, want active", got)
	}
}

func TestAt_WithinTheWarning_IsComingWithTheMinutesLeftRoundedUp(t *testing.T) {
	got := official.At(at(20, 5, 40).Add(30 * time.Second))

	if got != (maintenance.Status{Phase: maintenance.Coming, Minutes: 20}) {
		t.Fatalf("At() = %+v, want coming in 20 minutes", got)
	}
}

func TestAt_BeforeTheWarning_IsNone(t *testing.T) {
	got := official.At(at(20, 5, 29))

	if got != (maintenance.Status{}) {
		t.Fatalf("At() = %+v, want none", got)
	}
}

func TestAt_AtTheWindowsEnd_IsNone(t *testing.T) {
	got := official.At(at(20, 8, 0))

	if got != (maintenance.Status{}) {
		t.Fatalf("At() = %+v, want none", got)
	}
}

func TestAt_NoSchedule_IsNone(t *testing.T) {
	got := maintenance.Schedule{}.At(at(20, 7, 0))

	if got != (maintenance.Status{}) {
		t.Fatalf("At() = %+v, want none", got)
	}
}

func TestNext_AfterTodaysWindow_IsTomorrows(t *testing.T) {
	got, ok := official.Next(at(20, 9, 0))

	if !ok || !got.Start.Equal(at(21, 6, 0)) || !got.End.Equal(at(21, 8, 0)) {
		t.Fatalf("Next() = %+v, %v; want 21st 06:00 to 08:00", got, ok)
	}
}

func TestNext_DatedWindowExtendingTheDailyOne_MergesThem(t *testing.T) {
	schedule := official
	schedule.Windows = []maintenance.Window{{Start: at(20, 6, 0), End: at(20, 9, 15)}}

	got, ok := schedule.Next(at(20, 5, 0))

	if !ok || !got.Start.Equal(at(20, 6, 0)) || !got.End.Equal(at(20, 9, 15)) {
		t.Fatalf("Next() = %+v, %v; want 06:00 to 09:15", got, ok)
	}
}

func TestNext_DatedWindowEndingAtTheDailyStart_MergesThem(t *testing.T) {
	schedule := official
	schedule.Windows = []maintenance.Window{{Start: at(20, 0, 0), End: at(20, 6, 0)}}

	got, ok := schedule.Next(at(20, 1, 0))

	if !ok || !got.Start.Equal(at(20, 0, 0)) || !got.End.Equal(at(20, 8, 0)) {
		t.Fatalf("Next() = %+v, %v; want 00:00 to 08:00", got, ok)
	}
}

func TestNext_DailyWindowPastMidnight_EndsTheNextDay(t *testing.T) {
	schedule := maintenance.Schedule{Daily: &maintenance.Daily{Start: 23 * time.Hour, End: time.Hour}}

	got, ok := schedule.Next(at(20, 0, 30))

	if !ok || !got.Start.Equal(at(19, 23, 0)) || !got.End.Equal(at(20, 1, 0)) {
		t.Fatalf("Next() = %+v, %v; want 19th 23:00 to 20th 01:00", got, ok)
	}
}

func TestNext_OnlyPastDatedWindows_IsNone(t *testing.T) {
	schedule := maintenance.Schedule{Windows: []maintenance.Window{{Start: at(19, 6, 0), End: at(19, 8, 0)}}}

	_, ok := schedule.Next(at(20, 7, 0))

	if ok {
		t.Fatal("Next() found a window, want none")
	}
}
