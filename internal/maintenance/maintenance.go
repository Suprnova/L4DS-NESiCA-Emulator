// Package maintenance decides when the server is down for maintenance: windows that may repeat, such
// as one every day, and how long before a window the games are warned.
package maintenance

import (
	"slices"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/calendar"
)

// Window is a stretch of maintenance, from Start until End.
type Window struct {
	Start, End time.Time
}

// Schedule is when maintenance happens. Windows that overlap or touch, such as a one-off window
// extending a daily one, count as one window.
type Schedule struct {
	// Windows are the maintenance windows, each a span that may repeat.
	Windows []calendar.Span
	// Warn is how long before a window the games are told it is coming; 0 tells them nothing.
	Warn time.Duration
}

// Phase is where the server stands with maintenance.
type Phase string

const (
	// None is no maintenance under way or about to start.
	None Phase = ""
	// Coming is a window starting within the schedule's Warn.
	Coming Phase = "coming"
	// Active is a window under way: the server is down for maintenance.
	Active Phase = "active"
)

// Status is the maintenance phase at a moment.
type Status struct {
	Phase Phase `json:"phase,omitempty"`
	// Minutes is how many minutes are left, rounded up, until a Coming window starts.
	Minutes int `json:"minutes,omitempty"`
}

// At returns the maintenance status at now.
func (s Schedule) At(now time.Time) Status {
	window, ok := s.Next(now)
	switch {
	case !ok:
		return Status{}
	case !now.Before(window.Start):
		return Status{Phase: Active}
	case s.Warn > 0 && window.Start.Sub(now) <= s.Warn:
		return Status{Phase: Coming, Minutes: int((window.Start.Sub(now) + time.Minute - 1) / time.Minute)}
	}
	return Status{}
}

// lookahead is how far ahead of now Next looks for a window.
const lookahead = 7 * 24 * time.Hour

// Next returns the window under way at now or else the next one starting within a week, with
// overlapping and touching windows merged, and false when there is none.
func (s Schedule) Next(now time.Time) (Window, bool) {
	var windows []Window
	for _, span := range s.Windows {
		for _, run := range span.Runs(now, now.Add(lookahead)) {
			windows = append(windows, Window{run.Start, run.End})
		}
	}
	if len(windows) == 0 {
		return Window{}, false
	}
	slices.SortFunc(windows, func(a, b Window) int { return a.Start.Compare(b.Start) })
	merged := windows[0]
	for _, w := range windows[1:] {
		if w.Start.After(merged.End) {
			break
		}
		if w.End.After(merged.End) {
			merged.End = w.End
		}
	}
	return merged, true
}
