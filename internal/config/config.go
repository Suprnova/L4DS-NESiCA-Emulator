// Package config reads the configuration file of the server, the shim, or both: the stores the
// server serves, its versus schedule and events, and the shim's server, store, server key, and card keys.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"go.yaml.in/yaml/v4"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/hotkey"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

// Config is a configuration file. l4ds-server reads its server section, l4ds-shim its shim
// section, and l4ds-aio both.
type Config struct {
	Server Server `yaml:"server"`
	Shim   Shim   `yaml:"shim"`
}

// Server configures the server.
type Server struct {
	// Listen is the address shims connect to, such as ":8080".
	Listen string `yaml:"listen"`
	// Stores are the stores whose cabinets the server serves; a shim naming another is refused.
	Stores []Store      `yaml:"stores"`
	Versus []VersusUnit `yaml:"versus"`
	Events []Event      `yaml:"events"`
}

// Store is a store the server serves, as its cabinets report it to the game.
type Store struct {
	ID   uint32 `yaml:"id"`
	Name string `yaml:"name"`
	// Prefecture must not be empty.
	Prefecture string `yaml:"prefecture"`
}

// Shim configures the shim.
type Shim struct {
	// Server is the base URL of the server, such as "http://192.168.1.10:8080".
	Server string `yaml:"server"`
	// Store is the id of the store this cabinet is in, which the server must list.
	Store uint32 `yaml:"store"`
	// ServerKey is the host string the game compares with its table to choose the matching
	// master and lobby; the host must resolve to the server.
	ServerKey string `yaml:"server_key"`
	// Netcon is the port the game on this machine takes console commands on (its -netconport),
	// which card keys send to. 0 means none.
	Netcon int `yaml:"netcon"`
	// Cards are keys that put a card on a game's card reader, for machines without one.
	Cards []CardKey `yaml:"cards"`
}

// CardKey is a key that puts a card on the card reader of a game on this machine, through the
// game's netcon port, whichever program has the focus. The game reads it at its next card scan.
type CardKey struct {
	Key hotkey.Key `yaml:"key"`
	// Card is the card's 16-digit number.
	Card string `yaml:"card"`
	// Netcon is the netcon port of the game the card goes to. Left out, it is the shim's.
	Netcon int `yaml:"netcon"`
}

// VersusUnit is one map of the national versus schedule, offered from Start until End.
type VersusUnit struct {
	ID      int `yaml:"id"`
	Stage   int `yaml:"stage"`
	Chapter int `yaml:"chapter"`
	// Difficulty is easy, normal, advanced or expert.
	Difficulty string `yaml:"difficulty"`
	// Cost is the SVP charged to enter; 0 charges the operator's versus setting.
	Cost       int       `yaml:"cost"`
	TakeWeapon bool      `yaml:"take_weapon"`
	GameMode   string    `yaml:"game_mode"`
	Start      time.Time `yaml:"start"`
	End        time.Time `yaml:"end"`
}

// Event is a national or store event. It is offered during its period (always, without one)
// and its daily hours (all day, without them).
type Event struct {
	ID int `yaml:"id"`
	// Type is national, store, national_ex or store_ex. A store event is offered only when its
	// id equals the cabinet's store event id.
	Type    string `yaml:"type"`
	Stage   int    `yaml:"stage"`
	Chapter int    `yaml:"chapter"`
	// Difficulty is easy, normal, advanced or expert; God shows a fifth "GOD" badge.
	Difficulty     string `yaml:"difficulty"`
	God            bool   `yaml:"god"`
	Cost           int    `yaml:"cost"`
	TakeWeapon     bool   `yaml:"take_weapon"`
	TakeDisposable bool   `yaml:"take_disposable"`
	MultiMatch     bool   `yaml:"multi_match"`
	GameMode       string `yaml:"game_mode"`
	Name           string `yaml:"name"`
	// Detail is shown on the announcement; "|" starts a new line.
	Detail   string    `yaml:"detail"`
	MinLevel int       `yaml:"min_level"`
	MaxLevel int       `yaml:"max_level"`
	MinClass int       `yaml:"min_class"`
	MaxClass int       `yaml:"max_class"`
	Open     time.Time `yaml:"open"`
	Close    time.Time `yaml:"close"`
	// DailyOpen and DailyClose are times of day in the server's time zone, as "HH:MM"; a close
	// before the open runs past midnight.
	DailyOpen  string `yaml:"daily_open"`
	DailyClose string `yaml:"daily_close"`
}

// Default is the configuration without a file: a server on port 8080 serving store 1, the
// emulator's store in Tokyo, with no schedule, so versus and the events stay unavailable; and a
// shim in store 1 that reaches that server on the same machine with the development server key.
func Default() Config {
	return Config{
		Server: Server{
			Listen: ":8080",
			Stores: []Store{{ID: 1, Name: "L4DS Emulator", Prefecture: "Tokyo"}},
		},
		Shim: Shim{
			Server:    "http://127.0.0.1:8080",
			Store:     1,
			ServerKey: "dev.game.zet.square-enix.co.jp",
		},
	}
}

// Store returns the store the server lists under id.
func (s Server) Store(id uint32) (Store, bool) {
	for _, store := range s.Stores {
		if store.ID == id {
			return store, true
		}
	}
	return Store{}, false
}

