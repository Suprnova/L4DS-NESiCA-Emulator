package shim_test

import (
	"encoding/binary"
	"errors"
	"net"
	"slices"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/maintenance"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/remote"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/shim"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// server is a fake server whose answers the test changes.
type server struct {
	down        bool
	store       *remote.Store
	maintenance maintenance.Status
}

func (s *server) Hello() (maintenance.Status, error) {
	if s.down {
		return maintenance.Status{}, errors.New("connection refused")
	}
	return s.maintenance, nil
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

// listed is a fake server listing store 7.
func listed() *server { return &server{store: &remote.Store{ID: 7, Prefecture: "Osaka"}} }

func TestLink_MaintenanceComing_CertifiesAndWarnsWithTheMinutes(t *testing.T) {
	fake := listed()
	fake.maintenance = maintenance.Status{Phase: maintenance.Coming, Minutes: 30}
	link := shim.NewLink(fake, 7, "key")

	link.Check()

	got := link.Status().Current()
	if len(got) != 2 || got[0].Command != wire.CertInitNotice || got[1].Command != wire.SoonMaintenance || string(got[1].Payload[4:6]) != "30" {
		t.Fatalf("Current() = %+v", got)
	}
}

func TestLink_MaintenanceStillComing_GivesConnectingGamesTheMinutesLeft(t *testing.T) {
	fake := listed()
	fake.maintenance = maintenance.Status{Phase: maintenance.Coming, Minutes: 30}
	link := shim.NewLink(fake, 7, "key")
	link.Check()

	fake.maintenance.Minutes = 29
	link.Check()

	got := link.Status().Current()
	if len(got) != 2 || string(got[1].Payload[4:6]) != "29" {
		t.Fatalf("Current() = %+v, want a warning of 29 minutes", got)
	}
}

func TestLink_MaintenanceActive_ClosesTheGame(t *testing.T) {
	fake := listed()
	fake.maintenance = maintenance.Status{Phase: maintenance.Active}
	link := shim.NewLink(fake, 7, "key")

	link.Check()

	got := link.Status().Current()
	if len(got) != 1 || got[0].Command != wire.NetworkError || binary.LittleEndian.Uint32(got[0].Payload) != nesys.CodeMaintenance {
		t.Fatalf("Current() = %+v", got)
	}
}

func TestLink_MaintenanceWindowRunsItsCourse_WarnsClosesAndRecovers(t *testing.T) {
	fake := listed()
	link := shim.NewLink(fake, 7, "key")
	link.Check()
	game, service := net.Pipe()
	t.Cleanup(func() { game.Close() })
	go nesys.Serve(service, nesys.Config{Status: link.Status()})
	connect(t, game)

	var got []wire.Command
	for _, phase := range []maintenance.Phase{maintenance.Coming, maintenance.Active, maintenance.None} {
		fake.maintenance = maintenance.Status{Phase: phase, Minutes: 5}
		link.Check()
	}
	for range 5 {
		frame, err := wire.ReadFrame(game)
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, frame.Command)
	}

	want := []wire.Command{wire.SoonMaintenance, wire.NetworkError, wire.NetworkRecoverNotice, wire.CertInitNotice, wire.SoonMaintenance}
	if !slices.Equal(got, want) {
		t.Fatalf("notices = %v, want %v", got, want)
	}
}

// connect connects the game and reads the notices it gets on connecting.
func connect(t *testing.T, game net.Conn) {
	t.Helper()
	if _, err := game.Write(wire.Frame{Command: wire.ConnectRequest}.Encode()); err != nil {
		t.Fatal(err)
	}
	for _, want := range []wire.Command{wire.ConnectReply, wire.CertInitNotice} {
		if frame, err := wire.ReadFrame(game); err != nil || frame.Command != want {
			t.Fatalf("on connecting: %+v, %v; want %v", frame, err, want)
		}
	}
}
