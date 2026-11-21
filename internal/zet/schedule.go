package zet

import (
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
)

// Schedule is what the game can play besides the stage list, the national versus schedule and
// the events, which the game fetches once at startup; and the grants handed out at login. Each
// entry is open during the runs of its span. The startup lists send every run open at the time of
// the request or starting no later than the next day turn, each with its own times, and the game opens
// and closes them itself; a run starting later is sent once the game restarts after it.
type Schedule struct {
	Versus []VersusUnit
	Events []Event
	Grants []Grant
	// CoopStages are the national co-op stage lists. The first open at the time of the stage list
	// request is what national co-op offers; with none open, it offers every stage.
	CoopStages []StageList
	// The campaigns: gifts once a day, for days logged in, for coming back, and for plays.
	DailyGifts   []DailyGift
	LoginLadders []LoginLadder
	Returning    []ReturningPack
	PlayLadders  []PlayLadder
}

// StageList is a list of stages offered while its span is open, such as the official service's
// four national co-op stages, changed every Thursday.
type StageList struct {
	When calendar.Span
	// Stages are stage ids, in the order offered.
	Stages []int
}

// Grant is a gift every card gets once in each run of the grant's span, at its first login during
// the run. The login's reward pop-up shows it under its name: a page for its ZG when it has any,
// then one for its item.
type Grant struct {
	// ID tells grants apart: a card that had a run of the grant with an id does not get that run
	// again.
	ID   int
	When calendar.Span
	Gift
}

// VersusUnit is one map of the national versus schedule. The mode select offers versus during a
// run of the unit's span.
type VersusUnit struct {
	ID, StageID, ChapterID int
	// Difficulty is the game's difficulty plus 1, as in room entries.
	Difficulty int
	// Cost is the SVP charged to enter; 0 charges the operator's arcadeVersusConsumePoint.
	Cost       int
	TakeWeapon bool
	// GameMode is a mode from update\scripts\gamemodes.txt that the stage's mission lists.
	GameMode string
	When     calendar.Span
}

// Event types. The store types are offered only when the event's id equals the operator's
// arcadeStoreEventID.
const (
	EventNational   = 1
	EventStore      = 2
	EventNationalEX = 4
	EventStoreEX    = 5
)

// Event is a national or store event: a stage played with special rules. It is open during a run
// of its span (always, for the zero span) and its daily hours.
type Event struct {
	ID, Type, StageID, ChapterID int
	// Difficulty is the game's difficulty plus 1, as in room entries.
	Difficulty int
	// SpecialMode 1 shows a fifth "GOD" difficulty badge.
	SpecialMode int
	// Cost is the SVP charged to enter; 0 charges the operator's arcadeVersusConsumePoint.
	Cost                                   int
	TakeWeapon, TakeDisposable, MultiMatch bool
	GameMode                               string
	// Name and Detail are shown on the event announcement; "|" in Detail starts a new line.
	Name, Detail string
	// The entry conditions shown on the announcement: a level range and a class range.
	MinLevel, MaxLevel, MinClassID, MaxClassID int
	When                                       calendar.Span
	// DailyOpen and DailyClose are the event's hours, as times whose time of day (UTC) counts;
	// equal ones mean all day.
	DailyOpen, DailyClose time.Time
	// Prizes are given for the ranks of each run, at a ranked card's first login after the run
	// closes. An always-open event never closes, so it gives none.
	Prizes []EventPrize
	// Participation is given for a card's first play in each run, in that play's result reply;
	// nil gives nothing.
	Participation *Gift
}

// startupRuns returns the runs of span that the startup lists send at now: those open now or
// starting no later than the next day turn, so a cabinet started during the morning maintenance
// gets the runs that start when the day turns.
func startupRuns(span calendar.Span, now time.Time) []calendar.Run {
	return span.Runs(now, calendar.NextDayTurn(now).Add(time.Nanosecond))
}

// versusList answers the national versus schedule: the unit count, then nine lines per run of a
// unit.
func (s *Server) versusList(w http.ResponseWriter, _ *http.Request) {
	now := s.now()
	var lines []string
	for _, u := range s.schedule.Versus {
		for _, run := range startupRuns(u.When, now) {
			lines = append(lines, strconv.Itoa(u.ID), strconv.Itoa(u.StageID), strconv.Itoa(u.ChapterID),
				strconv.Itoa(u.Difficulty), strconv.Itoa(u.Cost), flag(u.TakeWeapon), u.GameMode, unix(run.Start), unix(run.End))
		}
	}
	header := []string{"1", "1", "0", strconv.Itoa(len(lines) / 9)}
	w.Write([]byte(strings.Join(append(header, lines...), "\n") + "\n"))
}

// eventList answers the events: the event count, then 21 lines per run of an event. An event open
// always is sent with zero times.
func (s *Server) eventList(w http.ResponseWriter, _ *http.Request) {
	now := s.now()
	var lines []string
	for _, e := range s.schedule.Events {
		for _, run := range startupRuns(e.When, now) {
			lines = append(lines, strconv.Itoa(e.ID), strconv.Itoa(e.Type), strconv.Itoa(e.StageID), strconv.Itoa(e.ChapterID),
				strconv.Itoa(e.Difficulty), strconv.Itoa(e.SpecialMode), strconv.Itoa(e.Cost),
				flag(e.TakeWeapon), flag(e.TakeDisposable), flag(e.MultiMatch), e.GameMode, e.Name, e.Detail,
				strconv.Itoa(e.MinLevel), strconv.Itoa(e.MaxLevel), strconv.Itoa(e.MinClassID), strconv.Itoa(e.MaxClassID),
				unix(run.Start), unix(run.End), unix(e.DailyOpen), unix(e.DailyClose))
		}
	}
	header := []string{"1", "1", "0", strconv.Itoa(len(lines) / 21)}
	w.Write([]byte(strings.Join(append(header, lines...), "\n") + "\n"))
}
func flag(b bool) string {
	if b {
		return "1"
	}
	return "0"
}

// unix formats a time as the game reads it, in Unix seconds; the zero time is 0.
func unix(t time.Time) string {
	if t.IsZero() {
		return "0"
	}
	return strconv.FormatInt(t.Unix(), 10)
}
