package nesys_test

import (
	"encoding/binary"
	"fmt"
	"net"
	"net/http"
	"testing"

	"l4ds-nesica-emulator/internal/nesys"
	"l4ds-nesica-emulator/internal/wire"
)

var testConfig = nesys.Config{
	Store:          wire.StoreInfo{ID: 1, Name: "Test Store", Prefecture: "Tokyo"},
	ServerKey:      "dev.game.zet.square-enix.co.jp",
	Adapter:        wire.AdapterDetails{IP: "10.0.0.5", MAC: "001C42888475"},
	Public:         wire.PublicAddress{IP: "203.0.113.7"},
	ServiceVersion: "2.79 2014/07/08",
}

// startGame serves the test config on one end of an in-memory pipe and returns the game's end.
func startGame(t *testing.T) net.Conn {
	t.Helper()
	game, service := net.Pipe()
	t.Cleanup(func() { game.Close() })
	go nesys.Serve(service, testConfig)
	return game
}

func exchange(t *testing.T, game net.Conn, command wire.Command, payload []byte) wire.Frame {
	t.Helper()
	if _, err := game.Write(wire.Frame{Command: command, Payload: payload}.Encode()); err != nil {
		t.Fatal(err)
	}
	reply, err := wire.ReadFrame(game)
	if err != nil {
		t.Fatal(err)
	}
	return reply
}

func TestServe_ClientStart_RepliesWithClientStartReply(t *testing.T) {
	game := startGame(t)

	reply := exchange(t, game, wire.ClientStart, make([]byte, 0x12C))

	if reply.Command != wire.ClientStartReply || len(reply.Payload) != 0 {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_ConnectRequest_RepliesThenAnnouncesTheNetwork(t *testing.T) {
	game := startGame(t)

	connect := exchange(t, game, wire.ConnectRequest, nil)
	notice, err := wire.ReadFrame(game)

	if connect.Command != wire.ConnectReply || err != nil || notice.Command != wire.CertInitNotice {
		t.Fatalf("replies = %+v, %+v, %v", connect, notice, err)
	}
	if notice.Payload[0x65] != 'T' {
		t.Fatalf("prefecture byte = %#x, want it set", notice.Payload[0x65])
	}
}

func TestServe_QueryCommands_ReplyWithMatchingCommandAndSize(t *testing.T) {
	tests := []struct {
		request wire.Command
		reply   wire.Command
		size    int
	}{
		{wire.ServiceVersion, wire.ServiceVersionReply, 0x21},
		{wire.AdapterInfo, wire.AdapterInfoReply, 0xE8},
		{wire.GlobalAddress, wire.GlobalAddressReply, 0x28},
		{wire.RowEventDataList, wire.RowEventDataReply, 4},
	}

	for _, test := range tests {
		reply := exchange(t, startGame(t), test.request, nil)

		if reply.Command != test.reply || len(reply.Payload) != test.size {
			t.Errorf("command %#x: reply = %#x with %d bytes, want %#x with %d", test.request, reply.Command, len(reply.Payload), test.reply, test.size)
		}
	}
}

func httpPostPayload(url, body string) []byte {
	payload := make([]byte, 0xC59+len(body))
	copy(payload, url)
	binary.LittleEndian.PutUint32(payload[0x848:], 5)
	binary.LittleEndian.PutUint32(payload[0x854:], uint32(len(body)))
	copy(payload[0xC59:], body)
	return payload
}

func TestServe_HTTPPost_RunsTheHandlerAndRepliesWithItsBody(t *testing.T) {
	config := testConfig
	config.HTTP = http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.ParseForm()
		fmt.Fprintf(w, "%s|%s", r.URL.Path, r.PostForm.Get("version"))
	})
	game, service := net.Pipe()
	defer game.Close()
	go nesys.Serve(service, config)

	start := exchange(t, game, wire.HTTPPost, httpPostPayload("http://lobby/sendversion", "tenpo_id=1&version=1.2604"))
	reply, err := wire.ReadFrame(game)

	if start.Command != wire.HTTPStart || err != nil || reply.Command != wire.HTTPReply {
		t.Fatalf("replies = %+v, %+v, %v", start, reply, err)
	}
	if got := string(reply.Payload[0x414:]); got != "/sendversion|1.2604" ||
		binary.LittleEndian.Uint32(reply.Payload[0x404:]) != 200 || binary.LittleEndian.Uint32(reply.Payload[0x40C:]) != 5 {
		t.Fatalf("reply body = %q, payload head = % X", got, reply.Payload[0x400:0x414])
	}
}

func TestServe_HTTPPostWithoutHandler_RepliesWithAnErrorCode(t *testing.T) {
	game := startGame(t)

	exchange(t, game, wire.HTTPPost, httpPostPayload("http://lobby/login", ""))
	reply, err := wire.ReadFrame(game)

	if err != nil || binary.LittleEndian.Uint32(reply.Payload[0x408:]) == 0 {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
}

func TestServe_ClientEnd_RepliesAndStops(t *testing.T) {
	game, service := net.Pipe()
	done := make(chan error, 1)
	go func() { done <- nesys.Serve(service, testConfig) }()

	reply := exchange(t, game, wire.ClientEnd, nil)

	if reply.Command != wire.ClientEndReply || <-done != nil {
		t.Fatalf("reply = %+v", reply)
	}
}

type fakeCards struct{}

func (fakeCards) Select(cardID string) string { return "1\n1\n" + cardID + "\n0\n" }

func TestServe_CardSelect_RepliesWithTheCardsBody(t *testing.T) {
	config := testConfig
	config.Cards = fakeCards{}
	game, service := net.Pipe()
	t.Cleanup(func() { game.Close() })
	go nesys.Serve(service, config)
	payload := make([]byte, 24)
	copy(payload, "7020392000000001")

	reply := exchange(t, game, wire.CardSelect, payload)

	want := wire.CardReply{Status: 1, Body: "1\n1\n7020392000000001\n0\n"}.Encode()
	if reply.Command != wire.CardSelectReply || string(reply.Payload) != string(want) {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_GameStart_RepliesWithGameStatus(t *testing.T) {
	game := startGame(t)

	reply := exchange(t, game, wire.GameStart, make([]byte, 7))

	if reply.Command != wire.GameStatusReply || binary.LittleEndian.Uint32(reply.Payload) != 1 {
		t.Fatalf("reply = %+v", reply)
	}
}
