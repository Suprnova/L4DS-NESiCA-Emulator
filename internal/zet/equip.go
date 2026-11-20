package zet

import (
	"fmt"
	"log/slog"
	"net/http"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// equipFailed is the equip reply that sets nothing: result 0, which the game takes as a failure.
const equipFailed = "1\n0\n0\n"

// equip puts item_num of the item item_id, of category item_cgr_id, in the player's slot_id, and
// answers with slot_set_num, the slot's new count, which the game shows as the slot's count
// (GameUI 0x10056d40 for a weapon, 0x10056e70 for a skill item). Only weapons and skill items
// are kept; other categories, a guest, a slot outside 1 to [store.Slots], and a count below 1
// get the failure reply.
func (s *Server) equip(w http.ResponseWriter, r *http.Request) {
	playerID, kind := formInt(r, "player_id"), store.ItemKind(formInt(r, "item_cgr_id"))
	if kind != store.Weapon && kind != store.SkillItem {
		slog.Info("stub reply", "path", r.URL.Path, "ignored", "equipping avatars, accessories, and titles", "category", kind)
		w.Write([]byte(equipFailed))
		return
	}
	item := store.Equipped{Kind: kind, Slot: formInt(r, "slot_id"), ItemID: formInt(r, "item_id"), Count: formInt(r, "item_num")}
	if guest(playerID) || item.Slot < 1 || item.Slot > store.Slots || item.Count < 1 {
		slog.Warn("equip refused", "player", playerID, "item", item)
		w.Write([]byte(equipFailed))
		return
	}

	count, err := s.store.Equip(playerID, item)
	if err != nil {
		slog.Error("equip", "player", playerID, "item", item, "error", err)
		w.Write([]byte(equipFailed))
		return
	}
	fmt.Fprintf(w, "%s%d\n", successHeader, count)
}
