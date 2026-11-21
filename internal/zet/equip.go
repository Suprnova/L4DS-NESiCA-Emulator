package zet

import (
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// equipFailed is the equip reply that sets nothing: result 0, which the game takes as a failure.
const equipFailed = "1\n0\n0\n"

// equipSlots are the slots each kind of item is set in, from the first to the last: weapons and
// skill items in slots 1 to [store.Slots], a title and a head item in 0, upper and lower clothing
// in the avatar site the game numbers them by (2 and 3), and accessories in 1 (face), 2 (upper
// body), or 3 (lower body), the site GameUI 0x100131b0 finds for the accessory.
var equipSlots = map[store.ItemKind][2]int64{
	store.Weapon: {1, store.Slots}, store.SkillItem: {1, store.Slots},
	store.Title: {0, 0}, store.Head: {0, 0}, store.Upper: {2, 2}, store.Lower: {3, 3}, store.Accessory: {1, 3},
}

// equip fills the player's slot_id with the item item_id, of category item_cgr_id, from their
// owned items until it holds item_num in all, and answers with slot_set_num, the slot's new
// count. For a weapon or skill item the game sends the item's count plus what the slot holds of
// it already (GameUI 0x10056b50), and shows slot_set_num as the slot's count (0x10056d40,
// 0x10056e70). A title or avatar item is set alone, and the game sends item_num 0 for it
// (0x10056a60, 0x10056ad0). A category not kept, a guest, a slot the kind is not set in, a
// weapon or skill item count below 1, and an item the player does not own get the failure reply.
func (s *Server) equip(w http.ResponseWriter, r *http.Request) {
	playerID, kind := formInt(r, "player_id"), store.ItemKind(formInt(r, "item_cgr_id"))
	slots, kept := equipSlots[kind]
	if !kept {
		slog.Info("stub reply", "path", r.URL.Path, "ignored", "equipping this category", "category", kind)
		w.Write([]byte(equipFailed))
		return
	}
	slot, itemID, n := formInt(r, "slot_id"), formInt(r, "item_id"), formInt(r, "item_num")
	if !kind.Stacks() {
		n = 1
	}
	if guest(playerID) || slot < slots[0] || slot > slots[1] || n < 1 {
		slog.Warn("equip refused", "player", playerID, "kind", kind, "slot", slot, "item", itemID, "count", n)
		w.Write([]byte(equipFailed))
		return
	}

	count, err := s.store.Equip(playerID, kind, slot, itemID, n)
	switch {
	case errors.Is(err, store.ErrNotOwned):
		slog.Warn("equip refused: item not owned", "player", playerID, "kind", kind, "item", itemID)
		w.Write([]byte(equipFailed))
	case err != nil:
		slog.Error("equip", "player", playerID, "kind", kind, "slot", slot, "item", itemID, "error", err)
		w.Write([]byte(equipFailed))
	default:
		fmt.Fprintf(w, "%s%d\n", successHeader, count)
	}
}

// fluctuationFields are the result fields that report how many of each kind of item the chapter
// used up.
var fluctuationFields = map[store.ItemKind]string{
	store.Weapon:    "weapon_fluctuation",
	store.SkillItem: "disposable_fluctuation",
}

// countChanges reads a result's fluctuation field, "<slot>=<change>" pairs joined by commas such
// as "1=-1,3=-2", into the change per slot. Malformed pairs are skipped.
func countChanges(field string) map[int64]int64 {
	changes := map[int64]int64{}
	for pair := range strings.SplitSeq(field, ",") {
		slot, change, found := strings.Cut(pair, "=")
		s, errSlot := strconv.ParseInt(slot, 10, 64)
		c, errChange := strconv.ParseInt(change, 10, 64)
		if found && errSlot == nil && errChange == nil {
			changes[s] += c
		}
	}
	return changes
}

// useItems takes the items a result reports used up off the player's slots.
func (s *Server) useItems(playerID int64, r *http.Request) {
	for kind, field := range fluctuationFields {
		changes := countChanges(r.PostFormValue(field))
		if len(changes) == 0 {
			continue
		}
		if err := s.store.ChangeCounts(playerID, kind, changes); err != nil {
			slog.Error("use items", "player", playerID, "field", field, "error", err)
		}
	}
}
