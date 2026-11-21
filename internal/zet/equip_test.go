package zet_test

import (
	"fmt"
	"slices"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

var lastGrant int64

// give gives a player items to own, each as a grant of its own.
func give(t *testing.T, s *store.Store, playerID int64, items ...store.Owned) {
	t.Helper()
	for _, item := range items {
		lastGrant++
		if _, err := s.Deliver(playerID, lastGrant, time.Now(), store.Gift{Name: "Gift", Item: item}); err != nil {
			t.Fatal(err)
		}
	}
}

func TestHandler_Equip_MovesTheOwnedItemIntoTheSlot(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	give(t, s, player.ID, store.Owned{Kind: store.Weapon, ItemID: 6, Name: "Rifle", Count: 5})

	got := postTo(server, "/equip", fmt.Sprintf("player_id=%d&item_cgr_id=1&item_id=6&slot_id=1&item_num=5", player.ID))

	equipment, _ := s.Equipment(player.ID)
	want := []store.Equipped{{Kind: store.Weapon, Slot: 1, ItemID: 6, Name: "Rifle", Count: 5}}
	if got != "1\n1\n0\n5\n" || !slices.Equal(equipment, want) {
		t.Fatalf("body = %q, equipment = %+v", got, equipment)
	}
}

func TestHandler_Equip_SameItemAgain_FillsTheSlotToTheCount(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	give(t, s, player.ID, store.Owned{Kind: store.SkillItem, ItemID: 30, Count: 5})
	postTo(server, "/equip", fmt.Sprintf("player_id=%d&item_cgr_id=8&item_id=30&slot_id=2&item_num=3", player.ID))

	got := postTo(server, "/equip", fmt.Sprintf("player_id=%d&item_cgr_id=8&item_id=30&slot_id=2&item_num=5", player.ID))

	if got != "1\n1\n0\n5\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Equip_UnkeptOrBadRequests_Fail(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	give(t, s, player.ID, store.Owned{Kind: store.Weapon, ItemID: 6, Count: 5})
	bodies := []string{
		fmt.Sprintf("player_id=%d&item_cgr_id=4&item_id=1&slot_id=1&item_num=1", player.ID),
		fmt.Sprintf("player_id=%d&item_cgr_id=1&item_id=6&slot_id=5&item_num=1", player.ID),
		fmt.Sprintf("player_id=%d&item_cgr_id=1&item_id=6&slot_id=1&item_num=0", player.ID),
		fmt.Sprintf("player_id=%d&item_cgr_id=1&item_id=7&slot_id=1&item_num=1", player.ID),
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

func TestHandler_Result_TakesUsedItemsOffTheSlots(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	give(t, s, player.ID, store.Owned{Kind: store.Weapon, ItemID: 6, Count: 1}, store.Owned{Kind: store.SkillItem, ItemID: 30, Count: 3}, store.Owned{Kind: store.SkillItem, ItemID: 8, Count: 4})
	s.Equip(player.ID, store.Weapon, 1, 6, 1)
	s.Equip(player.ID, store.SkillItem, 1, 30, 3)
	s.Equip(player.ID, store.SkillItem, 3, 8, 4)

	postTo(server, "/result", fmt.Sprintf("player_id=%d&weapon_fluctuation=1%%3D-1&disposable_fluctuation=1%%3D-1%%2C3%%3D-2", player.ID))

	got, _ := s.Equipment(player.ID)
	want := []store.Equipped{
		{Kind: store.SkillItem, Slot: 1, ItemID: 30, Count: 2},
		{Kind: store.SkillItem, Slot: 3, ItemID: 8, Count: 2},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("equipment = %+v, want %+v", got, want)
	}
}
