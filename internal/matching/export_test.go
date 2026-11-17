package matching

import "time"

// SetClock makes m read the current time from now.
func (m *Master) SetClock(now func() time.Time) { m.now = now }
