package shim_test

import (
	"io"
	"net"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/shim"
)

func TestPutCard_SendsTheCardCommandToTheNetconPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()
	received := make(chan string, 1)
	go func() {
		conn, err := listener.Accept()
		if err != nil {
			received <- err.Error()
			return
		}
		defer conn.Close()
		data, _ := io.ReadAll(conn)
		received <- string(data)
	}()

	err = shim.PutCard(listener.Addr().(*net.TCPAddr).Port, "7020392000000001")

	if got := <-received; err != nil || got != "change_debug_dummyCardID 7020392000000001\n" {
		t.Fatalf("PutCard() = %v; the game got %q", err, got)
	}
}
