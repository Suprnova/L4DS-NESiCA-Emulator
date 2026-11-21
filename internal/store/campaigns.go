package store

import (
	"strings"
	"time"
)

// Step names one gift of a campaign: the campaign by its kind and id, which run of it, and which
// step within the run.
type Step struct {
	// Kind is the campaign kind, such as "daily"; each kind has its own ids.
	Kind string
	ID   int64
	Run  int
	Step int
}

// DeliverStep gives a player the gift of a campaign step, recorded as delivered at the given time.
// Each step is delivered once: when it was delivered to the player before, DeliverStep gives
// nothing and reports false.
func (s *Store) DeliverStep(playerID int64, step Step, at time.Time, gift Gift) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	outcome, err := tx.Exec(`INSERT INTO campaign_deliveries (player_id, kind, campaign_id, run, step, delivered_at) VALUES (?, ?, ?, ?, ?, ?) ON CONFLICT DO NOTHING`,
		playerID, step.Kind, step.ID, step.Run, step.Step, at.Unix())
	if err != nil {
		return false, err
	}
	if n, err := outcome.RowsAffected(); err != nil || n == 0 {
		return false, err
	}
	if err := give(tx, playerID, gift); err != nil {
		return false, err
	}
	return true, tx.Commit()
}

// ResultFilter picks results to count. Its zero value counts every result.
type ResultFilter struct {
	// Modes are the matching modes counted (game_mode); none counts every mode.
	Modes []int64
	// StageID is the stage counted; 0 counts every stage.
	StageID int64
	// Cleared counts only results that cleared their chapter in a campaign play.
	Cleared bool
}

// CountResults returns how many of a player's results arrived from from until to and pass filter.
func (s *Store) CountResults(playerID int64, from, to time.Time, filter ResultFilter) (int, error) {
	query := `SELECT COUNT(*) FROM results WHERE player_id = ? AND played_at >= ? AND played_at < ?`
	args := []any{playerID, from.Unix(), to.Unix()}
	if len(filter.Modes) > 0 {
		query += ` AND game_mode IN (?` + strings.Repeat(`, ?`, len(filter.Modes)-1) + `)`
		for _, mode := range filter.Modes {
			args = append(args, mode)
		}
	}
	if filter.StageID != 0 {
		query += ` AND stage_id = ?`
		args = append(args, filter.StageID)
	}
	if filter.Cleared {
		query += ` AND cleared = 1`
	}
	var n int
	err := s.db.QueryRow(query, args...).Scan(&n)
	return n, err
}
