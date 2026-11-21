// Package calendar is the timing the server's schedule shares: spans of time that may repeat, in
// the server's time zone, and the service day, which turns at 08:00.
package calendar

import (
	"errors"
	"fmt"
	"time"
)

// DayTurn is when the service day turns, as an offset from midnight: 08:00, as the official
// service's did.
const DayTurn = 8 * time.Hour

// NextDayTurn returns the first day turn after t, in t's time zone.
func NextDayTurn(t time.Time) time.Time {
	turn := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, t.Location()).Add(DayTurn)
	if !turn.After(t) {
		turn = turn.AddDate(0, 0, 1)
	}
	return turn
}

// DayOf returns the start of the service day t falls in: the latest day turn at or before t, in
// t's time zone.
func DayOf(t time.Time) time.Time { return NextDayTurn(t).AddDate(0, 0, -1) }

// Every is how often a span repeats: every Days days, or every Years years on the same date and
// time. The zero Every never repeats.
type Every struct {
	Days, Years int
}

// Span is when a scheduled entry is open: a first run from Start until End, and when Every is set
// a run every Every after it, the last starting no later than Until (the zero Until never ends).
// The zero Span is always open.
type Span struct {
	Start, End time.Time
	Every      Every
	Until      time.Time
}

// Run is one run of a span, from Start until End. N counts the span's runs from 0.
type Run struct {
	N          int
	Start, End time.Time
}

// Always reports whether the span is the zero Span, open at every time.
func (s Span) Always() bool { return s.Start.IsZero() && s.End.IsZero() }

// Check reports what is wrong with the span: an end not after the start, a run that lasts into
// the next, or an Until before the start. The zero Span is fine.
func (s Span) Check() error {
	switch {
	case s.Always():
		return nil
	case !s.End.After(s.Start):
		return fmt.Errorf("end %s is not after the start %s", s.End, s.Start)
	case s.Every == Every{}:
		return nil
	case s.Every.Days < 0 || s.Every.Years < 0 || s.Every.Days > 0 && s.Every.Years > 0:
		return errors.New("a span repeats every so many days or every so many years")
	case s.run(1).Start.Before(s.End):
		return fmt.Errorf("a run lasts into the next: %s is longer than the repeat", s.End.Sub(s.Start))
	case !s.Until.IsZero() && s.Until.Before(s.Start):
		return fmt.Errorf("until %s is before the start %s", s.Until, s.Start)
	}
	return nil
}

// Runs returns the runs open at some time from from until to, in order. An always-open span has
// one run, with zero times.
func (s Span) Runs(from, to time.Time) []Run {
	if s.Always() {
		return []Run{{}}
	}
	var runs []Run
	for n := s.firstEndingAfter(from); ; n++ {
		run := s.run(n)
		if !run.Start.Before(to) || !s.Until.IsZero() && run.Start.After(s.Until) {
			break
		}
		if run.End.After(from) {
			runs = append(runs, run)
		}
		if s.Every == (Every{}) {
			break
		}
	}
	return runs
}

// At returns the run open at t, and false when none is.
func (s Span) At(t time.Time) (Run, bool) {
	runs := s.Runs(t, t.Add(time.Nanosecond))
	if len(runs) == 0 {
		return Run{}, false
	}
	return runs[0], true
}

func (s Span) run(n int) Run {
	years, days := n*s.Every.Years, n*s.Every.Days
	return Run{N: n, Start: s.Start.AddDate(years, 0, days), End: s.End.AddDate(years, 0, days)}
}

// firstEndingAfter returns a run number no later than that of the first run ending after t.
func (s Span) firstEndingAfter(t time.Time) int {
	switch {
	case s.Every.Days > 0:
		return max(int(t.Sub(s.End)/(time.Duration(s.Every.Days)*24*time.Hour))-1, 0)
	case s.Every.Years > 0:
		return max((t.Year()-s.End.Year())/s.Every.Years-1, 0)
	}
	return 0
}
