package nesys

import (
	"encoding/binary"
	"log/slog"
	"sync"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// Status is the network status a service reports to the game: the notices a game gets right
// after it connects, and the notices every connected game gets when the status changes.
type Status struct {
	mu          sync.Mutex
	current     []wire.Frame
	subscribers map[chan []wire.Frame]struct{}
}

// NewStatus returns a status whose games get current when they connect.
func NewStatus(current ...wire.Frame) *Status {
	return &Status{current: current, subscribers: map[chan []wire.Frame]struct{}{}}
}

// Set makes current what games get when they connect from now on, and sends change to every
// game connected now.
func (s *Status) Set(current, change []wire.Frame) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.current = current
	for subscriber := range s.subscribers {
		select {
		case subscriber <- change:
		default:
			slog.Warn("a game is not reading its pipe; dropped a status notice")
		}
	}
}

// Current returns what a game gets when it connects.
func (s *Status) Current() []wire.Frame {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.current
}

// subscribe returns a channel of the changes sent from now on, and a function that ends the
// subscription and closes the channel.
func (s *Status) subscribe() (<-chan []wire.Frame, func()) {
	changes := make(chan []wire.Frame, 4)
	s.mu.Lock()
	s.subscribers[changes] = struct{}{}
	s.mu.Unlock()
	return changes, func() {
		s.mu.Lock()
		delete(s.subscribers, changes)
		s.mu.Unlock()
		close(changes)
	}
}

// Codes carried by NW_ERROR and CERT_ERROR (the library's error tables).
const (
	// CodeMaintenance (0x20000001) puts the game in maintenance.
	CodeMaintenance uint32 = 0x20000001
	// CodeHTTPAccess (0x20000011) is the library's HTTP access error, for a server that cannot be
	// reached.
	CodeHTTPAccess uint32 = 0x20000011
	// CodeUnregisteredStore (0x21000001) is the auth server's "unregistered store".
	CodeUnregisteredStore uint32 = 0x21000001
)

// NetworkError returns an NW_ERROR notice with code: the game goes offline.
func NetworkError(code uint32) wire.Frame {
	return wire.Frame{Command: wire.NetworkError, Payload: binary.LittleEndian.AppendUint32(nil, code)}
}

// CertError returns a CERT_ERROR notice with code: the game goes offline, uncertified.
func CertError(code uint32) wire.Frame {
	return wire.Frame{Command: wire.CertError, Payload: binary.LittleEndian.AppendUint32(nil, code)}
}

// NetworkRecover returns an NWRECOVER_NOTICE: the game is online again, and needs a CERT_INIT
// notice after it to be certified again.
func NetworkRecover() wire.Frame { return wire.Frame{Command: wire.NetworkRecoverNotice} }

// CertInit returns the CERT_INIT notice certifying a cabinet in store with serverKey.
func CertInit(store wire.StoreInfo, news wire.NewsImage, serverKey string) wire.Frame {
	return wire.Frame{Command: wire.CertInitNotice, Payload: wire.CertInit{Store: store, News: news, ServerKey: serverKey}.Encode()}
}
