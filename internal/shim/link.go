package shim

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/remote"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// Server is what a link asks of the server: whether it answers, and which store it lists
// under an id.
type Server interface {
	Hello() error
	Store(id uint32) (remote.Store, error)
}

// linkState is where a link stands with its server.
type linkState int

const (
	unchecked linkState = iota
	online
	offline
	refused
)

func (s linkState) String() string {
	return [...]string{"unchecked", "online", "offline", "refused"}[s]
}

// Link keeps the game told whether its server can be reached. Online, a game is certified in
// the store the server lists under the cabinet's store id. Offline, it gets NW_ERROR, and
// NWRECOVER_NOTICE and CERT_INIT_NOTICE when the server is back. Refused, because the server does
// not list the store, it gets CERT_ERROR "unregistered store".
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
	err := l.server.Hello()
	var store remote.Store
	if err == nil {
		store, err = l.server.Store(l.storeID)
	}

	switch {
	case err == nil:
		certified := nesys.CertInit(wire.StoreInfo{ID: store.ID, Name: store.Name, Prefecture: store.Prefecture}, wire.NewsImage{}, l.serverKey)
		l.become(online, []wire.Frame{certified}, []wire.Frame{nesys.NetworkRecover(), certified}, "store", store.Name)
	case errors.Is(err, remote.ErrUnknownStore):
		refusal := []wire.Frame{nesys.CertError(nesys.CodeUnregisteredStore)}
		l.become(refused, refusal, refusal, "store", l.storeID, "error", err)
	default:
		lost := []wire.Frame{nesys.NetworkError(nesys.CodeHTTPAccess)}
		l.become(offline, lost, lost, "error", err)
	}
}

// become moves the link to state, making current what connecting games get and sending change
// to connected ones, unless it is in that state already.
func (l *Link) become(state linkState, current, change []wire.Frame, details ...any) {
	if state == l.state {
		return
	}
	slog.Info("server link "+state.String(), details...)
	l.state = state
	l.status.Set(current, change)
}

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
