package pipe_test

import (
	"fmt"
	"os"
	"testing"

	"github.com/Microsoft/go-winio"

	"l4ds-nesica-emulator/internal/nesys"
	"l4ds-nesica-emulator/internal/pipe"
	"l4ds-nesica-emulator/internal/wire"
)

func TestListen_GameClient_CompletesTheStartupSequence(t *testing.T) {
	path := fmt.Sprintf(`\\.\pipe\l4ds-test-%d`, os.Getpid())
	listener, err := pipe.Listen(path)
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	config := nesys.Config{Store: wire.StoreInfo{Prefecture: "Tokyo"}, ServerKey: "dev.game.zet.square-enix.co.jp"}
	go func() {
		conn, err := listener.Accept()
		if err == nil {
			nesys.Serve(conn, config)
		}
	}()
	game, err := winio.DialPipe(path, nil)
	if err != nil {
		t.Fatal(err)
	}
	defer game.Close()

	game.Write(wire.Frame{Command: wire.ClientStart, Payload: make([]byte, 0x12C)}.Encode())
	game.Write(wire.Frame{Command: wire.ConnectRequest}.Encode())
	first, err1 := wire.ReadFrame(game)
	second, err2 := wire.ReadFrame(game)
	third, err3 := wire.ReadFrame(game)

	if err1 != nil || err2 != nil || err3 != nil ||
		first.Command != wire.ClientStartReply || second.Command != wire.ConnectReply || third.Command != wire.CertInitNotice {
		t.Fatalf("replies = %#x, %#x, %#x, errors %v %v %v", first.Command, second.Command, third.Command, err1, err2, err3)
	}
}
