package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

// loadCalendar loads a configuration whose calendar file, beside it, holds text.
func loadCalendar(t *testing.T, text string) (config.Server, error) {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "calendar.yaml"), []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "l4ds.yaml"), []byte("server:\n  calendar: calendar.yaml\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	loaded, err := config.Load(filepath.Join(dir, "l4ds.yaml"))
	return loaded.Server, err
}

func TestLoad_ExampleFiles_Load(t *testing.T) {
	got, err := config.Load(filepath.Join("..", "..", "l4ds.example.yaml"))

	if err != nil || len(got.Server.Schedule.Versus) != 1 || len(got.Server.Schedule.Events) != 2 || len(got.Server.Stores) != 1 {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_MissingCalendarFile_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "server:\n  calendar: none.yaml\n"))

	if err == nil {
		t.Fatal("Load() error = nil")
	}
}

func TestLoad_UnknownCalendarKey_IsAnError(t *testing.T) {
	if _, err := loadCalendar(t, "verus: []\n"); err == nil {
		t.Fatal("Load() error = nil")
	}
}

func TestCalendar_VersusUnit_RepeatsAsItsWhenSays(t *testing.T) {
	got, err := loadCalendar(t, `versus:
  - id: 1
    stage: 2
    chapter: 3
    difficulty: advanced
    cost: 100
    take_weapon: true
    game_mode: versus
    when: {start: "2026-11-05T08:00:00+09:00", end: "2026-11-19T08:00:00+09:00", every: 4w, until: 2027-03-31}
`)

	jst := time.FixedZone("", 9*60*60)
	want := zet.VersusUnit{ID: 1, StageID: 2, ChapterID: 3, Difficulty: 3, Cost: 100, TakeWeapon: true, GameMode: "versus"}
	if err != nil || len(got.Schedule.Versus) != 1 {
		t.Fatalf("Load() = %+v, %v", got.Schedule, err)
	}
	unit := got.Schedule.Versus[0]
	when := unit.When
	unit.When = calendar.Span{}
	if unit != want || !when.Start.Equal(time.Date(2026, 11, 5, 8, 0, 0, 0, jst)) || !when.End.Equal(time.Date(2026, 11, 19, 8, 0, 0, 0, jst)) ||
		when.Every != (calendar.Every{Days: 28}) || !when.Until.Equal(time.Date(2027, 4, 1, 8, 0, 0, 0, time.Local)) {
		t.Fatalf("Versus[0] = %+v, when %+v", unit, when)
	}
}

func TestCalendar_Dates_AreServiceDaysFrom0800To0800(t *testing.T) {
	got, err := loadCalendar(t, "grants:\n  - {id: 1, name: A, zet: 1, when: {start: 2027-04-29, end: 2027-05-05, every: 1y}}\n")

	when := got.Schedule.Grants[0].When
	if err != nil || !when.Start.Equal(time.Date(2027, 4, 29, 8, 0, 0, 0, time.Local)) || !when.End.Equal(time.Date(2027, 5, 6, 8, 0, 0, 0, time.Local)) || when.Every.Years != 1 {
		t.Fatalf("when = %+v, %v", when, err)
	}
}

