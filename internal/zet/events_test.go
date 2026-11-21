package zet_test

import (
	"fmt"
	"net/url"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

// rankedEvent is a national event from 2027-04-01 08:00 for a week, repeated weekly, whose first
// place gets a title and 30 M60s, and second and third places 300 ZG.
func rankedEvent() zet.Event {
	title := zet.Gift{Gift: store.Gift{Name: "Hunt", Item: store.Owned{Kind: store.Title, ItemID: 11, Name: "Hunter", Count: 1}}}
	weapons := zet.Gift{Gift: store.Gift{Name: "Hunt", Item: store.Owned{Kind: store.Weapon, ItemID: 13, Name: "M60", Count: 30}}}
	return zet.Event{
		ID: 4, Type: zet.EventNational, Name: "Hunt",
		When: calendar.Span{Start: on(4, 1, 8), End: on(4, 8, 8), Every: calendar.Every{Days: 7}},
		Prizes: []zet.EventPrize{
			{From: 1, To: 1, Gifts: []zet.Gift{title, weapons}},
			{From: 2, To: 3, Gifts: []zet.Gift{zg("Hunt", 300)}},
		},
	}
}

func eventResult(score int64) store.Result {
	return store.Result{Played: on(4, 2, 12), GameMode: 2, EventID: 4, Score: score}
}

func TestHandler_Login_AfterAnEventRun_GivesTheRanksPrize(t *testing.T) {
	now := on(4, 9, 9)
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{rankedEvent()}})
	rival, _ := s.Register("7020392000000002", "Rival", 1, 1)
	s.RecordResult(player, eventResult(500))
	s.RecordResult(player, eventResult(400))
	s.RecordResult(rival.ID, eventResult(800))

	first := login(server, player)
	again := login(server, player)

	want := []string{"2",
		"4_1", "Hunt", "1", "900", "0", "11", "4", "Hunter", "1", "0", "0", "0", "0",
		"4_1", "Hunt", "1", "900", "0", "13", "1", "M60", "30", "13", "0", "0", "0"}
	owned, _ := s.Owned(player)
	if !slices.Equal(first[6:6+len(want)], want) || again[6] != "0" || len(owned) != 2 {
		t.Fatalf("eventprizes: first %q, again %q; owned %+v", first[6:6+len(want)], again[6:7], owned)
	}
}

func TestHandler_Login_DuringAnEventRun_GivesNoPrize(t *testing.T) {
	now := on(4, 3, 12)
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{rankedEvent()}})
	s.RecordResult(player, eventResult(500))

	if got := login(server, player); got[6] != "0" {
		t.Fatalf("eventprizes count = %q, want 0", got[6])
	}
}

func TestHandler_Login_TiedEventTotals_ShareARank(t *testing.T) {
	now := on(4, 9, 9)
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{rankedEvent()}})
	first, _ := s.Register("7020392000000002", "First", 1, 1)
	tied, _ := s.Register("7020392000000003", "Tied", 1, 1)
	s.RecordResult(first.ID, eventResult(900))
	s.RecordResult(tied.ID, eventResult(500))
	s.RecordResult(player, eventResult(500))

	got := login(server, player)

	if want := []string{"1", "4_1", "Hunt", "2", "500", "300"}; !slices.Equal(got[6:12], want) {
		t.Fatalf("eventprizes = %q, want %q", got[6:12], want)
	}
}

func TestHandler_Login_RankWithoutAPrize_GivesNothing(t *testing.T) {
	now := on(4, 9, 9)
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{rankedEvent()}})
	for i, score := range []int64{900, 800, 700} {
		other, _ := s.Register(fmt.Sprintf("702039200000001%d", i), "Other", 1, 1)
		s.RecordResult(other.ID, eventResult(score))
	}
	s.RecordResult(player, eventResult(100))

	if got := login(server, player); got[6] != "0" {
		t.Fatalf("eventprizes count = %q, want 0", got[6])
	}
}

func TestHandler_Login_BeforeTheResultsArePosted_GivesNoPrize(t *testing.T) {
	now := on(4, 9, 7)
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{rankedEvent()}})
	s.RecordResult(player, eventResult(500))

	if got := login(server, player); got[6] != "0" {
		t.Fatalf("eventprizes count = %q, want 0", got[6])
	}
}

func TestHandler_Login_ResultsAfter_PostsTheResultsThatLongAfterTheClose(t *testing.T) {
	now := on(4, 8, 9)
	event := rankedEvent()
	event.ResultsAfter = time.Hour
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{event}})
	s.RecordResult(player, eventResult(500))

	if got := login(server, player); got[6] != "2" {
		t.Fatalf("eventprizes count = %q, want 2", got[6])
	}
}

func TestHandler_ChapterStartedBeforeTheClose_CountsForTheRun(t *testing.T) {
	now := on(4, 8, 8).Add(3 * time.Minute)
	event := rankedEvent()
	participation := zg("Hunt", 100)
	event.Participation = &participation
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{event}})
	rival, _ := s.Register("7020392000000002", "Rival", 1, 1)
	s.RecordResult(rival.ID, eventResult(800))
	started := on(4, 8, 8).Add(-2 * time.Minute).Local().Format(time.DateTime)

	reply := postTo(server, "/result", fmt.Sprintf("player_id=%d&game_mode=2&event_id=4&TOTAL_SCORE=900&begin_play_at=%s", player, url.QueryEscape(started)))
	now = on(4, 9, 9)
	got := login(server, player)

	if !strings.Contains(reply, "\n100\n") || got[6] != "2" || got[9] != "1" {
		t.Fatalf("result reply %q; eventprizes %q", reply, got[6:10])
	}
}

func TestHandler_Login_StoreEventPlays_DoNotCountForANationalEventWithItsID(t *testing.T) {
	now := on(4, 9, 9)
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{rankedEvent()}})
	inStore := eventResult(500)
	inStore.GameMode = 3
	s.RecordResult(player, inStore)

	if got := login(server, player); got[6] != "0" {
		t.Fatalf("eventprizes count = %q, want 0", got[6])
	}
}

func TestHandler_Result_FirstEventPlayOfARun_RepliesWithTheParticipationPrize(t *testing.T) {
	now := on(4, 2, 12)
	event := rankedEvent()
	event.Participation = &zet.Gift{Gift: store.Gift{Name: "Hunt", Zet: 100, Item: store.Owned{Kind: store.SkillItem, ItemID: 29, Name: "Charger Flip", Count: 2}}}
	server, s, player := campaignServer(t, &now, zet.Schedule{Events: []zet.Event{event}})
	play := fmt.Sprintf("player_id=%d&game_mode=2&event_id=4", player)

	first := strings.Split(postTo(server, "/result", play), "\n")
	second := postTo(server, "/result", play)
	now = on(4, 9, 12)
	nextRun := strings.Split(postTo(server, "/result", play), "\n")

	want := slices.Concat([]string{"1", "1", "0", "0", "0", "1", "100", "0", "29", "8", "Charger Flip", "2", "0", "0", "0", "0"},
		slices.Repeat([]string{"0"}, 13), []string{""})
	owned, _ := s.Owned(player)
	if !slices.Equal(first, want) || second != "1\n1\n0\n" || !slices.Equal(nextRun, want) || len(owned) != 1 || owned[0].Count != 4 {
		t.Fatalf("replies:\n%q\n%q\n%q\nowned %+v", first, second, nextRun, owned)
	}
}
