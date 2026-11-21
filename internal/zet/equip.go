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

// equip fills the player's slot_id with the item item_id, of category item_cgr_id, from their
// owned items until it holds item_num in all (the game sends the item's count plus what the slot
// holds of it already, GameUI 0x10056b50), and answers with slot_set_num, the slot's new count, which the game
// shows as the slot's count (GameUI 0x10056d40 for a weapon, 0x10056e70 for a skill item). Only
// weapons and skill items are kept; other categories, a guest, a slot outside 1 to
// [store.Slots], a count below 1, and an item the player does not own get the failure reply.
func (s *Server) equip(w http.ResponseWriter, r *http.Request) {
	playerID, kind := formInt(r, "player_id"), store.ItemKind(formInt(r, "item_cgr_id"))
	if kind != store.Weapon && kind != store.SkillItem {
		slog.Info("stub reply", "path", r.URL.Path, "ignored", "equipping avatars, accessories, and titles", "category", kind)
		w.Write([]byte(equipFailed))
		return
	}
	slot, itemID, n := formInt(r, "slot_id"), formInt(r, "item_id"), formInt(r, "item_num")
	if guest(playerID) || slot < 1 || slot > store.Slots || n < 1 {
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
