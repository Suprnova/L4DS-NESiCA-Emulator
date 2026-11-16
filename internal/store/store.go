// Package store persists players and their progress in SQLite.
package store

import (
	"database/sql"
	"errors"

	_ "modernc.org/sqlite"
)

const schema = `
CREATE TABLE IF NOT EXISTS players (
	id      INTEGER PRIMARY KEY,
	card_id TEXT    NOT NULL UNIQUE,
	name    TEXT    NOT NULL,
	zet     INTEGER NOT NULL DEFAULT 0,
	exp     INTEGER NOT NULL DEFAULT 0,
	score   INTEGER NOT NULL DEFAULT 0
)`

// DefaultName is the name of a player created by their first card scan.
const DefaultName = "Survivor"

// Player is a registered player and their progress totals.
type Player struct {
	ID     int64
	CardID string
	Name   string
	Zet    int64
	Exp    int64
	Score  int64
}

// Result is what a finished chapter adds to a player's totals.
type Result struct {
	Zet   int64
	Exp   int64
	Score int64
}

// Store is a SQLite database of players.
type Store struct {
	db *sql.DB
}

// Open opens the database at path, creating it and its schema when missing. Use ":memory:" for
// a database that lives as long as the store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	if _, err := db.Exec(schema); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// PlayerByCard returns the player registered to a card, registering a new one with
// DefaultName when the card is unknown.
func (s *Store) PlayerByCard(cardID string) (Player, error) {
	if _, err := s.db.Exec(`INSERT OR IGNORE INTO players (card_id, name) VALUES (?, ?)`, cardID, DefaultName); err != nil {
		return Player{}, err
	}
	return s.player(`card_id = ?`, cardID)
}

// Player returns the player with the given id, or sql.ErrNoRows when there is none.
func (s *Store) Player(id int64) (Player, error) { return s.player(`id = ?`, id) }

// RecordResult adds a finished chapter to a player's totals.
func (s *Store) RecordResult(playerID int64, result Result) error {
	outcome, err := s.db.Exec(`UPDATE players SET zet = zet + ?, exp = exp + ?, score = score + ? WHERE id = ?`,
		result.Zet, result.Exp, result.Score, playerID)
	if err != nil {
		return err
	}
	if rows, _ := outcome.RowsAffected(); rows == 0 {
		return errors.New("store: no such player")
	}
	return nil
}

func (s *Store) player(where string, arg any) (Player, error) {
	var player Player
	err := s.db.QueryRow(`SELECT id, card_id, name, zet, exp, score FROM players WHERE `+where, arg).
		Scan(&player.ID, &player.CardID, &player.Name, &player.Zet, &player.Exp, &player.Score)
	return player, err
}
