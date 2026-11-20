package store_test

import (
	"slices"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

var hunterCounter = store.Equipped{Kind: store.SkillItem, Slot: 1, ItemID: 30, Name: "ハンター返し", Count: 3}

func TestEquip_EmptySlot_SetsTheItem(t *testing.T) {
	s := openStore(t)
	player := register(t, s)

	count, err := s.Equip(player.ID, hunterCounter)

	got, _ := s.Equipment(player.ID)
	if err != nil || count != 3 || !slices.Equal(got, []store.Equipped{hunterCounter}) {
		t.Fatalf("Equip() = %d, %v; Equipment() = %+v", count, err, got)
	}
}

func TestEquip_SameItem_AddsToTheStack(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.Equip(player.ID, hunterCounter)
	more := hunterCounter
	more.Name, more.Count = "", 2

	count, err := s.Equip(player.ID, more)

	got, _ := s.Equipment(player.ID)
	want := hunterCounter
	want.Count = 5
	if err != nil || count != 5 || !slices.Equal(got, []store.Equipped{want}) {
		t.Fatalf("Equip() = %d, %v; Equipment() = %+v", count, err, got)
	}
}

func TestEquip_OtherItem_ReplacesTheSlot(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.Equip(player.ID, hunterCounter)
	other := store.Equipped{Kind: store.SkillItem, Slot: 1, ItemID: 8, Count: 1}

	count, err := s.Equip(player.ID, other)

	got, _ := s.Equipment(player.ID)
	if err != nil || count != 1 || !slices.Equal(got, []store.Equipped{other}) {
		t.Fatalf("Equip() = %d, %v; Equipment() = %+v", count, err, got)
	}
}

func TestChangeCounts_TakesOffTheCountAndEmptiesSlotsThatRunOut(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.Equip(player.ID, hunterCounter)
	ak := store.Equipped{Kind: store.Weapon, Slot: 2, ItemID: 6, Name: "AK-47", Count: 1}
	s.Equip(player.ID, ak)

	errSkill := s.ChangeCounts(player.ID, store.SkillItem, map[int64]int64{1: -1})
	errWeapon := s.ChangeCounts(player.ID, store.Weapon, map[int64]int64{2: -1})

	got, _ := s.Equipment(player.ID)
	want := hunterCounter
	want.Count = 2
	if errSkill != nil || errWeapon != nil || !slices.Equal(got, []store.Equipped{want}) {
		t.Fatalf("Equipment() = %+v (%v, %v), want %+v", got, errSkill, errWeapon, want)
	}
}

func TestChangeCounts_OnlyTouchesTheGivenKind(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.Equip(player.ID, hunterCounter)

	s.ChangeCounts(player.ID, store.Weapon, map[int64]int64{1: -3})

	if got, _ := s.Equipment(player.ID); !slices.Equal(got, []store.Equipped{hunterCounter}) {
		t.Fatalf("Equipment() = %+v, want %+v", got, hunterCounter)
	}
}
