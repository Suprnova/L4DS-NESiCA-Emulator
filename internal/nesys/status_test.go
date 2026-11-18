package nesys_test

import (
	"encoding/binary"
	"net"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// serveStatus serves the test config with status and returns the game's end of the pipe.
func serveStatus(t *testing.T, status *nesys.Status) net.Conn {
	t.Helper()
	config := testConfig
	config.Status = status
	game, service := net.Pipe()
	t.Cleanup(func() { game.Close() })
	go nesys.Serve(service, config)
	return game
}

func TestServe_ConnectWithStatus_SendsTheCurrentNotices(t *testing.T) {
	game := serveStatus(t, nesys.NewStatus(nesys.CertError(nesys.CodeUnregisteredStore)))

	connect := exchange(t, game, wire.ConnectRequest, nil)
	notice, err := wire.ReadFrame(game)

	if connect.Command != wire.ConnectReply || err != nil || notice.Command != wire.CertError ||
		binary.LittleEndian.Uint32(notice.Payload) != nesys.CodeUnregisteredStore {
		t.Fatalf("replies = %+v, %+v, %v", connect, notice, err)
	}
}

func TestStatus_Set_TellsConnectedGames(t *testing.T) {
	status := nesys.NewStatus(nesys.NetworkError(nesys.CodeHTTPAccess))
	game := serveStatus(t, status)
	exchange(t, game, wire.ConnectRequest, nil)
	wire.ReadFrame(game)
	certified := nesys.CertInit(testConfig.Store, wire.NewsImage{}, testConfig.ServerKey)

	status.Set([]wire.Frame{certified}, []wire.Frame{nesys.NetworkRecover(), certified})

	recovered, err := wire.ReadFrame(game)
	init, err2 := wire.ReadFrame(game)
	if err != nil || err2 != nil || recovered.Command != wire.NetworkRecoverNotice || len(recovered.Payload) != 0 || init.Command != wire.CertInitNotice {
		t.Fatalf("notices = %+v (%v), %+v (%v)", recovered, err, init, err2)
	}
}

func TestStatus_Set_IsWhatLaterGamesGet(t *testing.T) {
	status := nesys.NewStatus(nesys.NetworkError(nesys.CodeHTTPAccess))
	status.Set([]wire.Frame{nesys.NetworkRecover()}, nil)
	game := serveStatus(t, status)

	exchange(t, game, wire.ConnectRequest, nil)
	notice, err := wire.ReadFrame(game)

	if err != nil || notice.Command != wire.NetworkRecoverNotice {
		t.Fatalf("notice = %+v, %v", notice, err)
	}
}
