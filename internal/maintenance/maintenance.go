// Package maintenance decides when the server is down for maintenance: a window every day, dated
// windows, and how long before a window the games are warned.
package maintenance

import (
	"slices"
	"time"
)

// Window is a span of maintenance, from Start until End.
type Window struct {
	Start, End time.Time
}

// Daily is a window every day, as offsets from midnight in the server's time zone. An End at or
// before Start ends the next day.
type Daily struct {
	Start, End time.Duration
}

// Schedule is when maintenance happens. Windows that overlap or touch, such as a dated window
// extending the daily one, count as one window.
type Schedule struct {
	// Daily, when set, is a window every day.
	Daily *Daily
	// Windows are one-off windows.
	Windows []Window
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

// Next returns the window under way at now or else the next one, with overlapping and touching
// windows merged, and false when there is none.
func (s Schedule) Next(now time.Time) (Window, bool) {
	windows := slices.DeleteFunc(s.windowsAround(now), func(w Window) bool { return !w.End.After(now) })
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

// windowsAround returns the dated windows and the daily window's occurrences from the day before
// now to the day after, which covers any daily window under way or next.
func (s Schedule) windowsAround(now time.Time) []Window {
	windows := slices.Clone(s.Windows)
	if s.Daily != nil {
		midnight := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		for day := -1; day <= 1; day++ {
			start := midnight.AddDate(0, 0, day).Add(s.Daily.Start)
			end := midnight.AddDate(0, 0, day).Add(s.Daily.End)
			if !end.After(start) {
				end = end.AddDate(0, 0, 1)
			}
			windows = append(windows, Window{start, end})
		}
	}
	return windows
}
