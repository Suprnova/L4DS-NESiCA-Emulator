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

// ResultFilter picks results. Its zero value picks every result.
type ResultFilter struct {
	// Modes are the matching modes picked (game_mode); none picks every mode.
	Modes []int64
	// EventID is the event or versus unit picked; 0 picks results of any or none.
	EventID int64
	// StageID is the stage picked; 0 picks every stage.
	StageID int64
	// Cleared picks only results that cleared their chapter in a campaign play.
	Cleared bool
}

// where returns the conditions of the filter as SQL to follow a WHERE clause's other conditions,
// and their arguments.
func (f ResultFilter) where() (string, []any) {
	var clause strings.Builder
	var args []any
	if len(f.Modes) > 0 {
		clause.WriteString(` AND game_mode IN (?`)
		clause.WriteString(strings.Repeat(`, ?`, len(f.Modes)-1))
		clause.WriteString(`)`)
		for _, mode := range f.Modes {
			args = append(args, mode)
		}
	}
	if f.EventID != 0 {
		clause.WriteString(` AND event_id = ?`)
		args = append(args, f.EventID)
	}
	if f.StageID != 0 {
		clause.WriteString(` AND stage_id = ?`)
		args = append(args, f.StageID)
	}
	if f.Cleared {
		clause.WriteString(` AND cleared = 1`)
	}
	return clause.String(), args
}

// CountResults returns how many of a player's results arrived from from until to and pass filter.
func (s *Store) CountResults(playerID int64, from, to time.Time, filter ResultFilter) (int, error) {
	clause, args := filter.where()
	var n int
	err := s.db.QueryRow(`SELECT COUNT(*) FROM results WHERE player_id = ? AND played_at >= ? AND played_at < ?`+clause,
		append([]any{playerID, from.Unix(), to.Unix()}, args...)...).Scan(&n)
	return n, err
}

// ResultTimes returns the times, oldest first and each once, at which a player's results that
// pass filter arrived before until.
func (s *Store) ResultTimes(playerID int64, until time.Time, filter ResultFilter) ([]time.Time, error) {
	clause, args := filter.where()
	rows, err := s.db.Query(`SELECT DISTINCT played_at FROM results WHERE player_id = ? AND played_at < ?`+clause+` ORDER BY played_at`,
		append([]any{playerID, until.Unix()}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var times []time.Time
	for rows.Next() {
		var played int64
		if err := rows.Scan(&played); err != nil {
			return nil, err
		}
		times = append(times, time.Unix(played, 0))
	}
	return times, rows.Err()
}

// Total is a player's total score over some results.
type Total struct {
	PlayerID int64
	Score    int64
}

// TotalScores returns each player's total score over their results that arrived from from until
// to and pass filter, highest first; players with the same total are in player id order.
func (s *Store) TotalScores(from, to time.Time, filter ResultFilter) ([]Total, error) {
	clause, args := filter.where()
	rows, err := s.db.Query(`SELECT player_id, SUM(score) AS total FROM results WHERE played_at >= ? AND played_at < ?`+clause+`
		GROUP BY player_id ORDER BY total DESC, player_id`, append([]any{from.Unix(), to.Unix()}, args...)...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var totals []Total
	for rows.Next() {
		var total Total
		if err := rows.Scan(&total.PlayerID, &total.Score); err != nil {
			return nil, err
		}
		totals = append(totals, total)
	}
	return totals, rows.Err()
}
