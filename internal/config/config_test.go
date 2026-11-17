package config_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"l4ds-nesica-emulator/internal/config"
	"l4ds-nesica-emulator/internal/zet"
)

func writeConfig(t *testing.T, text string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "l4ds.yaml")
	if err := os.WriteFile(path, []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	return path
}

func TestLoad_EmptyFile_KeepsTheDefaults(t *testing.T) {
	got, err := config.Load(writeConfig(t, ""))

	want := config.Default()
	if err != nil || got.Store != want.Store || got.ServerKey != want.ServerKey || len(got.Versus) != 0 || len(got.Events) != 0 {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_OnlyComments_KeepsTheDefaults(t *testing.T) {
	got, err := config.Load(writeConfig(t, "# nothing set yet\n"))

	if err != nil || got.Store != config.Default().Store {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_ReadsTheStoreAndServerKey(t *testing.T) {
	path := writeConfig(t, "store:\n  name: Arcade\n  prefecture: Osaka\nserver_key: example.test\n")

	got, err := config.Load(path)

	if err != nil || got.Store.Name != "Arcade" || got.Store.Prefecture != "Osaka" || got.ServerKey != "example.test" {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_UnknownKey_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "store:\n  nmae: Arcade\n"))

	if err == nil {
		t.Fatal("Load() err = nil")
	}
}

func TestLoad_EmptyPrefecture_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "store:\n  prefecture: \"\"\n"))

	if err == nil || !strings.Contains(err.Error(), "prefecture") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestLoad_UnknownDifficulty_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "versus:\n  - id: 3\n    difficulty: hard\n"))

	if err == nil || !strings.Contains(err.Error(), "versus unit 3") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestSchedule_ConvertsAVersusUnit(t *testing.T) {
	path := writeConfig(t, `versus:
  - id: 1
    stage: 2
    chapter: 3
    difficulty: advanced
    cost: 100
    take_weapon: true
    game_mode: versus
    start: 2026-11-01T00:00:00Z
    end: 2027-11-01T00:00:00Z
`)
	loaded, _ := config.Load(path)

	got, err := loaded.Schedule()

	want := zet.VersusUnit{ID: 1, StageID: 2, ChapterID: 3, Difficulty: 3, Cost: 100, TakeWeapon: true, GameMode: "versus",
		Start: time.Date(2026, 11, 1, 0, 0, 0, 0, time.UTC), End: time.Date(2027, 11, 1, 0, 0, 0, 0, time.UTC)}
	if err != nil || len(got.Versus) != 1 || !got.Versus[0].Start.Equal(want.Start) || !got.Versus[0].End.Equal(want.End) {
		t.Fatalf("Schedule() = %+v, %v", got, err)
	}
	got.Versus[0].Start, got.Versus[0].End = want.Start, want.End
	if got.Versus[0] != want {
		t.Fatalf("Schedule().Versus[0] = %+v, want %+v", got.Versus[0], want)
	}
}

func TestSchedule_ConvertsAnEvent(t *testing.T) {
	path := writeConfig(t, `events:
  - id: 7
    type: store
    stage: 1
    chapter: 2
    difficulty: expert
    god: true
    take_disposable: true
    multi_match: true
    game_mode: mutation2
    name: Headshots
    detail: Only headshots count.
    min_level: 5
    max_class: 3
`)
	loaded, _ := config.Load(path)

	got, err := loaded.Schedule()

	want := zet.Event{ID: 7, Type: zet.EventStore, StageID: 1, ChapterID: 2, Difficulty: 4, SpecialMode: 1, TakeDisposable: true,
		MultiMatch: true, GameMode: "mutation2", Name: "Headshots", Detail: "Only headshots count.", MinLevel: 5, MaxClassID: 3}
	if err != nil || len(got.Events) != 1 || got.Events[0] != want {
		t.Fatalf("Schedule() = %+v, %v", got, err)
	}
}

func TestSchedule_DailyHours_AreLocalTimesOfDayAfter1970(t *testing.T) {
	path := writeConfig(t, "events:\n  - id: 1\n    type: national\n    difficulty: normal\n    daily_open: \"10:00\"\n    daily_close: \"22:30\"\n")
	loaded, _ := config.Load(path)

	got, _ := loaded.Schedule()

	open, closing := got.Events[0].DailyOpen.Local(), got.Events[0].DailyClose.Local()
	if open.Hour() != 10 || open.Minute() != 0 || closing.Hour() != 22 || closing.Minute() != 30 || open.Unix() <= 0 {
		t.Fatalf("daily hours %v to %v", open, closing)
	}
}

func TestLoad_BadDailyTime_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "events:\n  - id: 2\n    type: national\n    difficulty: normal\n    daily_open: 25h\n"))

	if err == nil || !strings.Contains(err.Error(), "event 2") {
		t.Fatalf("Load() err = %v", err)
	}
}
