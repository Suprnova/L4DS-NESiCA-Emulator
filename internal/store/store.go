// Package store persists players, their cards, and their results in SQLite.
package store

import (
	"database/sql"
	"errors"
	"fmt"
	"time"

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
	// Cards move to their own table, so a player can outlive a card (reissues and takeovers).
	// SQLite cannot drop a UNIQUE column, so players is rebuilt without card_id.
	`CREATE TABLE cards (
		card_id   TEXT    PRIMARY KEY,
		player_id INTEGER NOT NULL REFERENCES players (id)
	);
	INSERT INTO cards (card_id, player_id) SELECT card_id, id FROM players;
	CREATE TABLE players_new (
		id                       INTEGER PRIMARY KEY,
		name                     TEXT    NOT NULL,
		zet                      INTEGER NOT NULL DEFAULT 0,
		exp                      INTEGER NOT NULL DEFAULT 0,
		score                    INTEGER NOT NULL DEFAULT 0,
		logins                   INTEGER NOT NULL DEFAULT 0,
		chara_id                 INTEGER NOT NULL DEFAULT 0,
		coop_class_id            INTEGER NOT NULL DEFAULT 1,
		coop_class_point         INTEGER NOT NULL DEFAULT 0,
		vs_class_id              INTEGER NOT NULL DEFAULT 1,
		vs_class_point           INTEGER NOT NULL DEFAULT 0,
		vs_plays                 INTEGER NOT NULL DEFAULT 0,
		vs_wins                  INTEGER NOT NULL DEFAULT 0,
		master_volume            INTEGER NOT NULL DEFAULT 5,
		bgm_volume               INTEGER NOT NULL DEFAULT 5,
		mouse_horizontal_reverse INTEGER NOT NULL DEFAULT 0,
		mouse_vertical_reverse   INTEGER NOT NULL DEFAULT 0,
		mouse_sensitivity        INTEGER NOT NULL DEFAULT 3,
		mouse_acceleration       INTEGER NOT NULL DEFAULT 0,
		key_map                  INTEGER NOT NULL DEFAULT 1
	);
	INSERT INTO players_new SELECT id, name, zet, exp, score, logins, chara_id,
		coop_class_id, coop_class_point, vs_class_id, vs_class_point, vs_plays, vs_wins,
		master_volume, bgm_volume, mouse_horizontal_reverse, mouse_vertical_reverse,
		mouse_sensitivity, mouse_acceleration, key_map FROM players;
	DROP TABLE players;
	ALTER TABLE players_new RENAME TO players`,
	`CREATE TABLE results (
		id               INTEGER PRIMARY KEY,
		player_id        INTEGER NOT NULL REFERENCES players (id),
		played_at        INTEGER NOT NULL,
		game_mode        INTEGER NOT NULL,
		stage_id         INTEGER NOT NULL,
		chapter_id       INTEGER NOT NULL,
		cleared          INTEGER NOT NULL,
		zet              INTEGER NOT NULL,
		exp              INTEGER NOT NULL,
		score            INTEGER NOT NULL,
		coop_class_point INTEGER NOT NULL,
		vs_class_point   INTEGER NOT NULL,
		coop_class_id    INTEGER NOT NULL,
		vs_class_id      INTEGER NOT NULL,
		vs_plays         INTEGER NOT NULL,
		vs_wins          INTEGER NOT NULL
	);
	CREATE INDEX results_by_time ON results (played_at)`,
	`ALTER TABLE players ADD COLUMN home_store INTEGER NOT NULL DEFAULT 1`,
	`CREATE TABLE equipment (
		player_id INTEGER NOT NULL REFERENCES players (id),
		kind      INTEGER NOT NULL,
		slot      INTEGER NOT NULL,
		item_id   INTEGER NOT NULL,
		name      TEXT    NOT NULL,
		count     INTEGER NOT NULL,
		PRIMARY KEY (player_id, kind, slot)
	)`,
	`CREATE TABLE owned_items (
		player_id INTEGER NOT NULL REFERENCES players (id),
		kind      INTEGER NOT NULL,
		item_id   INTEGER NOT NULL,
		name      TEXT    NOT NULL,
		count     INTEGER NOT NULL,
		PRIMARY KEY (player_id, kind, item_id)
	);
	CREATE TABLE grant_deliveries (
		player_id    INTEGER NOT NULL REFERENCES players (id),
		grant_id     INTEGER NOT NULL,
		delivered_at INTEGER NOT NULL,
		PRIMARY KEY (player_id, grant_id)
	)`,
	`CREATE TABLE queued_grants (
		id           INTEGER PRIMARY KEY,
		player_id    INTEGER NOT NULL REFERENCES players (id),
		name         TEXT    NOT NULL,
		zet          INTEGER NOT NULL,
		kind         INTEGER NOT NULL,
		item_id      INTEGER NOT NULL,
		item_name    TEXT    NOT NULL,
		item_count   INTEGER NOT NULL,
		queued_at    INTEGER NOT NULL,
		delivered_at INTEGER
	)`,
	// A grant that repeats is delivered once in each run, so deliveries record the run; every
	// earlier one was of a grant's only run, 0.
	`CREATE TABLE grant_deliveries_new (
		player_id    INTEGER NOT NULL REFERENCES players (id),
		grant_id     INTEGER NOT NULL,
		run          INTEGER NOT NULL,
		delivered_at INTEGER NOT NULL,
		PRIMARY KEY (player_id, grant_id, run)
	);
	INSERT INTO grant_deliveries_new SELECT player_id, grant_id, 0, delivered_at FROM grant_deliveries;
	DROP TABLE grant_deliveries;
	ALTER TABLE grant_deliveries_new RENAME TO grant_deliveries`,
	`CREATE TABLE login_days (
		player_id INTEGER NOT NULL REFERENCES players (id),
		day       INTEGER NOT NULL,
		PRIMARY KEY (player_id, day)
	)`,
}

