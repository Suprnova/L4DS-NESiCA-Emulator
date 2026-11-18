package shim_test

import (
	"encoding/binary"
	"errors"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/remote"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/shim"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// server is a fake server whose answers the test changes.
type server struct {
	down  bool
	store *remote.Store
}

func (s *server) Hello() error {
	if s.down {
		return errors.New("connection refused")
	}
	return nil
}

func (s *server) Store(uint32) (remote.Store, error) {
	if s.store == nil {
		return remote.Store{}, remote.ErrUnknownStore
	}
	return *s.store, nil
}

func commands(frames []wire.Frame) []wire.Command {
	var ids []wire.Command
	for _, frame := range frames {
		ids = append(ids, frame.Command)
	}
	return ids
}

func TestLink_BeforeTheFirstCheck_IsOffline(t *testing.T) {
	link := shim.NewLink(&server{}, 1, "key")

	got := link.Status().Current()

	if len(got) != 1 || got[0].Command != wire.NetworkError || binary.LittleEndian.Uint32(got[0].Payload) != nesys.CodeHTTPAccess {
		t.Fatalf("Current() = %+v", got)
	}
}

func TestLink_ServerListsTheStore_CertifiesTheGameInIt(t *testing.T) {
	link := shim.NewLink(&server{store: &remote.Store{ID: 7, Name: "Arcade", Prefecture: "Osaka"}}, 7, "key")

	link.Check()

	got := link.Status().Current()
	if len(got) != 1 || got[0].Command != wire.CertInitNotice {
		t.Fatalf("Current() = %v", commands(got))
	}
}

func TestLink_ServerDoesNotListTheStore_RefusesWithUnregisteredStore(t *testing.T) {
	link := shim.NewLink(&server{}, 7, "key")

	link.Check()

	got := link.Status().Current()
	if len(got) != 1 || got[0].Command != wire.CertError || binary.LittleEndian.Uint32(got[0].Payload) != nesys.CodeUnregisteredStore {
		t.Fatalf("Current() = %+v", got)
	}
}

func TestLink_ServerGoesDownAndComesBack_GoesOfflineThenRecovers(t *testing.T) {
	fake := &server{store: &remote.Store{ID: 7, Prefecture: "Osaka"}}
	link := shim.NewLink(fake, 7, "key")
	link.Check()

	fake.down = true
	link.Check()
	whileDown := commands(link.Status().Current())
	fake.down = false
	link.Check()
	afterwards := commands(link.Status().Current())

	if len(whileDown) != 1 || whileDown[0] != wire.NetworkError || len(afterwards) != 1 || afterwards[0] != wire.CertInitNotice {
		t.Fatalf("while down %v, afterwards %v", whileDown, afterwards)
	}
}