// Load reads the configuration file at path. Settings the file leaves out keep their
// [Default] values; keys it does not know are an error.
func Load(path string) (Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return Config{}, err
	}
	config := Default()
	loader, err := yaml.NewLoader(bytes.NewReader(data), yaml.WithKnownFields())
	if err != nil {
		return Config{}, err
	}
	if err := loader.Load(&config); err != nil && !errors.Is(err, io.EOF) {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := config.Server.check(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	if err := config.Shim.resolve(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return config, nil
}

// resolve gives each card key its netcon port, and reports the first card key that has no port,
// a card number that is not 16 digits, or a key given twice.
func (s *Shim) resolve() error {
	keys := map[hotkey.Key]bool{}
	for i := range s.Cards {
		card := &s.Cards[i]
		if card.Netcon == 0 {
			card.Netcon = s.Netcon
		}
		switch {
		case card.Key == hotkey.Key{}:
			return fmt.Errorf("shim.cards: card %s has no key", card.Card)
		case keys[card.Key]:
			return fmt.Errorf("shim.cards: key %s is given twice", card.Key)
		case len(card.Card) != 16 || strings.Trim(card.Card, "0123456789") != "":
			return fmt.Errorf("shim.cards: card %q is not 16 digits", card.Card)
		case card.Netcon <= 0 || card.Netcon > 65535:
			return fmt.Errorf("shim.cards: card %s needs a netcon port, its own or shim.netcon", card.Card)
		}
		keys[card.Key] = true
	}
	return nil
}

// check reports the first problem with the server section: no stores, a store id listed twice, a
// store without a prefecture, or a schedule entry the game cannot take.
func (s Server) check() error {
	if len(s.Stores) == 0 {
		return errors.New("server.stores must list at least one store")
	}
	ids := map[uint32]bool{}
	for _, store := range s.Stores {
		if ids[store.ID] {
			return fmt.Errorf("store %d is listed twice", store.ID)
		}
		ids[store.ID] = true
		if store.Prefecture == "" {
			return fmt.Errorf("store %d: prefecture must not be empty", store.ID)
		}
	}
	_, err := s.Schedule()
	return err
}

// difficulties maps difficulty names to the game's difficulty plus 1.
var difficulties = map[string]int{"easy": 1, "normal": 2, "advanced": 3, "expert": 4}

// eventTypes maps event type names to the game's event types.
var eventTypes = map[string]int{"national": zet.EventNational, "store": zet.EventStore, "national_ex": zet.EventNationalEX, "store_ex": zet.EventStoreEX}

// Schedule returns the versus units and events as the game server offers them.
func (s Server) Schedule() (zet.Schedule, error) {
	var schedule zet.Schedule
	for _, unit := range s.Versus {
		difficulty, err := lookup("versus unit", unit.ID, "difficulty", difficulties, unit.Difficulty)
		if err != nil {
			return zet.Schedule{}, err
		}
		schedule.Versus = append(schedule.Versus, zet.VersusUnit{
			ID: unit.ID, StageID: unit.Stage, ChapterID: unit.Chapter, Difficulty: difficulty,
			Cost: unit.Cost, TakeWeapon: unit.TakeWeapon, GameMode: unit.GameMode,
			Start: unit.Start, End: unit.End,
		})
	}
	for _, event := range s.Events {
		difficulty, err := lookup("event", event.ID, "difficulty", difficulties, event.Difficulty)
		if err != nil {
			return zet.Schedule{}, err
		}
		kind, err := lookup("event", event.ID, "type", eventTypes, event.Type)
		if err != nil {
			return zet.Schedule{}, err
		}
		dailyOpen, err := timeOfDay(event.ID, event.DailyOpen)
		if err != nil {
			return zet.Schedule{}, err
		}
		dailyClose, err := timeOfDay(event.ID, event.DailyClose)
		if err != nil {
			return zet.Schedule{}, err
		}
		special := 0
		if event.God {
			special = 1
		}
		schedule.Events = append(schedule.Events, zet.Event{
			ID: event.ID, Type: kind, StageID: event.Stage, ChapterID: event.Chapter, Difficulty: difficulty,
			SpecialMode: special, Cost: event.Cost, TakeWeapon: event.TakeWeapon, TakeDisposable: event.TakeDisposable,
			MultiMatch: event.MultiMatch, GameMode: event.GameMode, Name: event.Name, Detail: event.Detail,
			MinLevel: event.MinLevel, MaxLevel: event.MaxLevel, MinClassID: event.MinClass, MaxClassID: event.MaxClass,
			Open: event.Open, Close: event.Close, DailyOpen: dailyOpen, DailyClose: dailyClose,
		})
	}
	return schedule, nil
}

func lookup(what string, id int, key string, values map[string]int, name string) (int, error) {
	value, ok := values[name]
	if !ok {
		return 0, fmt.Errorf("%s %d: unknown %s %q", what, id, key, name)
	}
	return value, nil
}

// timeOfDay reads an "HH:MM" time of day in the server's time zone, as an instant on 2 January
// 1970: the game compares only the UTC time of day, and its time functions reject times before
// 1970. An empty time is the zero time.
func timeOfDay(event int, text string) (time.Time, error) {
	if text == "" {
		return time.Time{}, nil
	}
	parsed, err := time.Parse("15:04", text)
	if err != nil {
		return time.Time{}, fmt.Errorf("event %d: daily time %q is not HH:MM", event, text)
	}
	return time.Date(1970, 1, 2, parsed.Hour(), parsed.Minute(), 0, 0, time.Local), nil
}