// settingsColumns lists the columns holding a player's [Settings], in field order.
const settingsColumns = `master_volume, bgm_volume, mouse_horizontal_reverse, mouse_vertical_reverse,
	mouse_sensitivity, mouse_acceleration, key_map`

// ErrNotRegistered reports that no player is registered to a card.
var ErrNotRegistered = errors.New("store: card is not registered")

// ErrAlreadyRegistered reports that a card already has a player.
var ErrAlreadyRegistered = errors.New("store: card is already registered")

// ErrNoPlayer reports that the player a lookup or update names, by id or by card, does not exist.
var ErrNoPlayer = errors.New("store: no such player")

// Player is a registered player and their progress totals. Class ids count from 1, the
// lowest class (F5 in co-op, F in versus).
type Player struct {
	ID             int64
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
	// HomeStore is the id of the store the player registered at, as cabinets know it. Players
	// registered before home stores were recorded have store 1, the only store there was then.
	HomeStore int64
	Settings  Settings
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

// Result is a finished chapter: when it was played, in which game mode, stage and chapter, and
// what it adds to the player's totals. The game computes the classes itself: CoopClassID and
// VSClassID are the player's classes after the chapter (0 keeps the stored class), and
// CoopClassPoint and VSClassPoint are the changes in co-op and versus class points, which may be
// negative. VSPlays and VSWins add to the player's versus record. Played is kept to the second.
type Result struct {
	Played         time.Time
	GameMode       int64
	StageID        int64
	ChapterID      int64
	Cleared        bool
	Zet            int64
	Exp            int64
	Score          int64
	CoopClassPoint int64
	VSClassPoint   int64
	CoopClassID    int64
	VSClassID      int64
	VSPlays        int64
	VSWins         int64
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
// ":memory:" for a database that lives as long as the store. Foreign keys are enforced, and a
// write waits up to five seconds while another process holds the lock.
func Open(path string) (*Store, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=foreign_keys(1)&_pragma=busy_timeout(5000)")
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

// migrate applies the missing migrations with foreign keys off, as rebuilding a table needs, and
// checks the keys before turning them back on.
func migrate(db *sql.DB) error {
	var applied int
	if err := db.QueryRow(`PRAGMA user_version`).Scan(&applied); err != nil {
		return err
	}
	if applied == len(migrations) {
		return nil
	}

	if _, err := db.Exec(`PRAGMA foreign_keys = OFF`); err != nil {
		return err
	}
	for i := applied; i < len(migrations); i++ {
		if err := applyMigration(db, i); err != nil {
			return fmt.Errorf("store: migration %d: %w", i+1, err)
		}
	}
	rows, err := db.Query(`PRAGMA foreign_key_check`)
	if err != nil {
		return err
	}
	violated := rows.Next()
	rows.Close()
	if violated {
		return errors.New("store: migrated database has rows whose foreign keys point nowhere")
	}
	_, err = db.Exec(`PRAGMA foreign_keys = ON`)
	return err
}

// applyMigration applies migration i and records it in one transaction, so a failure leaves
// neither the change nor the new version behind.
func applyMigration(db *sql.DB, i int) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	if _, err := tx.Exec(migrations[i]); err != nil {
		return err
	}
	if _, err := tx.Exec(fmt.Sprintf(`PRAGMA user_version = %d`, i+1)); err != nil {
		return err
	}
	return tx.Commit()
}

// Close closes the database.
func (s *Store) Close() error { return s.db.Close() }

// byCard selects the player registered to a card.
const byCard = `id = (SELECT player_id FROM cards WHERE card_id = ?)`

// PlayerByCard returns the player registered to a card, or ErrNotRegistered.
func (s *Store) PlayerByCard(cardID string) (Player, error) {
	player, err := s.player(byCard, cardID)
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, ErrNotRegistered
	}
	return player, err
}

// Register creates a player registered at the store homeStore and registers a card to them, or
// returns ErrAlreadyRegistered.
func (s *Store) Register(cardID, name string, charaID, homeStore int64) (Player, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return Player{}, err
	}
	defer tx.Rollback()

	created, err := tx.Exec(`INSERT INTO players (name, chara_id, home_store) VALUES (?, ?, ?)`, name, charaID, homeStore)
	if err != nil {
		return Player{}, err
	}
	id, err := created.LastInsertId()
	if err != nil {
		return Player{}, err
	}
	card, err := tx.Exec(`INSERT OR IGNORE INTO cards (card_id, player_id) VALUES (?, ?)`, cardID, id)
	if err != nil {
		return Player{}, err
	}
	if rows, _ := card.RowsAffected(); rows == 0 {
		return Player{}, ErrAlreadyRegistered
	}
	if err := tx.Commit(); err != nil {
		return Player{}, err
	}
	return s.Player(id)
}

