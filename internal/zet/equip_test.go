package zet_test

import (
	"fmt"
	"slices"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

func TestHandler_Equip_SetsTheItemAndReportsTheSlotsCount(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)

	got := postTo(server, "/equip", fmt.Sprintf("player_id=%d&item_cgr_id=1&item_id=6&slot_id=1&item_num=5", player.ID))

	equipment, _ := s.Equipment(player.ID)
	want := []store.Equipped{{Kind: store.Weapon, Slot: 1, ItemID: 6, Count: 5}}
	if got != "1\n1\n0\n5\n" || !slices.Equal(equipment, want) {
		t.Fatalf("body = %q, equipment = %+v", got, equipment)
	}
}

func TestHandler_Equip_SameItemAgain_ReportsTheStack(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	postTo(server, "/equip", fmt.Sprintf("player_id=%d&item_cgr_id=8&item_id=30&slot_id=2&item_num=3", player.ID))

	got := postTo(server, "/equip", fmt.Sprintf("player_id=%d&item_cgr_id=8&item_id=30&slot_id=2&item_num=2", player.ID))

	if got != "1\n1\n0\n5\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Equip_UnkeptOrBadRequests_Fail(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	bodies := []string{
		fmt.Sprintf("player_id=%d&item_cgr_id=4&item_id=1&slot_id=1&item_num=1", player.ID),
		fmt.Sprintf("player_id=%d&item_cgr_id=1&item_id=6&slot_id=5&item_num=1", player.ID),
		fmt.Sprintf("player_id=%d&item_cgr_id=1&item_id=6&slot_id=1&item_num=0", player.ID),
		"player_id=-1&item_cgr_id=1&item_id=6&slot_id=1&item_num=1",
	}

	for _, body := range bodies {
		if got := postTo(server, "/equip", body); got != "1\n0\n0\n" {
			t.Errorf("%s: body = %q", body, got)
		}
	}
	if equipment, _ := s.Equipment(player.ID); len(equipment) != 0 {
		t.Fatalf("equipment = %+v, want none", equipment)
	}
}
