package zet

import "time"

// SetClock makes s read the current time from now.
func (s *Server) SetClock(now func() time.Time) { s.now = now }
