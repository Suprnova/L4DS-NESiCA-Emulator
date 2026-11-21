package zet_test

import (
	"fmt"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

var (
	grantOpens  = time.Date(2026, 11, 1, 8, 0, 0, 0, time.UTC)
	grantCloses = time.Date(2026, 12, 1, 8, 0, 0, 0, time.UTC)
	starterPack = zet.Grant{
		ID: 7, When: calendar.Span{Start: grantOpens, End: grantCloses},
		Gift: zet.Gift{Gift: store.Gift{Name: "Starter Pack", Zet: 500, Item: store.Owned{Kind: store.Weapon, ItemID: 6, Name: "Rifle", Count: 5}}},
	}
)

func newGrantServer(t *testing.T, now time.Time, grants ...zet.Grant) (*zet.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	server := zet.NewServer(s, zet.Catalog{Schedule: zet.Schedule{Grants: grants}})
	server.SetClock(func() time.Time { return now })
	return server, s
}

// periodBonus returns the login reply's lines from the period_bonus count to the end, for a
// player without stage progress.
func periodBonus(body string) []string {
	return strings.Split(body, "\n")[3+5+25+4+1+3:]
}

func TestHandler_Login_OpenGrant_IsShownAndGiven(t *testing.T) {
	server, s := newGrantServer(t, grantOpens.Add(time.Hour), starterPack)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)

	got := periodBonus(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)))

	updated, _ := s.Player(player.ID)
	owned, _ := s.Owned(player.ID)
	want := []string{"1", "7", "Starter Pack", "500", "6", "1", "Rifle", "5", "6", "0", "0", "0", "4", "4", ""}
	if !slices.Equal(got, want) || updated.Zet != 500 || !slices.Equal(owned, []store.Owned{starterPack.Item}) {
		t.Fatalf("lines = %q, want %q; zet %d, owned %+v", got, want, updated.Zet, owned)
	}
}

func TestHandler_Login_GrantAlreadyGiven_IsNotShownAgain(t *testing.T) {
	server, s := newGrantServer(t, grantOpens.Add(time.Hour), starterPack)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID))

	got := periodBonus(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)))

	updated, _ := s.Player(player.ID)
	if got[0] != "0" || updated.Zet != 500 {
		t.Fatalf("lines = %q; zet %d", got, updated.Zet)
	}
}

func TestHandler_Login_RepeatingGrant_IsGivenAgainInItsNextRun(t *testing.T) {
	daily := zet.Grant{ID: 8, When: calendar.Span{Start: grantOpens, End: grantOpens.Add(24 * time.Hour), Every: calendar.Every{Days: 1}},
		Gift: zet.Gift{Gift: store.Gift{Name: "Daily", Zet: 100}}}
	server, s := newGrantServer(t, grantOpens.Add(time.Hour), daily)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID))
	postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID))
	server.SetClock(func() time.Time { return grantOpens.Add(25 * time.Hour) })

	got := periodBonus(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)))

	if updated, _ := s.Player(player.ID); got[0] != "1" || updated.Zet != 200 {
		t.Fatalf("lines = %q; zet %d", got, updated.Zet)
	}
}

func TestHandler_Login_ClosedGrant_IsNotGiven(t *testing.T) {
	for _, now := range []time.Time{grantOpens.Add(-time.Second), grantCloses} {
		server, s := newGrantServer(t, now, starterPack)
		player, _ := s.Register("7020392000000001", "Survivor", 0, 1)

		got := periodBonus(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)))

		if owned, _ := s.Owned(player.ID); got[0] != "0" || len(owned) != 0 {
			t.Fatalf("at %s: lines = %q; owned %+v", now, got, owned)
		}
	}
}

func TestHandler_Login_SVPGrant_IsShownButNotOwned(t *testing.T) {
	bonus := zet.Grant{ID: 1, When: calendar.Span{Start: grantOpens, End: grantCloses}, Gift: zet.Gift{Gift: store.Gift{Name: "Bonus SVP", Item: store.Owned{Kind: store.SVP, Count: 900}}}}
	server, s := newGrantServer(t, grantOpens, bonus)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)

	got := periodBonus(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)))

	want := []string{"1", "1", "Bonus SVP", "0", "0", "10", "", "900", "0", "0", "0", "0"}
	if owned, _ := s.Owned(player.ID); !slices.Equal(got[:12], want) || len(owned) != 0 {
		t.Fatalf("lines = %q, want %q; owned %+v", got, want, owned)
	}
}

func TestHandler_Login_QueuedGifts_AreSentOnceAsComps(t *testing.T) {
	server, s := newGrantServer(t, grantOpens)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	id, _ := s.Queue(player.ID, grantOpens, store.Gift{Name: "Apology", Zet: 1000, Item: store.Owned{Kind: store.SkillItem, ItemID: 29, Name: "Counter", Count: 3}})

	first := strings.Split(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)), "\n")
	again := strings.Split(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)), "\n")

	want := []string{"0", "0", "0", "0", "1", fmt.Sprint(id), "Apology", "1000", "29", "8", "Counter", "3", "0", "0", "0", "0", "5"}
	updated, _ := s.Player(player.ID)
	if !slices.Equal(first[3:20], want) || again[7] != "0" || again[8] != "5" || updated.Zet != 1000 {
		t.Fatalf("first login lines = %q, want %q; second %q; zet %d", first[3:20], want, again[3:9], updated.Zet)
	}
}

func avatarGrant(t *testing.T, name string, color int) zet.Grant {
	t.Helper()
	item, ok := game.Avatar(name)
	if !ok {
		t.Fatalf("no avatar item %s", name)
	}
	return zet.Grant{ID: 9, When: calendar.Span{Start: grantOpens, End: grantCloses}, Gift: zet.Gift{
		Gift:   store.Gift{Name: "Outfit", Item: store.Owned{Kind: zet.KindOf(item.Site), Name: "Coat"}},
		Avatar: &item, Color: color,
	}}
}

func TestHandler_Login_AvatarGrant_GivesTheSurvivorsOwnVariant(t *testing.T) {
	grant := avatarGrant(t, "firefighter_upper_avt", 2)
	server, s := newGrantServer(t, grantOpens, grant)
	player, _ := s.Register("7020392000000001", "Survivor", 2, 1)

	got := periodBonus(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)))

	resource := grant.Avatar.Resources[game.Sara]
	id := fmt.Sprint(resource*100 + 2)
	want := []string{"1", "9", "Outfit", "0", id, "2", "Coat", "1", fmt.Sprint(resource), "2", "0", "0"}
	owned, _ := s.Owned(player.ID)
	if !slices.Equal(got[:12], want) || len(owned) != 1 || fmt.Sprint(owned[0].ItemID) != id {
		t.Fatalf("lines = %q, want %q; owned %+v", got[:12], want, owned)
	}
}

func TestHandler_Login_AvatarGrantTheSurvivorCannotWear_GivesNoItem(t *testing.T) {
	grant := avatarGrant(t, "firefighter_upper_avt", 0)
	server, s := newGrantServer(t, grantOpens, grant)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)

	got := periodBonus(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)))

	if owned, _ := s.Owned(player.ID); got[0] != "1" || got[5] != "0" || len(owned) != 0 {
		t.Fatalf("lines = %q; owned %+v", got[:12], owned)
	}
}

func TestHandler_Login_Guest_GetsNoGrants(t *testing.T) {
	server, _ := newGrantServer(t, grantOpens, starterPack)

	if got := periodBonus(postTo(server, "/login", "player_id=-1")); got[0] != "0" {
		t.Fatalf("lines = %q", got)
	}
}
