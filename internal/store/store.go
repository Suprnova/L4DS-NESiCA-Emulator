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
	`ALTER TABLE players ADD COLUMN coop_class_id INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE players ADD COLUMN coop_class_point INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE players ADD COLUMN vs_class_id INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE players ADD COLUMN vs_class_point INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE players ADD COLUMN master_volume INTEGER NOT NULL DEFAULT 5`,
	`ALTER TABLE players ADD COLUMN bgm_volume INTEGER NOT NULL DEFAULT 5`,
	`ALTER TABLE players ADD COLUMN mouse_horizontal_reverse INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE players ADD COLUMN mouse_vertical_reverse INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE players ADD COLUMN mouse_sensitivity INTEGER NOT NULL DEFAULT 3`,
	`ALTER TABLE players ADD COLUMN mouse_acceleration INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE players ADD COLUMN key_map INTEGER NOT NULL DEFAULT 1`,
	`ALTER TABLE players ADD COLUMN vs_plays INTEGER NOT NULL DEFAULT 0`,
	`ALTER TABLE players ADD COLUMN vs_wins INTEGER NOT NULL DEFAULT 0`,
}

// settingsColumns lists the columns holding a player's [Settings], in field order.
const settingsColumns = `master_volume, bgm_volume, mouse_horizontal_reverse, mouse_vertical_reverse,
	mouse_sensitivity, mouse_acceleration, key_map`

// ErrNotRegistered reports that no player is registered to a card.
var ErrNotRegistered = errors.New("store: card is not registered")

// ErrAlreadyRegistered reports that a card already has a player.
var ErrAlreadyRegistered = errors.New("store: card is already registered")

// Player is a registered player and their progress totals. Class ids count from 1, the
// lowest class (F5 in co-op, F in versus).
type Player struct {
	ID             int64
	CardID         string
	Name           string
	Zet            int64
	Exp            int64
	Score          int64
	Logins         int64
	CharaID        int64
	CoopClassID    int64
	CoopClassPoint int64
	VSClassID      int64
	VSClassPoint   int64
	VSPlays        int64
	VSWins         int64
	Settings       Settings
}

// Settings are the options a player changes on the cabinet's settings screen. A new player has
// the volumes at 5, the mouse unreversed at sensitivity 3 without acceleration, and key map 1.
type Settings struct {
	MasterVolume           int64
	BGMVolume              int64
	MouseHorizontalReverse int64
	MouseVerticalReverse   int64
	MouseSensitivity       int64
	MouseAcceleration      int64
	KeyMap                 int64
}

// DefaultSettings are the settings a new player starts with.
var DefaultSettings = Settings{MasterVolume: 5, BGMVolume: 5, MouseSensitivity: 3, KeyMap: 1}

// Result is what a finished chapter adds to a player's totals. The game computes the classes
// itself: CoopClassID and VSClassID are the player's classes after the chapter (0 keeps the
// stored class), and CoopClassPoint and VSClassPoint are the changes in co-op and versus class
// points, which may be negative. VSPlays and VSWins add to the player's versus record.
type Result struct {
	Zet            int64
	Exp            int64
	Score          int64
	CoopClassPoint int64
	VSClassPoint   int64
	CoopClassID    int64
	VSClassID      int64
	VSPlays        int64
	VSWins         int64
	StageID        int64
	ChapterID      int64
	Cleared        bool
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

	outcome, err := tx.Exec(`UPDATE players SET zet = zet + ?, exp = exp + ?, score = score + ?,
		coop_class_point = coop_class_point + ?, vs_class_point = vs_class_point + ?,
		coop_class_id = COALESCE(NULLIF(?, 0), coop_class_id),
		vs_class_id = COALESCE(NULLIF(?, 0), vs_class_id),
		vs_plays = vs_plays + ?, vs_wins = vs_wins + ?
		WHERE id = ?`,
		result.Zet, result.Exp, result.Score, result.CoopClassPoint, result.VSClassPoint, result.CoopClassID, result.VSClassID,
		result.VSPlays, result.VSWins, playerID)
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

// UpdateSettings replaces the settings of the player registered to a card.
func (s *Store) UpdateSettings(cardID string, settings Settings) error {
	return s.updatePlayer(`UPDATE players SET (`+settingsColumns+`) = (?, ?, ?, ?, ?, ?, ?) WHERE card_id = ?`,
		settings.MasterVolume, settings.BGMVolume, settings.MouseHorizontalReverse, settings.MouseVerticalReverse,
		settings.MouseSensitivity, settings.MouseAcceleration, settings.KeyMap, cardID)
}

// Rename changes the name of the player registered to a card.
func (s *Store) Rename(cardID, name string) error {
	return s.updatePlayer(`UPDATE players SET name = ? WHERE card_id = ?`, name, cardID)
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

// TopScorers returns up to limit players with a score, highest score first.
func (s *Store) TopScorers(limit int) ([]Player, error) {
	return s.players(`score > 0 ORDER BY score DESC, id LIMIT ?`, limit)
}

// TopVersusPlayers returns up to limit players who have played versus, most versus class points
// first.
func (s *Store) TopVersusPlayers(limit int) ([]Player, error) {
	return s.players(`vs_plays > 0 ORDER BY vs_class_point DESC, id LIMIT ?`, limit)
}

// playerColumns lists the columns scanPlayer reads, in order.
const playerColumns = `id, card_id, name, zet, exp, score, logins, chara_id,
	coop_class_id, coop_class_point, vs_class_id, vs_class_point, vs_plays, vs_wins, ` + settingsColumns

func scanPlayer(row interface{ Scan(...any) error }) (Player, error) {
	var player Player
	err := row.Scan(&player.ID, &player.CardID, &player.Name, &player.Zet, &player.Exp, &player.Score, &player.Logins, &player.CharaID,
		&player.CoopClassID, &player.CoopClassPoint, &player.VSClassID, &player.VSClassPoint, &player.VSPlays, &player.VSWins,
		&player.Settings.MasterVolume, &player.Settings.BGMVolume, &player.Settings.MouseHorizontalReverse,
		&player.Settings.MouseVerticalReverse, &player.Settings.MouseSensitivity, &player.Settings.MouseAcceleration,
		&player.Settings.KeyMap)
	return player, err
}

func (s *Store) players(where string, args ...any) ([]Player, error) {
	rows, err := s.db.Query(`SELECT `+playerColumns+` FROM players WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var players []Player
	for rows.Next() {
		player, err := scanPlayer(rows)
		if err != nil {
			return nil, err
		}
		players = append(players, player)
	}
	return players, rows.Err()
}

func (s *Store) player(where string, arg any) (Player, error) {
	return scanPlayer(s.db.QueryRow(`SELECT `+playerColumns+` FROM players WHERE `+where, arg))
}
