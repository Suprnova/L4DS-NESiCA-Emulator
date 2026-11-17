// Package dedicated runs the dedicated game servers that host national rooms. A [Pool] keeps a
// spare server ready, hands it to each new room, and stops a room's server once its players
// have left.
package dedicated

import (
	"context"
	"log/slog"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	pollInterval = 3 * time.Second

	// startTimeout is how long a server may take to load its first map. Loading takes about a
	// minute.
	startTimeout = 3 * time.Minute

	// joinTimeout is how long a room's server waits for its first player. The room's
	// reservation changes map first, which takes about a minute.
	joinTimeout = 3 * time.Minute

	// emptyGrace is how long a room's server keeps running once its last player has left.
	emptyGrace = 30 * time.Second
)

// Config describes the servers a [Pool] runs.
type Config struct {
	// Dir is a server tree built by maketree.ps1, holding srcds.exe.
	Dir string
	// IP is the address the servers bind to and rooms are advertised at.
	IP netip.Addr
	// FirstPort is the game port of the first server; the others use the ports after it.
	FirstPort uint16
	// Max is the most servers that run at once.
	Max int
	// Spares is how many servers are kept starting or ready for new rooms.
	Spares int
	// ShowConsoles shows each server's console window instead of hiding it.
	ShowConsoles bool
}

// Status is what a server's status command reports.
type Status struct {
	// Map is the loaded map, or empty while none is.
	Map string
	// Humans is the number of connected players.
	Humans int
}

// ParseStatus reads the map and the number of players from the output of the status command.
func ParseStatus(output string) Status {
	var status Status
	for line := range strings.Lines(output) {
		key, value, found := strings.Cut(line, ":")
		if !found {
			continue
		}
		value = strings.TrimSpace(value)
		switch strings.TrimSpace(key) {
		case "map":
			status.Map = value
		case "players":
			count, _, _ := strings.Cut(value, " ")
			status.Humans, _ = strconv.Atoi(count)
		}
	}
	return status
}

// process is a running server process.
type process interface {
	Kill()
	Done() <-chan struct{}
}

type state int

const (
	starting state = iota
	idle
	assigned
	occupied
	stopping
)

type server struct {
	port       uint16
	password   string
	proc       process
	state      state
	since      time.Time
	emptySince time.Time
}

type startFunc func(port uint16, password string) (process, error)
type statusFunc func(address netip.AddrPort, password string) (Status, error)

// Pool runs the dedicated servers that host rooms.
type Pool struct {
	config  Config
	start   startFunc
	status  statusFunc
	newPass func() string
	now     func() time.Time

	mu      sync.Mutex
	servers []*server
}

func newPool(config Config, start startFunc, status statusFunc, newPass func() string) *Pool {
	return &Pool{config: config, start: start, status: status, newPass: newPass, now: time.Now}
}

// Run starts the spare servers and supervises every server until ctx is done, then kills them.
func (p *Pool) Run(ctx context.Context) {
	ticker := time.NewTicker(pollInterval)
	defer ticker.Stop()
	for {
		p.poll()
		select {
		case <-ctx.Done():
			p.killAll()
			return
		case <-ticker.C:
		}
	}
}

// Host hands a ready server to a new room and returns its address, or false when no server is
// ready.
func (p *Pool) Host() (netip.AddrPort, bool) {
	p.mu.Lock()
	defer p.mu.Unlock()

	for _, s := range p.servers {
		if s.state == idle {
			s.state, s.since = assigned, p.now()
			slog.Info("dedicated server assigned to a room", "port", s.port)
			return p.address(s), true
		}
	}
	slog.Warn("no dedicated server is ready for a new room")
	return netip.AddrPort{}, false
}

// Hosting reports whether the server at address still hosts the room it was handed.
func (p *Pool) Hosting(address netip.AddrPort) bool {
	p.mu.Lock()
	defer p.mu.Unlock()

	return slices.ContainsFunc(p.servers, func(s *server) bool {
		return p.address(s) == address && (s.state == assigned || s.state == occupied)
	})
}

func (p *Pool) address(s *server) netip.AddrPort { return netip.AddrPortFrom(p.config.IP, s.port) }

// poll queries every running server, applies what it reports, and starts spares.
func (p *Pool) poll() {
	p.mu.Lock()
	queried := slices.DeleteFunc(slices.Clone(p.servers), func(s *server) bool { return s.state == stopping })
	p.mu.Unlock()

	type report struct {
		status Status
		err    error
	}
	reports := make(map[*server]report, len(queried))
	for _, s := range queried {
		status, err := p.status(p.address(s), s.password)
		reports[s] = report{status, err}
	}

	p.mu.Lock()
	defer p.mu.Unlock()
	now := p.now()
	p.servers = slices.DeleteFunc(p.servers, func(s *server) bool {
		select {
		case <-s.proc.Done():
			if s.state != stopping {
				slog.Warn("dedicated server exited", "port", s.port)
			}
			return true
		default:
		}
		if r, ok := reports[s]; ok {
			p.update(s, r.status, r.err, now)
		}
		return false
	})
	p.startSpares(now)
}

// update moves s to its next state. A failed status query leaves the state as it is; the
// server answers nothing while it loads a map.
func (p *Pool) update(s *server, status Status, err error, now time.Time) {
	switch s.state {
	case starting:
		if err == nil && status.Map != "" {
			s.state, s.since = idle, now
			slog.Info("dedicated server ready", "port", s.port, "map", status.Map)
		} else if now.Sub(s.since) > startTimeout {
			p.stop(s, "it did not load a map")
		}
	case assigned:
		if err == nil && status.Humans > 0 {
			s.state, s.since = occupied, now
		} else if now.Sub(s.since) > joinTimeout {
			p.stop(s, "nobody joined its room")
		}
	case occupied:
		if err == nil {
			if status.Humans > 0 {
				s.emptySince = time.Time{}
			} else if s.emptySince.IsZero() {
				s.emptySince = now
			}
		}
		if !s.emptySince.IsZero() && now.Sub(s.emptySince) >= emptyGrace {
			p.stop(s, "its room is empty")
		}
	}
}

func (p *Pool) stop(s *server, reason string) {
	slog.Info("stopping dedicated server", "port", s.port, "reason", reason)
	s.proc.Kill()
	s.state = stopping
}

// startSpares starts servers until Spares of them are starting or ready, on ports no running
// server holds. A stopped server keeps its port until its process has exited.
func (p *Pool) startSpares(now time.Time) {
	spares := 0
	for _, s := range p.servers {
		if s.state == starting || s.state == idle {
			spares++
		}
	}

	for port := p.config.FirstPort; spares < p.config.Spares && int(port-p.config.FirstPort) < p.config.Max; port++ {
		if slices.ContainsFunc(p.servers, func(s *server) bool { return s.port == port }) {
			continue
		}
		password := p.newPass()
		proc, err := p.start(port, password)
		if err != nil {
			slog.Error("start dedicated server", "port", port, "error", err)
			return
		}
		slog.Info("starting dedicated server", "port", port)
		p.servers = append(p.servers, &server{port: port, password: password, proc: proc, state: starting, since: now})
		spares++
	}
}

func (p *Pool) killAll() {
	p.mu.Lock()
	defer p.mu.Unlock()
	for _, s := range p.servers {
		s.proc.Kill()
	}
	for _, s := range p.servers {
		<-s.proc.Done()
	}
	p.servers = nil
}
