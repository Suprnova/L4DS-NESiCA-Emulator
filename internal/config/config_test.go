package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/hotkey"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
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
	if err != nil || !reflect.DeepEqual(got.Shim, want.Shim) || got.Server.Listen != want.Server.Listen || !slices.Equal(got.Server.Stores, want.Server.Stores) ||
		len(got.Server.Versus) != 0 || len(got.Server.Events) != 0 {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_OnlyComments_KeepsTheDefaults(t *testing.T) {
	got, err := config.Load(writeConfig(t, "# nothing set yet\n"))

	if err != nil || !slices.Equal(got.Server.Stores, config.Default().Server.Stores) {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_ReadsTheStoresAndTheShim(t *testing.T) {
	path := writeConfig(t, `server:
  listen: ":9000"
  stores:
    - id: 7
      name: Arcade
      prefecture: Osaka
shim:
  server: http://10.0.0.2:9000
  store: 7
  server_key: example.test
`)

	got, err := config.Load(path)

	want := config.Config{
		Server: config.Server{Listen: ":9000", Stores: []config.Store{{ID: 7, Name: "Arcade", Prefecture: "Osaka"}}},
		Shim:   config.Shim{Server: "http://10.0.0.2:9000", Store: 7, ServerKey: "example.test"},
	}
	if err != nil || !reflect.DeepEqual(got.Shim, want.Shim) || got.Server.Listen != want.Server.Listen || !slices.Equal(got.Server.Stores, want.Server.Stores) {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestServer_Store_FindsAListedStore(t *testing.T) {
	server := config.Server{Stores: []config.Store{{ID: 3, Prefecture: "Kyoto"}, {ID: 7, Prefecture: "Osaka"}}}

	got, listed := server.Store(7)
	_, unlisted := server.Store(8)

	if !listed || got.Prefecture != "Osaka" || unlisted {
		t.Fatalf("Store(7) = %+v, %t; Store(8) listed = %t", got, listed, unlisted)
	}
}

func TestLoad_UnknownKey_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "shim:\n  stroe: 1\n"))

	if err == nil {
		t.Fatal("Load() err = nil")
	}
}

func TestLoad_EmptyPrefecture_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "server:\n  stores:\n    - id: 4\n      prefecture: \"\"\n"))

	if err == nil || !strings.Contains(err.Error(), "store 4: prefecture") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestLoad_StoreListedTwice_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "server:\n  stores:\n    - id: 4\n      prefecture: A\n    - id: 4\n      prefecture: B\n"))

	if err == nil || !strings.Contains(err.Error(), "store 4 is listed twice") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestLoad_NoStores_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "server:\n  stores: []\n"))

	if err == nil || !strings.Contains(err.Error(), "at least one store") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestLoad_UnknownDifficulty_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "server:\n  versus:\n    - id: 3\n      difficulty: hard\n"))

	if err == nil || !strings.Contains(err.Error(), "versus unit 3") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestSchedule_ConvertsAVersusUnit(t *testing.T) {
	path := writeConfig(t, `server:
  versus:
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

	got, err := loaded.Server.Schedule()

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
	path := writeConfig(t, `server:
  events:
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

	got, err := loaded.Server.Schedule()

	want := zet.Event{ID: 7, Type: zet.EventStore, StageID: 1, ChapterID: 2, Difficulty: 4, SpecialMode: 1, TakeDisposable: true,
		MultiMatch: true, GameMode: "mutation2", Name: "Headshots", Detail: "Only headshots count.", MinLevel: 5, MaxClassID: 3}
	if err != nil || len(got.Events) != 1 || got.Events[0] != want {
		t.Fatalf("Schedule() = %+v, %v", got, err)
	}
}

func TestSchedule_DailyHours_AreLocalTimesOfDayAfter1970(t *testing.T) {
	path := writeConfig(t, "server:\n  events:\n    - id: 1\n      type: national\n      difficulty: normal\n      daily_open: \"10:00\"\n      daily_close: \"22:30\"\n")
	loaded, _ := config.Load(path)

	got, _ := loaded.Server.Schedule()

	open, closing := got.Events[0].DailyOpen.Local(), got.Events[0].DailyClose.Local()
	if open.Hour() != 10 || open.Minute() != 0 || closing.Hour() != 22 || closing.Minute() != 30 || open.Unix() <= 0 {
		t.Fatalf("daily hours %v to %v", open, closing)
	}
}

func TestLoad_ExampleFile_Loads(t *testing.T) {
	got, err := config.Load(filepath.Join("..", "..", "l4ds.example.yaml"))

	if err != nil || len(got.Server.Versus) != 1 || len(got.Server.Events) != 2 || len(got.Server.Stores) != 1 {
		t.Fatalf("Load() = %+v, %v", got, err)
	}
}

func TestLoad_BadDailyTime_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "server:\n  events:\n    - id: 2\n      type: national\n      difficulty: normal\n      daily_open: 25h\n"))

	if err == nil || !strings.Contains(err.Error(), "event 2") {
		t.Fatalf("Load() err = %v", err)
	}
}

func TestLoad_CardKeys_TakeTheShimsNetconPortUnlessTheyHaveTheirOwn(t *testing.T) {
	path := writeConfig(t, `shim:
  netcon: 2121
  cards:
    - key: F5
      card: "7020392000000001"
    - key: ctrl+F6
      card: "7020392000000002"
      netcon: 2122
`)

	got, err := config.Load(path)

	f5, _ := hotkey.Parse("F5")
	f6, _ := hotkey.Parse("Ctrl+F6")
	want := []config.CardKey{{Key: f5, Card: "7020392000000001", Netcon: 2121}, {Key: f6, Card: "7020392000000002", Netcon: 2122}}
	if err != nil || !slices.Equal(got.Shim.Cards, want) {
		t.Fatalf("Load() = %+v, %v", got.Shim.Cards, err)
	}
}

