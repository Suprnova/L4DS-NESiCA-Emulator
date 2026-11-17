// Package config reads the server's configuration file: the store it presents to the game, the
// matching server key, and the versus schedule and events.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"time"

	"go.yaml.in/yaml/v4"

	"l4ds-nesica-emulator/internal/zet"
)

// Config is the server's configuration file.
type Config struct {
	Store Store `yaml:"store"`
	// ServerKey is the host string the game compares with its table to choose the matching
	// master and lobby.
	ServerKey string       `yaml:"server_key"`
	Versus    []VersusUnit `yaml:"versus"`
	Events    []Event      `yaml:"events"`
}

// Store is the store the game reports itself in.
type Store struct {
	Name string `yaml:"name"`
	// Prefecture must not be empty.
	Prefecture string `yaml:"prefecture"`
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

// Default is the configuration without a file: the emulator's store in Tokyo, the development
// server key and no schedule, so versus and the events stay unavailable.
func Default() Config {
	return Config{
		Store:     Store{Name: "L4DS Emulator", Prefecture: "Tokyo"},
		ServerKey: "dev.game.zet.square-enix.co.jp",
	}
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
	if config.Store.Prefecture == "" {
		return Config{}, fmt.Errorf("%s: store.prefecture must not be empty", path)
	}
	if _, err := config.Schedule(); err != nil {
		return Config{}, fmt.Errorf("%s: %w", path, err)
	}
	return config, nil
}

// difficulties maps difficulty names to the game's difficulty plus 1.
var difficulties = map[string]int{"easy": 1, "normal": 2, "advanced": 3, "expert": 4}

// eventTypes maps event type names to the game's event types.
var eventTypes = map[string]int{"national": zet.EventNational, "store": zet.EventStore, "national_ex": zet.EventNationalEX, "store_ex": zet.EventStoreEX}

// Schedule returns the versus units and events as the game server offers them.
func (c Config) Schedule() (zet.Schedule, error) {
	var schedule zet.Schedule
	for _, unit := range c.Versus {
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
	for _, event := range c.Events {
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
