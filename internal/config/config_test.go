package config_test

import (
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/hotkey"
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
		len(got.Server.Schedule.Versus) != 0 || len(got.Server.Schedule.Events) != 0 {
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
