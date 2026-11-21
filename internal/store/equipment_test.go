package store_test

import (
	"errors"
	"slices"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

var lastGrant int64

// own gives a player items to own, each as a scheduled grant of its own.
func own(t *testing.T, s *store.Store, playerID int64, items ...store.Owned) {
	t.Helper()
	for _, item := range items {
		lastGrant++
		if _, err := s.Deliver(playerID, lastGrant, delivered, store.Gift{Name: "Gift", Item: item}); err != nil {
			t.Fatal(err)
		}
	}
}

var (
	hunterCounter = store.Owned{Kind: store.SkillItem, ItemID: 30, Name: "ハンター返し", Count: 3}
	rifle         = store.Owned{Kind: store.Weapon, ItemID: 6, Name: "AK-47", Count: 5}
	delivered     = time.Date(2026, 11, 20, 12, 0, 0, 0, time.UTC)
)

func TestEquip_OwnedItem_MovesIntoTheSlotWithItsName(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	own(t, s, player.ID, rifle)

	count, err := s.Equip(player.ID, store.Weapon, 1, 6, 5)

	equipment, _ := s.Equipment(player.ID)
	owned, _ := s.Owned(player.ID)
	want := []store.Equipped{{Kind: store.Weapon, Slot: 1, ItemID: 6, Name: "AK-47", Count: 5}}
	if err != nil || count != 5 || !slices.Equal(equipment, want) || len(owned) != 0 {
		t.Fatalf("Equip() = %d, %v; equipment %+v, owned %+v", count, err, equipment, owned)
	}
}

func TestEquip_MoreThanOwned_MovesWhatIsOwned(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	own(t, s, player.ID, hunterCounter)

	count, err := s.Equip(player.ID, store.SkillItem, 2, 30, 10)

	if err != nil || count != 3 {
		t.Fatalf("Equip() = %d, %v, want 3", count, err)
	}
}

func TestEquip_PartOfTheStack_LeavesTheRestOwned(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	own(t, s, player.ID, rifle)

	s.Equip(player.ID, store.Weapon, 1, 6, 2)

	want := rifle
	want.Count = 3
	if got, _ := s.Owned(player.ID); !slices.Equal(got, []store.Owned{want}) {
		t.Fatalf("Owned() = %+v, want %+v", got, want)
	}
}

func TestEquip_SameItemAgain_AddsToTheSlot(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	own(t, s, player.ID, rifle)
	s.Equip(player.ID, store.Weapon, 1, 6, 2)

	count, err := s.Equip(player.ID, store.Weapon, 1, 6, 3)

	if err != nil || count != 5 {
		t.Fatalf("Equip() = %d, %v, want 5", count, err)
	}
}

func TestEquip_OtherItem_ReturnsTheSlotsItemToTheOwned(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	shotgun := store.Owned{Kind: store.Weapon, ItemID: 1, Name: "ポンプ式ショットガン", Count: 2}
	own(t, s, player.ID, rifle, shotgun)
	s.Equip(player.ID, store.Weapon, 1, 6, 5)

	s.Equip(player.ID, store.Weapon, 1, 1, 2)

	equipment, _ := s.Equipment(player.ID)
	owned, _ := s.Owned(player.ID)
	want := []store.Equipped{{Kind: store.Weapon, Slot: 1, ItemID: 1, Name: shotgun.Name, Count: 2}}
	if !slices.Equal(equipment, want) || !slices.Equal(owned, []store.Owned{rifle}) {
		t.Fatalf("equipment %+v, owned %+v", equipment, owned)
	}
}

func TestEquip_ItemNotOwned_ReturnsErrNotOwned(t *testing.T) {
	s := openStore(t)
	player := register(t, s)

	_, err := s.Equip(player.ID, store.Weapon, 1, 6, 1)

	if equipment, _ := s.Equipment(player.ID); !errors.Is(err, store.ErrNotOwned) || len(equipment) != 0 {
		t.Fatalf("Equip() error = %v; equipment %+v", err, equipment)
	}
}

func TestChangeCounts_TakesOffTheCountAndEmptiesSlotsThatRunOut(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	own(t, s, player.ID, hunterCounter, store.Owned{Kind: store.Weapon, ItemID: 6, Count: 1})
	s.Equip(player.ID, store.SkillItem, 1, 30, 3)
	s.Equip(player.ID, store.Weapon, 2, 6, 1)

	errSkill := s.ChangeCounts(player.ID, store.SkillItem, map[int64]int64{1: -1})
	errWeapon := s.ChangeCounts(player.ID, store.Weapon, map[int64]int64{2: -1})

	got, _ := s.Equipment(player.ID)
	want := []store.Equipped{{Kind: store.SkillItem, Slot: 1, ItemID: 30, Name: hunterCounter.Name, Count: 2}}
	if errSkill != nil || errWeapon != nil || !slices.Equal(got, want) {
		t.Fatalf("Equipment() = %+v (%v, %v), want %+v", got, errSkill, errWeapon, want)
	}
}

func TestChangeCounts_OnlyTouchesTheGivenKind(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	own(t, s, player.ID, hunterCounter)
	s.Equip(player.ID, store.SkillItem, 1, 30, 3)

	s.ChangeCounts(player.ID, store.Weapon, map[int64]int64{1: -3})

	if got, _ := s.Equipment(player.ID); len(got) != 1 || got[0].Count != 3 {
		t.Fatalf("Equipment() = %+v, want the skill item untouched", got)
	}
}