// Player returns the player with the given id, or ErrNoPlayer.
func (s *Store) Player(id int64) (Player, error) {
	player, err := s.player(`id = ?`, id)
	if errors.Is(err, sql.ErrNoRows) {
		return Player{}, ErrNoPlayer
	}
	return player, err
}

// RecordResult stores a finished chapter, adds it to the player's totals, and adds the chapter
// to their cleared chapters when it was cleared.
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
		return ErrNoPlayer
	}

	if _, err := tx.Exec(`INSERT INTO results (player_id, `+resultColumns+`) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		playerID, result.Played.Unix(), result.GameMode, result.StageID, result.ChapterID, result.Cleared,
		result.Zet, result.Exp, result.Score, result.CoopClassPoint, result.VSClassPoint, result.CoopClassID, result.VSClassID,
		result.VSPlays, result.VSWins); err != nil {
		return err
	}
	if result.Cleared {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO chapter_clears (player_id, stage_id, chapter_id) VALUES (?, ?, ?)`,
			playerID, result.StageID, result.ChapterID); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// resultColumns lists the columns of a result row after its player, in [Result] field order.
const resultColumns = `played_at, game_mode, stage_id, chapter_id, cleared, zet, exp, score,
	coop_class_point, vs_class_point, coop_class_id, vs_class_id, vs_plays, vs_wins`

