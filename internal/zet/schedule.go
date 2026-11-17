package zet

import (
	"net/http"
	"strconv"
	"strings"
	"time"
)

// Schedule is what the game can play besides the stage list: the national versus schedule and
// the events. The game fetches both once at startup.
type Schedule struct {
	Versus []VersusUnit
	Events []Event
}

// VersusUnit is one map of the national versus schedule. The mode select offers versus while a
// unit's window is open; a window whose start equals its end is never open.
type VersusUnit struct {
	ID, StageID, ChapterID int
	// Difficulty is the game's difficulty plus 1, as in room entries.
	Difficulty int
	// Cost is the SVP charged to enter; 0 charges the operator's arcadeVersusConsumePoint.
	Cost       int
	TakeWeapon bool
	// GameMode is a mode from update\scripts\gamemodes.txt that the stage's mission lists.
	GameMode   string
	Start, End time.Time
}

// Event types. The store types are offered only when the event's id equals the operator's
// arcadeStoreEventID.
const (
	EventNational   = 1
	EventStore      = 2
	EventNationalEX = 4
	EventStoreEX    = 5
)

// Event is a national or store event: a stage played with special rules. It is open while both
// its period and its daily hours contain the current time; a period or daily hours whose start
// equals its end always contain it, so zero times leave an event open.
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
	// Open and Close bound the event's period; DailyOpen and DailyClose its hours, as times
	// whose time of day (UTC) counts.
	Open, Close, DailyOpen, DailyClose time.Time
}

// versusList answers the national versus schedule: the unit count, then nine lines per unit.
func (s *Server) versusList(w http.ResponseWriter, _ *http.Request) {
	lines := []string{"1", "1", "0", strconv.Itoa(len(s.schedule.Versus))}
	for _, u := range s.schedule.Versus {
		lines = append(lines, strconv.Itoa(u.ID), strconv.Itoa(u.StageID), strconv.Itoa(u.ChapterID),
			strconv.Itoa(u.Difficulty), strconv.Itoa(u.Cost), flag(u.TakeWeapon), u.GameMode, unix(u.Start), unix(u.End))
	}
	w.Write([]byte(strings.Join(lines, "\n") + "\n"))
}

// eventList answers the events: the event count, then 21 lines per event.
func (s *Server) eventList(w http.ResponseWriter, _ *http.Request) {
	lines := []string{"1", "1", "0", strconv.Itoa(len(s.schedule.Events))}
	for _, e := range s.schedule.Events {
		lines = append(lines, strconv.Itoa(e.ID), strconv.Itoa(e.Type), strconv.Itoa(e.StageID), strconv.Itoa(e.ChapterID),
			strconv.Itoa(e.Difficulty), strconv.Itoa(e.SpecialMode), strconv.Itoa(e.Cost),
			flag(e.TakeWeapon), flag(e.TakeDisposable), flag(e.MultiMatch), e.GameMode, e.Name, e.Detail,
			strconv.Itoa(e.MinLevel), strconv.Itoa(e.MaxLevel), strconv.Itoa(e.MinClassID), strconv.Itoa(e.MaxClassID),
			unix(e.Open), unix(e.Close), unix(e.DailyOpen), unix(e.DailyClose))
	}
	w.Write([]byte(strings.Join(lines, "\n") + "\n"))
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
