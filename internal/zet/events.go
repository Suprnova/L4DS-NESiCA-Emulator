package zet

import (
	"fmt"
	"log/slog"
	"slices"
	"strconv"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// EventPrize is the prize for ranks From to To of an event's run. Cards are ranked by their total
// score over their results in the run, and cards with the same total share a rank. Each gift is
// one entry of the login pop-up's event results, shown with the rank and the total.
type EventPrize struct {
	From, To int
	Gifts    []Gift
}

// Campaign kinds of the event gifts, as the store records their steps. A national and a store
// event may share an id, so each has its own kind. A run's prizes are its steps from 0, and the
// participation prize is participationStep.
const (
	nationalEventKind = "event"
	storeEventKind    = "store_event"
	participationStep = -1
)

// inStore reports whether the event is a store event, whose id may also be a national event's.
func (e Event) inStore() bool { return e.Type == EventStore || e.Type == EventStoreEX }

// Clashes reports whether two events cannot both be scheduled: they are both national events or
// both store events, with the same id, so their results and prizes could not be told apart.
func (e Event) Clashes(other Event) bool { return e.ID == other.ID && e.inStore() == other.inStore() }

// mode is the matching mode (game_mode) the event is played in.
func (e Event) mode() int64 {
	if e.inStore() {
		return 3
	}
	return 2
}

// kind is the campaign kind of the event's prizes.
func (e Event) kind() string {
	if e.inStore() {
		return storeEventKind
	}
	return nationalEventKind
}

// results picks the event's results.
func (e Event) results() store.ResultFilter {
	return store.ResultFilter{Modes: []int64{e.mode()}, EventID: int64(e.ID)}
}

// prize returns the prize for rank, and false when the rank has none.
func (e Event) prize(rank int) (EventPrize, bool) {
	i := slices.IndexFunc(e.Prizes, func(p EventPrize) bool { return p.From <= rank && rank <= p.To })
	if i < 0 {
		return EventPrize{}, false
	}
	return e.Prizes[i], true
}

// eventPrize is an event prize given at a login: the event, the card's rank and total score in
// the run, and the gift.
type eventPrize struct {
	event Event
	rank  int
	score int64
	gift  store.Gift
}

// deliverEventPrizes gives a player the prizes of the event runs they played in that closed by
// now and that they have not had, and returns them.
func (s *Server) deliverEventPrizes(player store.Player, now time.Time) []eventPrize {
	var given []eventPrize
	for _, event := range s.schedule.Events {
		if len(event.Prizes) == 0 || event.When.Always() {
			continue
		}
		for _, run := range s.playedRuns(player.ID, event, now) {
			given = append(given, s.deliverRunPrize(player, event, run, now)...)
		}
	}
	return given
}

// playedRuns returns the runs of an event that a player played in and that closed by now.
func (s *Server) playedRuns(playerID int64, event Event, now time.Time) []calendar.Run {
	times, err := s.store.ResultTimes(playerID, now, event.results())
	if err != nil {
		slog.Error("list event results", "player", playerID, "event", event.ID, "error", err)
		return nil
	}
	var runs []calendar.Run
	for _, played := range times {
		run, open := event.When.At(played)
		if open && !run.End.After(now) && (len(runs) == 0 || runs[len(runs)-1].N != run.N) {
			runs = append(runs, run)
		}
	}
	return runs
}

// deliverRunPrize gives a player the prize of their rank in a run of an event, unless they had it,
// and returns what it gave.
func (s *Server) deliverRunPrize(player store.Player, event Event, run calendar.Run, now time.Time) []eventPrize {
	totals, err := s.store.TotalScores(run.Start, run.End, event.results())
	if err != nil {
		slog.Error("total event scores", "event", event.ID, "run", run.N, "error", err)
		return nil
	}
	i := slices.IndexFunc(totals, func(t store.Total) bool { return t.PlayerID == player.ID })
	if i < 0 {
		return nil
	}
	score := totals[i].Score
	rank := 1 + slices.IndexFunc(totals, func(t store.Total) bool { return t.Score == score })
	prize, ok := event.prize(rank)
	if !ok {
		return nil
	}

	var given []eventPrize
	for n, gift := range prize.Gifts {
		resolved, _ := gift.For(player.CharaID)
		ok, err := s.store.DeliverStep(player.ID, store.Step{Kind: event.kind(), ID: int64(event.ID), Run: run.N, Step: n}, now, resolved)
		if err != nil {
			slog.Error("deliver event prize", "player", player.ID, "event", event.ID, "run", run.N, "error", err)
		}
		if ok {
			slog.Info("event prize delivered", "player", player.ID, "event", event.ID, "run", run.N, "rank", rank, "score", score)
			given = append(given, eventPrize{event, rank, score, resolved})
		}
	}
	return given
}

// deliverParticipation gives a player the participation prize of the event a result was played
// in, when it is their first result in the event's run, and returns it.
func (s *Server) deliverParticipation(player store.Player, result store.Result) []store.Gift {
	i := slices.IndexFunc(s.schedule.Events, func(e Event) bool { return e.mode() == result.GameMode && int64(e.ID) == result.EventID })
	if i < 0 || s.schedule.Events[i].Participation == nil {
		return nil
	}
	event := s.schedule.Events[i]
	run, open := event.When.At(result.Played)
	if !open {
		return nil
	}
	gift, _ := event.Participation.For(player.CharaID)
	step := store.Step{Kind: event.kind(), ID: int64(event.ID), Run: run.N, Step: participationStep}
	ok, err := s.store.DeliverStep(player.ID, step, result.Played, gift)
	if err != nil {
		slog.Error("deliver participation prize", "player", player.ID, "event", event.ID, "error", err)
	}
	if !ok {
		return nil
	}
	slog.Info("participation prize delivered", "player", player.ID, "event", event.ID, "run", run.N)
	return []store.Gift{gift}
}

// eventPrizeLines lists the login's eventprizes group: the number of prizes, then for each the
// event's id and type, its name, the rank, the total score, the ZG, and the item block.
func eventPrizeLines(prizes []eventPrize) []string {
	lines := []string{strconv.Itoa(len(prizes))}
	for _, p := range prizes {
		lines = append(lines, fmt.Sprintf("%d_%d", p.event.ID, p.event.Type), p.event.Name,
			strconv.Itoa(p.rank), strconv.FormatInt(p.score, 10), strconv.FormatInt(p.gift.Zet, 10))
		lines = append(lines, itemLines(p.gift.Item)...)
	}
	return lines
}