func TestLoad_BadCardKeys_AreErrors(t *testing.T) {
	cases := map[string]string{
		"no port": `
  cards:
    - {key: F5, card: "7020392000000001"}`,
		"short card": `
  netcon: 2121
  cards:
    - {key: F5, card: "702039200000001"}`,
		"no key": `
  netcon: 2121
  cards:
    - {card: "7020392000000001"}`,
		"unknown key": `
  netcon: 2121
  cards:
    - {key: Enter, card: "7020392000000001"}`,
		"key twice": `
  netcon: 2121
  cards:
    - {key: F5, card: "7020392000000001"}
    - {key: f5, card: "7020392000000002"}`,
		"port too big": `
  cards:
    - {key: F5, card: "7020392000000001", netcon: 70000}`,
	}
	for name, cards := range cases {
		_, err := config.Load(writeConfig(t, "shim:"+cards))

		if err == nil {
			t.Errorf("%s: Load() succeeded", name)
		}
	}
}

func TestLoad_Maintenance_ReadsTheWindows(t *testing.T) {
	path := writeConfig(t, `
server:
  maintenance:
    warn: 30m
    daily_start: "06:00"
    daily_end: "08:00"
    windows:
      - start: 2026-12-24T00:00:00+09:00
        end: 2026-12-24T08:00:00+09:00
`)

	loaded, err := config.Load(path)
	got, scheduleErr := loaded.Server.Maintenance.Schedule()

	if err != nil || scheduleErr != nil || got.Warn != 30*time.Minute || got.Daily == nil || got.Daily.Start != 6*time.Hour || got.Daily.End != 8*time.Hour ||
		len(got.Windows) != 1 || got.Windows[0].End.Sub(got.Windows[0].Start) != 8*time.Hour {
		t.Fatalf("Schedule() = %+v, %v (load %v)", got, scheduleErr, err)
	}
}

func TestLoad_MaintenanceDailyStartWithoutEnd_IsAnError(t *testing.T) {
	_, err := config.Load(writeConfig(t, "server:\n  maintenance:\n    daily_start: \"06:00\"\n"))

	if err == nil {
		t.Fatal("Load() error = nil")
	}
}

func TestLoad_MaintenanceWindowEndingBeforeItStarts_IsAnError(t *testing.T) {
	path := writeConfig(t, `
server:
  maintenance:
    windows:
      - start: 2026-12-24T08:00:00+09:00
        end: 2026-12-24T06:00:00+09:00
`)

	_, err := config.Load(path)

	if err == nil {
		t.Fatal("Load() error = nil")
	}
}

func TestSchedule_ConvertsAGrant(t *testing.T) {
	path := writeConfig(t, `server:
  grants:
    - id: 3
      name: Welcome
      start: 2026-11-01T08:00:00Z
      end: 2026-12-01T08:00:00Z
      zet: 500
      item:
        category: weapon
        id: 6
        name: AK-47
        count: 5
`)
	loaded, _ := config.Load(path)

	got, err := loaded.Server.Schedule()

	if err != nil || len(got.Grants) != 1 {
		t.Fatalf("Schedule() = %+v, %v", got, err)
	}
	grant := got.Grants[0]
	want := store.Owned{Kind: store.Weapon, ItemID: 6, Name: "AK-47", Count: 5}
	if grant.ID != 3 || grant.Name != "Welcome" || grant.Zet != 500 || grant.Item != want ||
		!grant.Start.Equal(time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)) || !grant.End.Equal(time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)) {
		t.Fatalf("Grants[0] = %+v", grant)
	}
}

func TestLoad_BadGrants_AreErrors(t *testing.T) {
	const window = "      start: 2026-11-01T08:00:00Z\n      end: 2026-12-01T08:00:00Z\n"
	grants := map[string]string{
		"no id":           "    - name: A\n      zet: 1\n" + window,
		"no name":         "    - id: 1\n      zet: 1\n" + window,
		"nothing given":   "    - id: 1\n      name: A\n" + window,
		"ends at start":   "    - id: 1\n      name: A\n      zet: 1\n      start: 2026-11-01T08:00:00Z\n      end: 2026-11-01T08:00:00Z\n",
		"listed twice":    "    - id: 1\n      name: A\n      zet: 1\n" + window + "    - id: 1\n      name: B\n      zet: 1\n" + window,
		"unknown item":    "    - id: 1\n      name: A\n" + window + "      item: {category: hat, id: 1, name: Hat, count: 1}\n",
		"no item count":   "    - id: 1\n      name: A\n" + window + "      item: {category: weapon, id: 6, name: AK-47}\n",
		"weapon, no name": "    - id: 1\n      name: A\n" + window + "      item: {category: weapon, id: 6, count: 1}\n",
	}

	for name, grant := range grants {
		if _, err := config.Load(writeConfig(t, "server:\n  grants:\n"+grant)); err == nil {
			t.Errorf("%s: Load() error = nil", name)
		}
	}
}

func TestLoad_SVPGrant_NeedsNoIDOrName(t *testing.T) {
	path := writeConfig(t, "server:\n  grants:\n    - id: 1\n      name: Bonus\n      start: 2026-11-01T08:00:00Z\n      end: 2026-12-01T08:00:00Z\n      item: {category: svp, count: 900}\n")

	_, err := config.Load(path)

	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
}
