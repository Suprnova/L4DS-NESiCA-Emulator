package zet

import (
	"log/slog"
	"strconv"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
)

// DailyGift is a gift every card gets once a service day, at its first login of the day while the
// gift is open, shown in the login pop-up's daily division (dailys): bonus SVP, a double day's
// extra SVP, daily ZG. Its item, if any, is a weapon, a skill item, or SVP, the only ones the
// division shows.
type DailyGift struct {
	ID   int
	When calendar.Span
	// MaxLevel limits the gift to cards at this level or below, as the official level-1 bonus
	// was; 0 gives it to every card.
	MaxLevel int
	Gift
}

// LoginLadder is a list of gifts, one for each service day a card logs in during a run, in order:
// the first day's login gets the first step, and so on.
type LoginLadder struct {
	ID   int
	Name string
	When calendar.Span
	// Steps are the gifts in order. A step with ZG is shown in the pop-up's cumulative login
	// division (thrus) with its day count; one without ZG, which that division drops, under
	// Name with the grants (period_bonus).
	Steps []Gift
	// Rounds is how many times a card goes through the steps in a run; 0 starts over without end.
	Rounds int
}

// ReturningPack is a gift for a card that comes back after AbsentDays service days or more
// without a login or a play, given at its first login during a run, once per run.
type ReturningPack struct {
	ID         int
	When       calendar.Span
	AbsentDays int
	Gift
}

// PlayLadder is a set of gifts for counts of a card's plays during a run, such as clears of a new
// stage or plays of a versus map, given in the result reply (mode_reward) of the play that
// reaches each count.
type PlayLadder struct {
	ID     int
	When   calendar.Span
	Filter store.ResultFilter
	Steps  []PlayStep
}

// PlayStep is a gift for reaching At counted plays.
type PlayStep struct {
	At int
	Gift
}

// Campaign kinds, as the store records their steps.
const (
	dailyKind     = "daily"
	ladderKind    = "ladder"
	returningKind = "returning"
	playKind      = "play"
)

// loginGifts are the campaign gifts given at a login, by the pop-up group that shows them.
type loginGifts struct {
	dailys []store.Gift
	thrus  []thru
	// named are shown with the grants (period_bonus).
	named []delivered
}

// thru is a gift of a login ladder, shown with the day count it was given for.
type thru struct {
	days int
	gift store.Gift
}

// deliverCampaigns gives a player the login campaign gifts due at now: those of the daily gifts,
// the login ladders, and the returning-player packs. The player's login today must already be
// recorded.
func (s *Server) deliverCampaigns(player store.Player, now time.Time) loginGifts {
	var given loginGifts
	today := calendar.DayOf(now)
	level := s.levels.Level(player.Exp)
	deliver := func(step store.Step, gift Gift) (store.Gift, bool) {
		resolved, _ := gift.For(player.CharaID)
		ok, err := s.store.DeliverStep(player.ID, step, now, resolved)
		if err != nil {
			slog.Error("deliver campaign gift", "player", player.ID, "step", step, "error", err)
		}
		if ok {
			slog.Info("campaign gift delivered", "player", player.ID, "kind", step.Kind, "campaign", step.ID, "run", step.Run, "step", step.Step)
		}
		return resolved, ok
	}

	for _, daily := range s.schedule.DailyGifts {
		run, open := daily.When.At(now)
		if !open || daily.MaxLevel > 0 && level > daily.MaxLevel {
			continue
		}
		if gift, ok := deliver(store.Step{Kind: dailyKind, ID: int64(daily.ID), Run: run.N, Step: dayNumber(today)}, daily.Gift); ok {
			given.dailys = append(given.dailys, gift)
		}
	}

	for _, ladder := range s.schedule.LoginLadders {
		run, open := ladder.When.At(now)
		if !open {
			continue
		}
		days, err := s.store.LoginDays(player.ID, calendar.DayOf(run.Start), today.AddDate(0, 0, 1))
		if err != nil {
			slog.Error("count login days", "player", player.ID, "error", err)
			continue
		}
		if days < 1 || ladder.Rounds > 0 && days > ladder.Rounds*len(ladder.Steps) {
			continue
		}
		step := ladder.Steps[(days-1)%len(ladder.Steps)]
		gift, ok := deliver(store.Step{Kind: ladderKind, ID: int64(ladder.ID), Run: run.N, Step: days}, step)
		switch {
		case !ok:
		case gift.Zet > 0:
			given.thrus = append(given.thrus, thru{days, gift})
		default:
			gift.Name = ladder.Name
			given.named = append(given.named, delivered{int64(ladder.ID), gift})
		}
	}

	for _, pack := range s.schedule.Returning {
		run, open := pack.When.At(now)
		if !open {
			continue
		}
		last, seen, err := s.store.LastSeen(player.ID, today)
		if err != nil {
			slog.Error("find when a player was last seen", "player", player.ID, "error", err)
			continue
		}
		if !seen || calendar.DayOf(last).AddDate(0, 0, pack.AbsentDays).After(today) {
			continue
		}
		if gift, ok := deliver(store.Step{Kind: returningKind, ID: int64(pack.ID), Run: run.N}, pack.Gift); ok {
			given.named = append(given.named, delivered{int64(pack.ID), gift})
		}
	}
	return given
}