func TestCalendar_Event_ConvertsAndIsAlwaysOpenWithoutAWhen(t *testing.T) {
	got, err := loadCalendar(t, `events:
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

	want := zet.Event{ID: 7, Type: zet.EventStore, StageID: 1, ChapterID: 2, Difficulty: 4, SpecialMode: 1, TakeDisposable: true,
		MultiMatch: true, GameMode: "mutation2", Name: "Headshots", Detail: "Only headshots count.", MinLevel: 5, MaxClassID: 3}
	if err != nil || len(got.Schedule.Events) != 1 || !reflect.DeepEqual(got.Schedule.Events[0], want) {
		t.Fatalf("Load() = %+v, %v", got.Schedule, err)
	}
}

// eventWhen is an event's week, in calendar file text.
const eventWhen = "when: {start: 2027-04-01, end: 2027-04-07}"

func TestCalendar_EventPrizes_ReadTheRanksAndGifts(t *testing.T) {
	got, err := loadCalendar(t, `events:
  - id: 7
    type: national
    difficulty: normal
    name: Hunt
    `+eventWhen+`
    prizes:
      - ranks: 1
        zet: 500
        items:
          - {category: title, id: 11, name: Hunter}
          - {category: weapon, id: 13, name: M60, count: 30}
      - {ranks: 11-20, zet: 100}
    participation: {item: {category: skill_item, id: 29, name: チャージャー流し, count: 2}}
`)

	if err != nil || len(got.Schedule.Events) != 1 {
		t.Fatalf("Load() = %+v, %v", got.Schedule, err)
	}
	event := got.Schedule.Events[0]
	first, band := event.Prizes[0], event.Prizes[1]
	if len(event.Prizes) != 2 || first.From != 1 || first.To != 1 || len(first.Gifts) != 2 ||
		first.Gifts[0].Zet != 500 || first.Gifts[0].Item.Kind != store.Title || first.Gifts[1].Zet != 0 || first.Gifts[1].Item.Count != 30 ||
		band.From != 11 || band.To != 20 || len(band.Gifts) != 1 || band.Gifts[0].Zet != 100 || band.Gifts[0].Name != "Hunt" {
		t.Fatalf("Prizes = %+v", event.Prizes)
	}
	if p := event.Participation; p == nil || p.Name != "Hunt" || p.Item.ItemID != 29 || p.Item.Count != 2 {
		t.Fatalf("Participation = %+v", p)
	}
}

func TestCalendar_DailyHours_AreLocalTimesOfDayAfter1970(t *testing.T) {
	got, _ := loadCalendar(t, "events:\n  - id: 1\n    type: national\n    difficulty: normal\n    daily_open: \"10:00\"\n    daily_close: \"22:30\"\n")

	open, closing := got.Schedule.Events[0].DailyOpen.Local(), got.Schedule.Events[0].DailyClose.Local()
	if open.Hour() != 10 || open.Minute() != 0 || closing.Hour() != 22 || closing.Minute() != 30 || open.Unix() <= 0 {
		t.Fatalf("daily hours %v to %v", open, closing)
	}
}

func TestCalendar_Maintenance_ReadsTheWindows(t *testing.T) {
	got, err := loadCalendar(t, `maintenance:
  warn: 30m
  windows:
    - when: {start: "2026-01-01T06:00:00+09:00", end: "2026-01-01T08:00:00+09:00", every: 1d}
    - when: {start: "2026-12-24T00:00:00+09:00", end: "2026-12-24T08:00:00+09:00"}
`)

	windows := got.Maintenance.Windows
	if err != nil || got.Maintenance.Warn != 30*time.Minute || len(windows) != 2 || windows[0].Every.Days != 1 || windows[1].End.Sub(windows[1].Start) != 8*time.Hour {
		t.Fatalf("Maintenance = %+v, %v", got.Maintenance, err)
	}
}

func TestCalendar_CoopStages_ReadsTheLists(t *testing.T) {
	got, err := loadCalendar(t, "coop_stages:\n  - when: {start: 2027-04-01, end: 2027-04-07, every: 1w}\n    stages: [1, 3, 5, 9]\n")

	if err != nil || len(got.Schedule.CoopStages) != 1 || len(got.Schedule.CoopStages[0].Stages) != 4 || got.Schedule.CoopStages[0].When.Every.Days != 7 {
		t.Fatalf("CoopStages = %+v, %v", got.Schedule.CoopStages, err)
	}
}

func TestCalendar_Grant_Converts(t *testing.T) {
	got, err := loadCalendar(t, `grants:
  - id: 3
    name: Welcome
    when: {start: "2026-11-01T08:00:00Z", end: "2026-12-01T08:00:00Z"}
    zet: 500
    item: {category: weapon, id: 6, name: AK-47, count: 5}
`)

	if err != nil || len(got.Schedule.Grants) != 1 {
		t.Fatalf("Load() = %+v, %v", got.Schedule, err)
	}
	grant := got.Schedule.Grants[0]
	want := store.Owned{Kind: store.Weapon, ItemID: 6, Name: "AK-47", Count: 5}
	if grant.ID != 3 || grant.Name != "Welcome" || grant.Zet != 500 || grant.Item != want ||
		!grant.When.Start.Equal(time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)) || !grant.When.End.Equal(time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("Grants[0] = %+v", grant)
	}
}

func TestCalendar_BadEntries_AreErrors(t *testing.T) {
	const when = "      when: {start: 2026-11-01, end: 2026-11-30}\n"
	entries := map[string]string{
		"versus, unknown difficulty": "versus:\n  - id: 3\n    difficulty: hard\n    when: {start: 2026-11-01, end: 2026-11-30}\n",
		"versus, no when":            "versus:\n  - id: 3\n    difficulty: normal\n",
		"event, bad daily time":      "events:\n  - id: 2\n    type: national\n    difficulty: normal\n    daily_open: 25h\n",
		"grant, no id":               "grants:\n    - name: A\n      zet: 1\n" + when,
		"grant, no name":             "grants:\n    - id: 1\n      zet: 1\n" + when,
		"grant, nothing given":       "grants:\n    - id: 1\n      name: A\n" + when,
		"grant, no when":             "grants:\n    - id: 1\n      name: A\n      zet: 1\n",
		"grant, ends at start":       "grants:\n    - id: 1\n      name: A\n      zet: 1\n      when: {start: \"2026-11-01T08:00:00Z\", end: \"2026-11-01T08:00:00Z\"}\n",
		"grant, listed twice":        "grants:\n    - {id: 1, name: A, zet: 1, when: {start: 2026-11-01, end: 2026-11-30}}\n    - {id: 1, name: B, zet: 1, when: {start: 2026-11-01, end: 2026-11-30}}\n",
		"grant, unknown item":        "grants:\n    - id: 1\n      name: A\n" + when + "      item: {category: hat, id: 1, name: Hat, count: 1}\n",
		"grant, no item count":       "grants:\n    - id: 1\n      name: A\n" + when + "      item: {category: weapon, id: 6, name: AK-47}\n",
		"grant, weapon, no name":     "grants:\n    - id: 1\n      name: A\n" + when + "      item: {category: weapon, id: 6, count: 1}\n",
		"maintenance, backwards":     "maintenance:\n  windows:\n    - when: {start: \"2026-12-24T08:00:00+09:00\", end: \"2026-12-24T06:00:00+09:00\"}\n",
		"maintenance, no when":       "maintenance:\n  windows:\n    - {}\n",
		"overlapping runs":           "coop_stages:\n  - when: {start: 2027-04-01, end: 2027-04-08, every: 1w}\n    stages: [1]\n",
		"unknown stage":              "coop_stages:\n  - when: {start: 2027-04-01, end: 2027-04-07, every: 1w}\n    stages: [99]\n",
		"bad repeat":                 "coop_stages:\n  - when: {start: 2027-04-01, end: 2027-04-07, every: weekly}\n    stages: [1]\n",
		"bad time":                   "coop_stages:\n  - when: {start: next thursday, end: 2027-04-07}\n    stages: [1]\n",
		"event, listed twice":        "events:\n  - {id: 2, type: national, difficulty: normal}\n  - {id: 2, type: national_ex, difficulty: normal}\n",
		"event, prizes, no when":     "events:\n  - {id: 2, name: A, type: national, difficulty: normal, prizes: [{ranks: 1, zet: 100}]}\n",
		"event, prizes overlap":      "events:\n  - {id: 2, name: A, type: national, difficulty: normal, " + eventWhen + ", prizes: [{ranks: 1-10, zet: 100}, {ranks: 10-20, zet: 50}]}\n",
		"event, backwards ranks":     "events:\n  - {id: 2, name: A, type: national, difficulty: normal, " + eventWhen + ", prizes: [{ranks: 20-11, zet: 100}]}\n",
		"event, rank 0":              "events:\n  - {id: 2, name: A, type: national, difficulty: normal, " + eventWhen + ", prizes: [{ranks: 0, zet: 100}]}\n",
		"event, empty prize":         "events:\n  - {id: 2, name: A, type: national, difficulty: normal, " + eventWhen + ", prizes: [{ranks: 1}]}\n",
		"event, title participation": "events:\n  - {id: 2, name: A, type: national, difficulty: normal, participation: {item: {category: title, id: 1, name: T}}}\n",
	}

	for name, text := range entries {
		if _, err := loadCalendar(t, text); err == nil {
			t.Errorf("%s: Load() error = nil", name)
		}
	}
}

func TestLoad_SVPGrant_NeedsNoIDOrName(t *testing.T) {
	_, err := loadCalendar(t, "grants:\n  - {id: 1, name: Bonus, when: {start: 2026-11-01, end: 2026-11-30}, item: {category: svp, count: 900}}\n")

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestNewGift_AvatarItem_KeepsTheUnitAndColor(t *testing.T) {
	got, err := config.NewGift("Outfit", 0, &config.GrantItem{Category: "upper", Unit: "firefighter_upper_avt", Color: 2, Name: "Coat"})

	if err != nil || got.Avatar == nil || got.Avatar.Name != "firefighter_upper_avt" || got.Color != 2 || got.Item.Kind != store.Upper || got.Item.Count != 1 {
		t.Fatalf("NewGift() = %+v, %v", got, err)
	}
}

func TestNewGift_Title_IsOneTitle(t *testing.T) {
	got, err := config.NewGift("Rank", 0, &config.GrantItem{Category: "title", ID: 12, Name: "Rookie"})

	if err != nil || got.Item != (store.Owned{Kind: store.Title, ItemID: 12, Name: "Rookie", Count: 1}) {
		t.Fatalf("NewGift() = %+v, %v", got, err)
	}
}

func TestNewGift_BadAvatarItems_AreErrors(t *testing.T) {
	items := map[string]config.GrantItem{
		"unknown unit":   {Category: "upper", Unit: "no_such_avt", Name: "X"},
		"wrong category": {Category: "lower", Unit: "firefighter_upper_avt", Name: "X"},
		"bad color":      {Category: "upper", Unit: "firefighter_upper_avt", Color: 100, Name: "X"},
		"no name":        {Category: "head", Unit: "santahat_head_acc"},
		"title, no id":   {Category: "title", Name: "Rookie"},
	}

	for name, item := range items {
		if _, err := config.NewGift("Gift", 0, &item); err == nil {
			t.Errorf("%s: NewGift() error = nil", name)
		}
	}
}

func TestCalendar_ErrorNamesTheCalendarFile(t *testing.T) {
	_, err := loadCalendar(t, "versus:\n  - id: 3\n    difficulty: hard\n")

	if err == nil || !strings.Contains(err.Error(), "calendar.yaml") {
		t.Fatalf("Load() error = %v", err)
	}
}

func TestCalendar_Campaigns_ReadEachKind(t *testing.T) {
	got, err := loadCalendar(t, `daily_gifts:
  - {id: 1, when: {start: 2027-01-01, end: 2027-12-31}, item: {category: svp, count: 900}}
  - {id: 2, when: {start: 2027-01-01, end: 2027-12-31}, max_level: 1, item: {category: svp, count: 330}}
login_ladders:
  - id: 1
    name: Golden Week
    when: {start: 2027-04-29, end: 2027-05-08, every: 1y}
    rounds: 2
    steps:
      - {zet: 100}
      - {zet: 100, item: {category: weapon, id: 13, name: M60, count: 2}}
returning:
  - {id: 1, name: Welcome back, when: {start: 2027-01-01, end: 2027-01-21, every: 8w}, absent_days: 15, zet: 15000}
play_ladders:
  - id: 1
    when: {start: 2027-06-09, end: 2027-06-22}
    count: {mode: coop, stage: 12, cleared: true}
    steps:
      - {at: 1, name: First clear, zet: 500}
      - {at: 3, name: Third clear, item: {category: title, id: 7, name: Harvester}}
`)

	schedule := got.Schedule
	if err != nil || len(schedule.DailyGifts) != 2 || schedule.DailyGifts[1].MaxLevel != 1 || len(schedule.LoginLadders) != 1 ||
		schedule.LoginLadders[0].Rounds != 2 || len(schedule.LoginLadders[0].Steps) != 2 || len(schedule.Returning) != 1 ||
		schedule.Returning[0].AbsentDays != 15 || len(schedule.PlayLadders) != 1 || schedule.PlayLadders[0].Filter.StageID != 12 ||
		!slices.Equal(schedule.PlayLadders[0].Filter.Modes, []int64{1}) || schedule.PlayLadders[0].Steps[1].At != 3 {
		t.Fatalf("Load() = %+v, %v", schedule, err)
	}
}

func TestCalendar_BadCampaigns_AreErrors(t *testing.T) {
	const when = "when: {start: 2027-01-01, end: 2027-01-31}"
	entries := map[string]string{
		"daily gift, title item":    "daily_gifts:\n  - {id: 1, " + when + ", item: {category: title, id: 1, name: A}}\n",
		"daily gift, no when":       "daily_gifts:\n  - {id: 1, zet: 100}\n",
		"daily gift, twice":         "daily_gifts:\n  - {id: 1, " + when + ", zet: 1}\n  - {id: 1, " + when + ", zet: 2}\n",
		"login ladder, no steps":    "login_ladders:\n  - {id: 1, name: A, " + when + "}\n",
		"login ladder, no name":     "login_ladders:\n  - {id: 1, " + when + ", steps: [{zet: 1}]}\n",
		"returning, no days away":   "returning:\n  - {id: 1, name: A, " + when + ", zet: 1}\n",
		"play ladder, bad mode":     "play_ladders:\n  - {id: 1, " + when + ", count: {mode: clan}, steps: [{at: 1, name: A, zet: 1}]}\n",
		"play ladder, step at 0":    "play_ladders:\n  - {id: 1, " + when + ", steps: [{at: 0, name: A, zet: 1}]}\n",
		"play ladder, step unnamed": "play_ladders:\n  - {id: 1, " + when + ", steps: [{at: 1, zet: 1}]}\n",
	}

	for name, text := range entries {
		if _, err := loadCalendar(t, text); err == nil {
			t.Errorf("%s: Load() error = nil", name)
		}
	}
}
