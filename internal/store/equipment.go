package store

import (
	"database/sql"
	"errors"
	"fmt"
)

// ItemKind is the kind of an item, numbered as the game's item categories (item_cgr_id).
type ItemKind int64

const (
	// Weapon is a carried weapon (category 1).
	Weapon ItemKind = 1
	// SkillItem is a skill item (category 8).
	SkillItem ItemKind = 8
	// SVP is play time (category 10). The game credits it itself, so it is never owned.
	SVP ItemKind = 10
)

// Slots is how many slots a player has of each kind.
const Slots = 4

// ErrNotOwned is returned by [Store.Equip] for an item the player does not own.
var ErrNotOwned = errors.New("store: item not owned")

// Owned is a stack of items a player owns outside the slots: given to them and not yet set, or
// taken out of a slot.
type Owned struct {
	Kind ItemKind
	// ItemID is the item: a weapon's or skill item's database id in the game's catalogue. A
	// weapon's id is also its resource id.
	ItemID int64
	// Name is the name the card shows once the item is set; empty when unknown.
	Name  string
	Count int64
}

// Equipped is a stack of items set in one of a player's slots.
type Equipped struct {
	Kind ItemKind
	// Slot is the slot, 1 to [Slots].
	Slot int64
	// ItemID is the item, as in [Owned].
	ItemID int64
	// Name is the name the card shows; empty when unknown.
	Name string
	// Count is how many are left.
	Count int64
}

// Equipment returns the items set in a player's slots, weapons first, by slot.
func (s *Store) Equipment(playerID int64) ([]Equipped, error) {
	rows, err := s.db.Query(`SELECT kind, slot, item_id, name, count FROM equipment WHERE player_id = ? ORDER BY kind, slot`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var equipment []Equipped
	for rows.Next() {
		var item Equipped
		if err := rows.Scan(&item.Kind, &item.Slot, &item.ItemID, &item.Name, &item.Count); err != nil {
			return nil, err
		}
		equipment = append(equipment, item)
	}
	return equipment, rows.Err()
}

// Owned returns the items a player owns outside the slots, by kind and item.
func (s *Store) Owned(playerID int64) ([]Owned, error) {
	rows, err := s.db.Query(`SELECT kind, item_id, name, count FROM owned_items WHERE player_id = ? ORDER BY kind, item_id`, playerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var owned []Owned
	for rows.Next() {
		var item Owned
		if err := rows.Scan(&item.Kind, &item.ItemID, &item.Name, &item.Count); err != nil {
			return nil, err
		}
		owned = append(owned, item)
	}
	return owned, rows.Err()
}

// Equip sets an owned item into a slot to hold count of it in all, as the game's equip request
// asks, and returns how many the slot then holds. What the slot holds of the item already counts
// towards count, and the rest comes from the player's owned items, as far as they go. When the
// slot holds another item, that one goes back to the owned items. It returns [ErrNotOwned] when
// the player owns none of the item outside the slots. count must be at least 1.
func (s *Store) Equip(playerID int64, kind ItemKind, slot, itemID, count int64) (int64, error) {
	if count < 1 {
		return 0, fmt.Errorf("store: cannot equip %d items", count)
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()

	var current Equipped
	err = tx.QueryRow(`SELECT item_id, name, count FROM equipment WHERE player_id = ? AND kind = ? AND slot = ?`, playerID, kind, slot).Scan(&current.ItemID, &current.Name, &current.Count)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	held := int64(0)
	if current.ItemID == itemID {
		held = current.Count
	}

	item := Owned{Kind: kind, ItemID: itemID}
	err = tx.QueryRow(`SELECT name, count FROM owned_items WHERE player_id = ? AND kind = ? AND item_id = ?`, playerID, kind, itemID).Scan(&item.Name, &item.Count)
	switch {
	case errors.Is(err, sql.ErrNoRows) && held > 0:
		return held, nil
	case errors.Is(err, sql.ErrNoRows):
		return 0, ErrNotOwned
	case err != nil:
		return 0, err
	}
	moved := min(item.Count, max(count-held, 0))
	if err := addOwned(tx, playerID, Owned{Kind: kind, ItemID: itemID, Count: -moved}); err != nil {
		return 0, err
	}
	item.Count = held + moved
	if held > 0 {
		item.Name = current.Name
	} else if current.Count > 0 {
		if err := addOwned(tx, playerID, Owned{Kind: kind, ItemID: current.ItemID, Name: current.Name, Count: current.Count}); err != nil {
			return 0, err
		}
	}
	_, err = tx.Exec(`INSERT INTO equipment (player_id, kind, slot, item_id, name, count) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (player_id, kind, slot) DO UPDATE SET item_id = excluded.item_id, name = excluded.name, count = excluded.count`,
		playerID, kind, slot, itemID, item.Name, item.Count)
	if err != nil {
		return 0, err
	}
	return item.Count, tx.Commit()
}

// addOwned adds item's count, which may be negative, to the player's stack of the item, and
// removes a stack left with none. A new stack takes item's name.
func addOwned(tx *sql.Tx, playerID int64, item Owned) error {
	_, err := tx.Exec(`INSERT INTO owned_items (player_id, kind, item_id, name, count) VALUES (?, ?, ?, ?, ?)
		ON CONFLICT (player_id, kind, item_id) DO UPDATE SET count = count + excluded.count`,
		playerID, item.Kind, item.ItemID, item.Name, item.Count)
	if err != nil {
		return err
	}
	_, err = tx.Exec(`DELETE FROM owned_items WHERE player_id = ? AND count <= 0`, playerID)
	return err
}

// ChangeCounts adds changes[slot] to the count of each of a player's slots of kind, and empties
// the slots left with none. Slots with nothing set are ignored.
func (s *Store) ChangeCounts(playerID int64, kind ItemKind, changes map[int64]int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	for slot, change := range changes {
		if _, err := tx.Exec(`UPDATE equipment SET count = count + ? WHERE player_id = ? AND kind = ? AND slot = ?`, change, playerID, kind, slot); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM equipment WHERE player_id = ? AND count <= 0`, playerID); err != nil {
		return err
	}
	return tx.Commit()
}
