package shim

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/maintenance"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/remote"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// Server is what a link asks of the server: whether it answers and its maintenance status, and
// which store it lists under an id.
type Server interface {
	Hello() (maintenance.Status, error)
	Store(id uint32) (remote.Store, error)
}

// linkState is where a link stands with its server.
type linkState int

const (
	unchecked linkState = iota
	online
	warned
	closed
	offline
	refused
)

func (s linkState) String() string {
	return [...]string{"unchecked", "online", "warned of maintenance", "closed for maintenance", "offline", "refused"}[s]
}

// Link keeps the game told whether its server can be reached. Online, a game is certified in
// the store the server lists under the cabinet's store id. Offline, it gets NW_ERROR, and
// NWRECOVER_NOTICE and CERT_INIT_NOTICE when the server is back. Refused, because the server does
// not list the store, it gets CERT_ERROR "unregistered store".
//
// The server's maintenance windows reach the game the same way: a warning with the minutes left
// while one is coming, NW_ERROR "maintenance" during it, and NWRECOVER_NOTICE, CERT_INIT_NOTICE
// and a cleared warning after it.
type Link struct {
	server    Server
	storeID   uint32
	serverKey string
	status    *nesys.Status
	state     linkState
}

// NewLink returns a link to server for a cabinet in store storeID, reporting serverKey to the
// game. It starts offline until its first check.
func NewLink(server Server, storeID uint32, serverKey string) *Link {
	return &Link{
		server:    server,
		storeID:   storeID,
		serverKey: serverKey,
		status:    nesys.NewStatus(nesys.NetworkError(nesys.CodeHTTPAccess)),
	}
}

// Status is the status games connected through this link are told.
func (l *Link) Status() *nesys.Status { return l.status }

// Check asks the server once and tells the games when the link's state changes.
func (l *Link) Check() {
	status, err := l.server.Hello()
	var store remote.Store
	if err == nil && status.Phase != maintenance.Active {
		store, err = l.server.Store(l.storeID)
	}

	switch {
	case err == nil && status.Phase == maintenance.Active:
		down := []wire.Frame{nesys.NetworkError(nesys.CodeMaintenance)}
		l.become(closed, down, down)
	case err == nil:
		l.certify(store, status)
	case errors.Is(err, remote.ErrUnknownStore):
		refusal := []wire.Frame{nesys.CertError(nesys.CodeUnregisteredStore)}
		l.become(refused, refusal, refusal, "store", l.storeID, "error", err)
	default:
		lost := []wire.Frame{nesys.NetworkError(nesys.CodeHTTPAccess)}
		l.become(offline, lost, lost, "error", err)
	}
}

// certify keeps the game certified in store, warned while maintenance is coming. A game coming
// back from offline or maintenance is told the network has recovered; a warning that no longer
// holds is cleared.
func (l *Link) certify(store remote.Store, status maintenance.Status) {
	certified := nesys.CertInit(wire.StoreInfo{ID: store.ID, Name: store.Name, Prefecture: store.Prefecture}, wire.NewsImage{}, l.serverKey)
	current := []wire.Frame{certified}
	var change []wire.Frame
	if l.state != online && l.state != warned {
		change = []wire.Frame{nesys.NetworkRecover(), certified}
	}
	if l.state == closed || l.state == warned && status.Phase != maintenance.Coming {
		change = append(change, nesys.MaintenanceCleared())
	}

	state := online
	if status.Phase == maintenance.Coming {
		state = warned
		warning := nesys.MaintenanceComing(status.Minutes)
		current = append(current, warning)
		if l.state != warned {
			change = append(change, warning)
		}
	}
	l.become(state, current, change, "store", store.Name)
}

// become moves the link to state, making current what connecting games get and sending change
// to connected ones. In the state already, it only updates what connecting games get.
func (l *Link) become(state linkState, current, change []wire.Frame, details ...any) {
	if state == l.state {
		l.status.Set(current, nil)
		return
	}
	slog.Info("server link "+state.String(), details...)
	l.state = state
	l.status.Set(current, change)
}

// CheckInterval is how often a link checks its server.
const CheckInterval = 10 * time.Second

// Run checks the server every interval until ctx is done.
func (l *Link) Run(ctx context.Context, interval time.Duration) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			l.Check()
		}
	}
}
