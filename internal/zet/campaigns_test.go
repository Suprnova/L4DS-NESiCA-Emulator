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

// spring is a campaign span: 2027-04-01 08:00 for thirty days.
var spring = calendar.Span{Start: on(4, 1, 8), End: on(5, 1, 8)}

func bonusSVP(svp int64) zet.Gift {
	return zet.Gift{Gift: store.Gift{Name: "Bonus", Item: store.Owned{Kind: store.SVP, Count: svp}}}
}

func zg(name string, amount int64) zet.Gift {
	return zet.Gift{Gift: store.Gift{Name: name, Zet: amount}}
}

// campaignServer is a server with schedule whose clock reads the time clock points at.
func campaignServer(t *testing.T, clock *time.Time, schedule zet.Schedule) (*zet.Server, *store.Store, int64) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	server := zet.NewServer(s, zet.Catalog{Levels: game.Levels, Schedule: schedule})
	server.SetClock(func() time.Time { return *clock })
	player, _ := s.Register("7020392000000001", "Survivor", 2, 1)
	return server, s, player.ID
}

func login(server *zet.Server, playerID int64) []string {
	return strings.Split(postTo(server, "/login", fmt.Sprintf("player_id=%d", playerID)), "\n")
}

func TestHandler_Login_DailyGift_IsGivenOnceAServiceDay(t *testing.T) {
	now := on(4, 2, 12)
	server, _, player := campaignServer(t, &now, zet.Schedule{DailyGifts: []zet.DailyGift{{ID: 1, When: spring, Gift: bonusSVP(900)}}})

	first := login(server, player)
	now = on(4, 3, 7)
	sameDay := login(server, player)
	now = on(4, 3, 8)
	nextDay := login(server, player)

	want := []string{"1", "0", "0", "10", "", "900", "0", "0", "0", "0"}
	if !slices.Equal(first[4:14], want) || sameDay[4] != "0" || !slices.Equal(nextDay[4:14], want) {
		t.Fatalf("dailys: first %q, same day %q, next day %q", first[4:14], sameDay[4:5], nextDay[4:14])
	}
}

func TestHandler_Login_DailyGiftForLowLevels_SkipsHigherLevels(t *testing.T) {
	now := on(4, 2, 12)
	server, s, player := campaignServer(t, &now, zet.Schedule{DailyGifts: []zet.DailyGift{{ID: 1, When: spring, MaxLevel: 1, Gift: bonusSVP(330)}}})
	s.RecordResult(player, store.Result{Played: on(4, 1, 12), Exp: 500})

	if got := login(server, player); got[4] != "0" {
		t.Fatalf("dailys count = %q, want 0", got[4])
	}
}

func TestHandler_Login_LoginLadder_GivesTheStepOfEachDayLoggedIn(t *testing.T) {
	now := on(4, 2, 12)
	ladder := zet.LoginLadder{ID: 3, Name: "Spring", When: spring, Steps: []zet.Gift{zg("Day 1", 100), zg("Day 2", 200)}, Rounds: 1}
	server, s, player := campaignServer(t, &now, zet.Schedule{LoginLadders: []zet.LoginLadder{ladder}})

	first := login(server, player)
	login(server, player)
	now = on(4, 9, 12)
	second := login(server, player)
	now = on(4, 10, 12)
	third := login(server, player)

	updated, _ := s.Player(player)
	if first[5] != "1" || first[6] != "1" || first[7] != "100" || second[6] != "2" || second[7] != "200" || third[5] != "0" || updated.Zet != 300 {
		t.Fatalf("thrus: %q, %q, %q; zet %d", first[5:8], second[5:8], third[5:6], updated.Zet)
	}
}

func TestHandler_Login_LoginLadderStepWithoutZet_IsShownUnderTheLaddersName(t *testing.T) {
	now := on(4, 2, 12)
	item := zet.Gift{Gift: store.Gift{Item: store.Owned{Kind: store.Weapon, ItemID: 13, Name: "M60", Count: 2}}}
	ladder := zet.LoginLadder{ID: 3, Name: "Start Dash", When: spring, Steps: []zet.Gift{item}}
	server, s, player := campaignServer(t, &now, zet.Schedule{LoginLadders: []zet.LoginLadder{ladder}})

	got := login(server, player)

	owned, _ := s.Owned(player)
	if got[5] != "0" || !slices.Contains(got, "Start Dash") || len(owned) != 1 || owned[0].Count != 2 {
		t.Fatalf("lines %q; owned %+v", got, owned)
	}
}

func TestHandler_Login_ReturningPack_NeedsTheDaysAway(t *testing.T) {
	now := on(4, 2, 12)
	pack := zet.ReturningPack{ID: 5, When: spring, AbsentDays: 15, Gift: zg("Welcome back", 15000)}
	server, s, player := campaignServer(t, &now, zet.Schedule{Returning: []zet.ReturningPack{pack}})
	s.RecordResult(player, store.Result{Played: on(3, 10, 12)})

	back := login(server, player)
	now = on(4, 20, 12)
	again := login(server, player)

	if updated, _ := s.Player(player); !slices.Contains(back, "Welcome back") || slices.Contains(again, "Welcome back") || updated.Zet != 15000 {
		t.Fatalf("first %q, then %q; zet %d", back, again, updated.Zet)
	}
}

func TestHandler_Login_ReturningPack_NotForACardSeenRecently(t *testing.T) {
	now := on(4, 2, 12)
	pack := zet.ReturningPack{ID: 5, When: spring, AbsentDays: 15, Gift: zg("Welcome back", 15000)}
	server, s, player := campaignServer(t, &now, zet.Schedule{Returning: []zet.ReturningPack{pack}})
	s.RecordResult(player, store.Result{Played: on(3, 25, 12)})

	if got := login(server, player); slices.Contains(got, "Welcome back") {
		t.Fatalf("lines %q", got)
	}
}

func TestHandler_Result_ReachingAPlayLadderStep_RepliesWithTheReward(t *testing.T) {
	now := on(4, 2, 12)
	ladder := zet.PlayLadder{ID: 2, When: spring, Filter: store.ResultFilter{Modes: []int64{7, 8, 9, 10}}, Steps: []zet.PlayStep{
		{At: 1, Gift: zet.Gift{Gift: store.Gift{Name: "First match", Item: store.Owned{Kind: store.Weapon, ItemID: 13, Name: "M60", Count: 10}}}},
		{At: 2, Gift: zg("Second match", 500)},
	}}
	server, _, player := campaignServer(t, &now, zet.Schedule{PlayLadders: []zet.PlayLadder{ladder}})
	versus := fmt.Sprintf("player_id=%d&game_mode=7", player)

	first := strings.Split(postTo(server, "/result", versus), "\n")
	coop := postTo(server, "/result", fmt.Sprintf("player_id=%d&game_mode=1", player))
	second := strings.Split(postTo(server, "/result", versus), "\n")

	empty := slices.Repeat([]string{"0"}, 15)
	wantFirst := slices.Concat([]string{"1", "1", "0"}, empty, []string{"1", "First match", "0", "13", "1", "M60", "10", "13", "0", "0", "0", ""})
	wantSecond := slices.Concat([]string{"1", "1", "0"}, empty, []string{"1", "Second match", "500", "0", ""})
	if !slices.Equal(first, wantFirst) || coop != "1\n1\n0\n" || !slices.Equal(second, wantSecond) {
		t.Fatalf("replies:\n%q\n%q\n%q", first, coop, second)
	}
}