// deliverPlayRewards gives a player the play ladder gifts their result at now reaches, and
// returns them. The result must already be recorded.
func (s *Server) deliverPlayRewards(player store.Player, now time.Time) []store.Gift {
	var given []store.Gift
	for _, ladder := range s.schedule.PlayLadders {
		run, open := ladder.When.At(now)
		if !open {
			continue
		}
		count, err := s.store.CountResults(player.ID, run.Start, run.End, ladder.Filter)
		if err != nil {
			slog.Error("count results", "player", player.ID, "error", err)
			continue
		}
		for i, step := range ladder.Steps {
			if step.At > count {
				continue
			}
			gift, _ := step.For(player.CharaID)
			ok, err := s.store.DeliverStep(player.ID, store.Step{Kind: playKind, ID: int64(ladder.ID), Run: run.N, Step: i}, now, gift)
			if err != nil {
				slog.Error("deliver play reward", "player", player.ID, "ladder", ladder.ID, "error", err)
			}
			if ok {
				slog.Info("play reward delivered", "player", player.ID, "ladder", ladder.ID, "run", run.N, "at", step.At)
				given = append(given, gift)
			}
		}
	}
	return given
}

// dayNumber numbers a service day by the days since 1970.
func dayNumber(day time.Time) int { return int(day.Unix() / (24 * 60 * 60)) }

// itemLines lists an item block: id, category, name, count, resource id, color index, replace,
// and rare. A weapon's resource id is its id, an avatar item's resource id and color are the ones
// its id carries, and everything else has 0 for both. Replace and rare are 0: a nonzero rare on
// an avatar item makes the pop-up take the next entry as its partner (GameUI 0x10058850).
func itemLines(item store.Owned) []string {
	resource, color := 0, 0
	switch {
	case item.Kind == store.Weapon:
		resource = int(item.ItemID)
	case item.Kind.Wearable():
		resource, color = avatarParts(item.ItemID)
	}
	return []string{strconv.FormatInt(item.ItemID, 10), strconv.FormatInt(int64(item.Kind), 10), item.Name,
		strconv.FormatInt(item.Count, 10), strconv.Itoa(resource), strconv.Itoa(color), "0", "0"}
}

// dailyLines lists the dailys group: the number of gifts, then each one's ZG and item block.
func dailyLines(gifts []store.Gift) []string {
	lines := []string{strconv.Itoa(len(gifts))}
	for _, gift := range gifts {
		lines = append(lines, strconv.FormatInt(gift.Zet, 10))
		lines = append(lines, itemLines(gift.Item)...)
	}
	return lines
}

// thruLines lists the thrus group: the number of gifts, then each one's day count, ZG, and item
// block.
func thruLines(gifts []thru) []string {
	lines := []string{strconv.Itoa(len(gifts))}
	for _, t := range gifts {
		lines = append(lines, strconv.Itoa(t.days), strconv.FormatInt(t.gift.Zet, 10))
		lines = append(lines, itemLines(t.gift.Item)...)
	}
	return lines
}

// resultLines lists the result reply for a chapter that earned rewards (engine.dll 0x10208520):
// a success header, every group before mode_reward empty and every single value 0 (what the game
// reads from a header-only reply), then mode_reward, each reward's name, ZG, and item block. In
// the result reply an empty item is one 0 line (0x10205e20).
func resultLines(rewards []store.Gift) []string {
	lines := []string{"1", "1", "0",
		"0",      // titles
		"0",      // levels
		"0",      // event_prizes
		"0", "0", // class_id, result_id
		"0", "0", // weapon_slots, item_slots
		"0",      // stage_end
		"0",      // infos
		"0", "0", // stage_remain_getting_title, chapter_remain_getting_title
		"0", "0", // daily_quest, chain_quest
		"0", // vs_class_id
		"0", // revisit_campaign
		strconv.Itoa(len(rewards)),
	}
	for _, gift := range rewards {
		lines = append(lines, gift.Name, strconv.FormatInt(gift.Zet, 10))
		if gift.Item.Kind == 0 {
			lines = append(lines, "0")
		} else {
			lines = append(lines, itemLines(gift.Item)...)
		}
	}
	return lines
}
