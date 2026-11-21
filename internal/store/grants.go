package store

import (
	"database/sql"
	"time"
)

// Gift is what a grant gives: ZG, an item, or both, under a name.
type Gift struct {
	Name string
	// Zet is the ZG given, added to the player's ZG.
	Zet int64
	// Item is the item given, or none when its Kind is 0. A weapon or skill item becomes the
	// player's own; SVP is credited by the game itself, with Count the seconds, and is not kept.
	Item Owned
}

// QueuedGift is a gift queued for one player, delivered at their next login.
type QueuedGift struct {
	// ID is the store's number for the gift, from 1.
	ID int64
	Gift
}

// Deliver gives a player the gift of the scheduled grant grantID, recorded as delivered at the
// given time. A grant is delivered once: when it was delivered to the player before, Deliver
// gives nothing and reports false.
func (s *Store) Deliver(playerID, grantID int64, at time.Time, gift Gift) (bool, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return false, err
	}
	defer tx.Rollback()

	outcome, err := tx.Exec(`INSERT INTO grant_deliveries (player_id, grant_id, delivered_at) VALUES (?, ?, ?) ON CONFLICT DO NOTHING`, playerID, grantID, at.Unix())
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

// Queue queues a gift for a player, to be delivered at their next login, and returns its id.
func (s *Store) Queue(playerID int64, at time.Time, gift Gift) (int64, error) {
	outcome, err := s.db.Exec(`INSERT INTO queued_grants (player_id, name, zet, kind, item_id, item_name, item_count, queued_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?)`,
		playerID, gift.Name, gift.Zet, gift.Item.Kind, gift.Item.ItemID, gift.Item.Name, gift.Item.Count, at.Unix())
	if err != nil {
		return 0, err
	}
	return outcome.LastInsertId()
}

// DeliverQueued gives a player the gifts queued for them and not yet delivered, in the order they
// were queued, marks them delivered at the given time, and returns them.
func (s *Store) DeliverQueued(playerID int64, at time.Time) ([]QueuedGift, error) {
	tx, err := s.db.Begin()
	if err != nil {
		return nil, err
	}
	defer tx.Rollback()

	rows, err := tx.Query(`SELECT id, name, zet, kind, item_id, item_name, item_count FROM queued_grants WHERE player_id = ? AND delivered_at IS NULL ORDER BY id`, playerID)
	if err != nil {
		return nil, err
	}
	var gifts []QueuedGift
	for rows.Next() {
		var g QueuedGift
		if err := rows.Scan(&g.ID, &g.Name, &g.Zet, &g.Item.Kind, &g.Item.ItemID, &g.Item.Name, &g.Item.Count); err != nil {
			rows.Close()
			return nil, err
		}
		gifts = append(gifts, g)
	}
	rows.Close()
	if err := rows.Err(); err != nil {
		return nil, err
	}

	for _, g := range gifts {
		if err := give(tx, playerID, g.Gift); err != nil {
			return nil, err
		}
		if _, err := tx.Exec(`UPDATE queued_grants SET delivered_at = ? WHERE id = ?`, at.Unix(), g.ID); err != nil {
			return nil, err
		}
	}
	return gifts, tx.Commit()
}

// give adds a gift's ZG to the player's, and its weapon or skill item to their owned items.
func give(tx *sql.Tx, playerID int64, gift Gift) error {
	if _, err := tx.Exec(`UPDATE players SET zet = zet + ? WHERE id = ?`, gift.Zet, playerID); err != nil {
		return err
	}
	if gift.Item.Kind != Weapon && gift.Item.Kind != SkillItem {
		return nil
	}
	return addOwned(tx, playerID, gift.Item)
}