// Results returns a player's results, oldest first.
func (s *Store) Results(playerID int64) ([]Result, error) {
	rows, err := s.db.Query(`SELECT `+resultColumns+` FROM results WHERE player_id = ? ORDER BY played_at, id`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var results []Result
	for rows.Next() {
		var result Result
		var played int64
		if err := rows.Scan(&played, &result.GameMode, &result.StageID, &result.ChapterID, &result.Cleared,
			&result.Zet, &result.Exp, &result.Score, &result.CoopClassPoint, &result.VSClassPoint,
			&result.CoopClassID, &result.VSClassID, &result.VSPlays, &result.VSWins); err != nil {
			return nil, err
		}
		result.Played = time.Unix(played, 0)
		results = append(results, result)
	}
	return results, rows.Err()
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
	return s.updatePlayer(`UPDATE players SET (`+settingsColumns+`) = (?, ?, ?, ?, ?, ?, ?) WHERE `+byCard,
		settings.MasterVolume, settings.BGMVolume, settings.MouseHorizontalReverse, settings.MouseVerticalReverse,
		settings.MouseSensitivity, settings.MouseAcceleration, settings.KeyMap, cardID)
}

// Rename changes the name of the player registered to a card.
func (s *Store) Rename(cardID, name string) error {
	return s.updatePlayer(`UPDATE players SET name = ? WHERE `+byCard, name, cardID)
}

// RecordLogin counts one more login for a player, on the service day that starts at day.
func (s *Store) RecordLogin(playerID int64, day time.Time) error {
	if err := s.updatePlayer(`UPDATE players SET logins = logins + 1 WHERE id = ?`, playerID); err != nil {
		return err
	}
	_, err := s.db.Exec(`INSERT INTO login_days (player_id, day) VALUES (?, ?) ON CONFLICT DO NOTHING`, playerID, day.Unix())
	return err
}

// LoginDays returns on how many service days a player logged in, counting the days that start
// from from until to.
func (s *Store) LoginDays(playerID int64, from, to time.Time) (int, error) {
	var days int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM login_days WHERE player_id = ? AND day >= ? AND day < ?`, playerID, from.Unix(), to.Unix()).Scan(&days)
	return days, err
}

// LastSeen returns the last time before before that a player logged in or a result of theirs
// arrived: the start of the service day for a login. It reports false for a player seen at
// neither before then, such as one whose logins came before the day log was kept.
func (s *Store) LastSeen(playerID int64, before time.Time) (time.Time, bool, error) {
	var last sql.NullInt64
	err := s.db.QueryRow(`SELECT MAX(seen) FROM (
			SELECT MAX(day) AS seen FROM login_days WHERE player_id = ? AND day < ?
			UNION ALL SELECT MAX(played_at) FROM results WHERE player_id = ? AND played_at < ?)`,
		playerID, before.Unix(), playerID, before.Unix()).Scan(&last)
	if err != nil || !last.Valid {
		return time.Time{}, false, err
	}
	return time.Unix(last.Int64, 0), true, nil
}

func (s *Store) updatePlayer(query string, args ...any) error {
	outcome, err := s.db.Exec(query, args...)
	if err != nil {
		return err
	}
	if rows, _ := outcome.RowsAffected(); rows == 0 {
		return ErrNoPlayer
	}
	return nil
}

// Scorer is a player and the score they made over a period.
type Scorer struct {
	Player Player
	Score  int64
}

// TopScorersSince returns up to limit players by the total score of their results played at or
// after since, highest first, leaving out versus results and players with no score. A nonzero
// homeStore keeps only the players of that home store.
func (s *Store) TopScorersSince(since time.Time, homeStore int64, limit int) ([]Scorer, error) {
	rows, err := s.db.Query(`SELECT `+playerColumns+`, total FROM players
		JOIN (SELECT player_id, SUM(score) AS total FROM results WHERE played_at >= ? AND vs_plays = 0 GROUP BY player_id)
		ON player_id = id
		WHERE total > 0 AND (? = 0 OR home_store = ?) ORDER BY total DESC, id LIMIT ?`, since.Unix(), homeStore, homeStore, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var scorers []Scorer
	for rows.Next() {
		var scorer Scorer
		if scorer.Player, err = scanPlayer(rows, &scorer.Score); err != nil {
			return nil, err
		}
		scorers = append(scorers, scorer)
	}
	return scorers, rows.Err()
}

// TopVersusPlayers returns up to limit players who have played versus, most versus class points
// first. A nonzero homeStore keeps only the players of that home store.
func (s *Store) TopVersusPlayers(homeStore int64, limit int) ([]Player, error) {
	return s.players(`vs_plays > 0 AND (? = 0 OR home_store = ?) ORDER BY vs_class_point DESC, id LIMIT ?`, homeStore, homeStore, limit)
}

// playerColumns lists the columns scanPlayer reads, in order.
const playerColumns = `id, name, zet, exp, score, logins, chara_id,
	coop_class_id, coop_class_point, vs_class_id, vs_class_point, vs_plays, vs_wins, home_store, ` + settingsColumns

// scanPlayer reads a row of playerColumns, followed by any extra columns into extra.
func scanPlayer(row interface{ Scan(...any) error }, extra ...any) (Player, error) {
	var player Player
	err := row.Scan(append([]any{&player.ID, &player.Name, &player.Zet, &player.Exp, &player.Score, &player.Logins, &player.CharaID,
		&player.CoopClassID, &player.CoopClassPoint, &player.VSClassID, &player.VSClassPoint, &player.VSPlays, &player.VSWins, &player.HomeStore,
		&player.Settings.MasterVolume, &player.Settings.BGMVolume, &player.Settings.MouseHorizontalReverse,
		&player.Settings.MouseVerticalReverse, &player.Settings.MouseSensitivity, &player.Settings.MouseAcceleration,
		&player.Settings.KeyMap}, extra...)...)
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
