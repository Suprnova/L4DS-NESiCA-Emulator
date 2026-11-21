// Package config reads the configuration file of the server, the shim, or both, and the server's
// calendar file: the stores the server serves and what it offers when, and the shim's server,
// store, server key, and card keys.
package config

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"go.yaml.in/yaml/v4"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/hotkey"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/maintenance"
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
	Stores []Store `yaml:"stores"`
	// Calendar is the calendar file's path, relative to the configuration file's folder; left out,
	// nothing is scheduled.
	Calendar string `yaml:"calendar"`
	// Schedule and Maintenance are what the calendar file schedules, read by [Load].
	Schedule    zet.Schedule         `yaml:"-"`
	Maintenance maintenance.Schedule `yaml:"-"`
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

// Load reads the configuration file at path, and the calendar file it names. Settings the file
// leaves out keep their [Default] values; keys either file does not know are an error.
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
	if err := config.Server.readCalendar(filepath.Dir(path)); err != nil {
		return Config{}, err
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

// check reports the first problem with the server section: no stores, a store id listed twice, or
// a store without a prefecture.
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
	return nil
}

// readCalendar reads the server's calendar file, if it names one, into its schedule, relative to
// dir, the configuration file's folder.
func (s *Server) readCalendar(dir string) error {
	if s.Calendar == "" {
		return nil
	}
	path := s.Calendar
	if !filepath.IsAbs(path) {
		path = filepath.Join(dir, path)
	}
	c, err := loadCalendar(path)
	if err != nil {
		return err
	}
	if s.Schedule, err = c.Schedule(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	if s.Maintenance, err = c.MaintenanceSchedule(); err != nil {
		return fmt.Errorf("%s: %w", path, err)
	}
	return nil
}
