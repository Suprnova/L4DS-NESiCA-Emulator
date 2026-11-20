package store

// ItemKind is the kind of item a slot holds, numbered as the game's item categories
// (item_cgr_id).
type ItemKind int64

const (
	// Weapon is a carried weapon (category 1).
	Weapon ItemKind = 1
	// SkillItem is a skill item (category 8).
	SkillItem ItemKind = 8
)

// Slots is how many slots a player has of each kind.
const Slots = 4

// Equipped is a stack of items set in one of a player's slots.
type Equipped struct {
	Kind ItemKind
	// Slot is the slot, 1 to [Slots].
	Slot int64
	// ItemID is the item: a weapon's or skill item's database id in the game's catalogue. A
	// weapon's id is also its resource id.
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

// Equip puts item in its slot and returns how many the slot then holds. When the slot already
// holds the same item, item's count is added to it and the slot keeps its name; otherwise item
// replaces what was there.
func (s *Store) Equip(playerID int64, item Equipped) (int64, error) {
	var count int64
	err := s.db.QueryRow(`INSERT INTO equipment (player_id, kind, slot, item_id, name, count) VALUES (?, ?, ?, ?, ?, ?)
		ON CONFLICT (player_id, kind, slot) DO UPDATE SET
			count = CASE WHEN item_id = excluded.item_id THEN count + excluded.count ELSE excluded.count END,
			name = CASE WHEN item_id = excluded.item_id THEN name ELSE excluded.name END,
			item_id = excluded.item_id
		RETURNING count`,
		playerID, item.Kind, item.Slot, item.ItemID, item.Name, item.Count).Scan(&count)
	return count, err
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
