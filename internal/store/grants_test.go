package store_test

import (
	"slices"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

func TestDeliver_GivesTheZetAndTheItem(t *testing.T) {
	s := openStore(t)
	player := register(t, s)

	ok, err := s.Deliver(player.ID, 1, delivered, store.Gift{Name: "Welcome", Zet: 500, Item: rifle})

	got, _ := s.Owned(player.ID)
	updated, _ := s.Player(player.ID)
	if !ok || err != nil || updated.Zet != 500 || !slices.Equal(got, []store.Owned{rifle}) {
		t.Fatalf("Deliver() = %v, %v; zet %d, owned %+v", ok, err, updated.Zet, got)
	}
}

func TestDeliver_SameGrantAgain_GivesNothing(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.Deliver(player.ID, 1, delivered, store.Gift{Name: "Welcome", Zet: 500, Item: rifle})

	ok, err := s.Deliver(player.ID, 1, delivered, store.Gift{Name: "Welcome", Zet: 500, Item: rifle})

	got, _ := s.Owned(player.ID)
	updated, _ := s.Player(player.ID)
	if ok || err != nil || updated.Zet != 500 || !slices.Equal(got, []store.Owned{rifle}) {
		t.Fatalf("Deliver() = %v, %v; zet %d, owned %+v", ok, err, updated.Zet, got)
	}
}

func TestDeliver_AnotherGrantOfTheSameItem_AddsToTheStack(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	s.Deliver(player.ID, 1, delivered, store.Gift{Name: "A", Item: rifle})

	s.Deliver(player.ID, 2, delivered, store.Gift{Name: "B", Item: rifle})

	want := rifle
	want.Count = 10
	if got, _ := s.Owned(player.ID); !slices.Equal(got, []store.Owned{want}) {
		t.Fatalf("Owned() = %+v, want %+v", got, want)
	}
}

func TestDeliver_SVP_IsNotOwned(t *testing.T) {
	s := openStore(t)
	player := register(t, s)

	s.Deliver(player.ID, 1, delivered, store.Gift{Name: "Bonus", Item: store.Owned{Kind: store.SVP, Count: 900}})

	if got, _ := s.Owned(player.ID); len(got) != 0 {
		t.Fatalf("Owned() = %+v, want none", got)
	}
}

func TestDeliverQueued_GivesTheQueuedGiftsOnceInOrder(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	apology := store.Gift{Name: "Apology", Zet: 1000}
	backfill := store.Gift{Name: "Backfill", Item: hunterCounter}
	first, _ := s.Queue(player.ID, delivered, apology)
	second, _ := s.Queue(player.ID, delivered, backfill)

	got, err := s.DeliverQueued(player.ID, delivered)
	again, _ := s.DeliverQueued(player.ID, delivered)

	updated, _ := s.Player(player.ID)
	owned, _ := s.Owned(player.ID)
	want := []store.QueuedGift{{ID: first, Gift: apology}, {ID: second, Gift: backfill}}
	if err != nil || !slices.Equal(got, want) || len(again) != 0 || updated.Zet != 1000 || !slices.Equal(owned, []store.Owned{hunterCounter}) {
		t.Fatalf("DeliverQueued() = %+v, %v, then %+v; zet %d, owned %+v", got, err, again, updated.Zet, owned)
	}
}

func TestDeliverQueued_OnlyGivesThePlayersOwnGifts(t *testing.T) {
	s := openStore(t)
	player := register(t, s)
	other, _ := s.Register("7020392000000002", "Other", 0, 1)
	s.Queue(other.ID, delivered, store.Gift{Name: "Apology", Zet: 1000})

	got, err := s.DeliverQueued(player.ID, delivered)

	if err != nil || len(got) != 0 {
		t.Fatalf("DeliverQueued() = %+v, %v, want none", got, err)
	}
}

func TestQueue_UnknownPlayer_ReturnsAnError(t *testing.T) {
	s := openStore(t)

	if _, err := s.Queue(99, delivered, store.Gift{Name: "Apology", Zet: 1000}); err == nil {
		t.Fatal("Queue() error = nil")
	}
}
