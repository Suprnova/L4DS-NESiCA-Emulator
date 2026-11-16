// Package store persists players and their progress in SQLite.
package store

import (
	"database/sql"
	"errors"
	"fmt"

	_ "modernc.org/sqlite"
)

// migrations holds the schema changes in order. The database's user_version is the number of
// migrations already applied.
var migrations = []string{
	`CREATE TABLE IF NOT EXISTS players (
		id      INTEGER PRIMARY KEY,
		card_id TEXT    NOT NULL UNIQUE,
		name    TEXT    NOT NULL,
		zet     INTEGER NOT NULL DEFAULT 0,
		exp     INTEGER NOT NULL DEFAULT 0,
		score   INTEGER NOT NULL DEFAULT 0
	)`,
	`ALTER TABLE players ADD COLUMN logins INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE players ADD COLUMN chara_id INTEGER NOT NULL DEFAULT 0`,
	`CREATE TABLE chapter_clears (
		player_id  INTEGER NOT NULL REFERENCES players (id),
		stage_id   INTEGER NOT NULL,
		chapter_id INTEGER NOT NULL,
		PRIMARY KEY (player_id, stage_id, chapter_id)
	)`,
}

// ErrNotRegistered reports that no player is registered to a card.
var ErrNotRegistered = errors.New("store: card is not registered")

// ErrAlreadyRegistered reports that a card already has a player.
var ErrAlreadyRegistered = errors.New("store: card is already registered")

// Player is a registered player and their progress totals.
type Player struct {
	ID      int64
	CardID  string
	Name    string
	Zet     int64
	Exp     int64
	Score   int64
	Logins  int64
	CharaID int64
}

// Result is what a finished chapter adds to a player's totals.
type Result struct {
	Zet       int64
	Exp       int64
	Score     int64
	StageID   int64
	ChapterID int64
	Cleared   bool
}

// Clear is a chapter a player has cleared.
type Clear struct {
	StageID   int64
	ChapterID int64
}

// Store is a SQLite database of players.
type Store struct {
	db *sql.DB
}

// Open opens the database at path, creating it and applying any missing schema changes. Use
// ":memory:" for a database that lives as long as the store.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}

	db.SetMaxOpenConns(1)
	if err := migrate(db); err != nil {
		db.Close()
		return nil, err
	}
	return &Store{db: db}, nil
}

func migrate(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&applied); err != nil {
		return err
	}

	for i := applied; i < len(migrations); i++ {
		if _, err := db.Exec(migrations[i]); err != nil {
			return err
		}
		if _, err := db.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
			return err
		}
	}
	return nil
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// PlayerByCard returns the player registered to a card, or ErrNotRegistered.
func (s *Store) PlayerByCard(cardID string) (Player, error) {
	player, err := s.player(`card_id = ?`, cardID)
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, ErrNotRegistered
	}
	return player, err
}

// Register creates the player for a card, or returns ErrAlreadyRegistered.
func (s *Store) Register(cardID, name string, charaID int64) (Player, error) {
	outcome, err := s.db.Exec(`INSERT OR IGNORE INTO players (card_id, name, chara_id) VALUES (?, ?, ?)`, cardID, name, charaID)
	if err != nil {
		return Player{}, err
	}
	if rows, _ := outcome.RowsAffected(); rows == 0 {
		return Player{}, ErrAlreadyRegistered
	}
	return s.player(`card_id = ?`, cardID)
}

// Player returns the player with the given id, or sql.ErrNoRows when there is none.
func (s *Store) Player(id int64) (Player, error) { return s.player(`id = ?`, id) }

// RecordResult adds a finished chapter to a player's totals, and to their cleared chapters
// when the chapter was cleared.
func (s *Store) RecordResult(playerID int64, result Result) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	outcome, err := tx.Exec(`UPDATE players SET zet = zet + ?, exp = exp + ?, score = score + ? WHERE id = ?`,
		result.Zet, result.Exp, result.Score, playerID)
	if err != nil {
		return err
	}
	if rows, _ := outcome.RowsAffected(); rows == 0 {
		return errors.New("store: no such player")
	}

	if result.Cleared {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO chapter_clears (player_id, stage_id, chapter_id) VALUES (?, ?, ?)`,
			playerID, result.StageID, result.ChapterID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// Clears returns the chapters a player has cleared, ordered by stage and chapter.
func (s *Store) Clears(playerID int64) ([]Clear, error) {
	rows, err := s.db.Query(`SELECT stage_id, chapter_id FROM chapter_clears WHERE player_id = ? ORDER BY stage_id, chapter_id`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clears []Clear
	for rows.Next() {
		var clear Clear
		if err := rows.Scan(&clear.StageID, &clear.ChapterID); err != nil {
			return nil, err
		}
		clears = append(clears, clear)
	}
	return clears, rows.Err()
}

// RecordLogin counts one more login for a player.
func (s *Store) RecordLogin(playerID int64) error {
	return s.updatePlayer(`UPDATE players SET logins = logins + 1 WHERE id = ?`, playerID)
}

func (s *Store) updatePlayer(query string, args ...any) error {
	outcome, err := s.db.Exec(query, args...)
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
	err := s.db.QueryRow(`SELECT id, card_id, name, zet, exp, score, logins, chara_id FROM players WHERE `+where, arg).
		Scan(&player.ID, &player.CardID, &player.Name, &player.Zet, &player.Exp, &player.Score, &player.Logins, &player.CharaID)
	return player, err
}
