package nesys_test

import (
	"encoding/binary"
	"errors"
	"fmt"
	"net"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
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

// backend is a nesys.Backend made of fakes; post answers the tunnelled requests.
type backend struct {
	nesys.Cards
	nesys.Rankings
	post func(url string, body []byte) (int, []byte, error)
}

func (b backend) Post(url string, body []byte) (int, []byte, error) { return b.post(url, body) }

// serveWith serves config with b as its backend and returns the game's end of the pipe.
func serveWith(t *testing.T, b backend) net.Conn {
	t.Helper()
	config := testConfig
	config.Backend = b
	game, service := net.Pipe()
	t.Cleanup(func() { game.Close() })
	go nesys.Serve(service, config)
	return game
}

func TestServe_HTTPPost_RepliesWithTheBackendsReply(t *testing.T) {
	game := serveWith(t, backend{post: func(url string, body []byte) (int, []byte, error) {
		return 200, []byte(url + "|" + string(body)), nil
	}})

	start := exchange(t, game, wire.HTTPPost, httpPostPayload("http://lobby/sendversion", "version=1.2604"))
	reply, err := wire.ReadFrame(game)

	if start.Command != wire.HTTPStart || err != nil || reply.Command != wire.HTTPReply {
		t.Fatalf("replies = %+v, %+v, %v", start, reply, err)
	}
	if got := string(reply.Payload[0x414:]); got != "http://lobby/sendversion|version=1.2604" ||
		binary.LittleEndian.Uint32(reply.Payload[0x404:]) != 200 || binary.LittleEndian.Uint32(reply.Payload[0x40C:]) != 5 {
		t.Fatalf("reply body = %q, payload head = % X", got, reply.Payload[0x400:0x414])
	}
}

func TestServe_HTTPPostTheBackendCannotAnswer_RepliesWithAnErrorCode(t *testing.T) {
	game := serveWith(t, backend{post: func(string, []byte) (int, []byte, error) {
		return 0, nil, errors.New("server unreachable")
	}})

	exchange(t, game, wire.HTTPPost, httpPostPayload("http://lobby/login", ""))
	reply, err := wire.ReadFrame(game)

	if err != nil || binary.LittleEndian.Uint32(reply.Payload[0x408:]) == 0 {
		t.Fatalf("reply = %+v, %v", reply, err)
	}
}

func TestServe_HTTPPostWithoutBackend_RepliesWithAnErrorCode(t *testing.T) {
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

func (fakeCards) Select(cardID string) (wire.CardReply, error) {
	return wire.CardReply{Status: 1, Body: "1\n1\n" + cardID + "\n0\n"}, nil
}

func (fakeCards) ReissueTest(cardID string) (wire.CardReply, error) {
	return wire.CardReply{Status: 1, Body: "0\n1\n" + cardID + "\n"}, nil
}

func (fakeCards) Insert(cardID, data string) (string, error) {
	return "1\n1\n7\n", nil
}

func (fakeCards) Update(cardID string, dataType uint32, data string) error { return nil }

// updateRecorder is a card server that reports each card update on a channel.
type updateRecorder struct {
	fakeCards
	updates chan string
}

func (r updateRecorder) Update(cardID string, dataType uint32, data string) error {
	r.updates <- fmt.Sprintf("%s %d %s", cardID, dataType, data)
	return nil
}

func TestServe_CardUpdate_PassesTheDataToTheCards(t *testing.T) {
	cards := updateRecorder{updates: make(chan string, 1)}
	game := serveWith(t, backend{Cards: cards})
	payload := make([]byte, 32)
	copy(payload, "7020392000000001")
	binary.LittleEndian.PutUint32(payload[24:], 3)
	binary.LittleEndian.PutUint32(payload[28:], 8)
	payload = append(payload, "volume=2"...)

	reply := exchange(t, game, wire.CardUpdate, payload)

	want := wire.CardReply{Status: 1, Body: "1\n1\n"}.Encode()
	if got := <-cards.updates; got != "7020392000000001 3 volume=2" || reply.Command != wire.CardUpdateReply || string(reply.Payload) != string(want) {
		t.Fatalf("update = %q, reply = %+v", got, reply)
	}
}

func TestServe_CardSelect_RepliesWithTheCardsBody(t *testing.T) {
	game := serveWith(t, backend{Cards: fakeCards{}})
	payload := make([]byte, 24)
	copy(payload, "7020392000000001")

	reply := exchange(t, game, wire.CardSelect, payload)

	want := wire.CardReply{Status: 1, Body: "1\n1\n7020392000000001\n0\n"}.Encode()
	if reply.Command != wire.CardSelectReply || string(reply.Payload) != string(want) {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_GameLifecycle_RepliesWithGameStatus(t *testing.T) {
	for _, command := range []wire.Command{wire.GameStart, wire.GameEnd, wire.GameContinue} {
		game := startGame(t)

		reply := exchange(t, game, command, make([]byte, 7))

		if reply.Command != wire.GameStatusReply || binary.LittleEndian.Uint32(reply.Payload) != 1 {
			t.Fatalf("%#x reply = %+v", uint32(command), reply)
		}
	}
}

func TestServe_IncomeStart_RepliesWithPlayerAndStatus(t *testing.T) {
	game := startGame(t)

	reply := exchange(t, game, wire.IncomeStart, make([]byte, 27))

	if reply.Command != wire.IncomeStatusReply || len(reply.Payload) != 8 {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_Shopping_RepliesWithShoppingReply(t *testing.T) {
	game := startGame(t)

	reply := exchange(t, game, wire.Shopping, make([]byte, 27))

	if reply.Command != wire.ShoppingReply {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_CardUpdate_RepliesWithSuccess(t *testing.T) {
	game := startGame(t)

	reply := exchange(t, game, wire.CardUpdate, make([]byte, 126))

	want := wire.CardReply{Status: 1, Body: "1\n1\n"}.Encode()
	if reply.Command != wire.CardUpdateReply || string(reply.Payload) != string(want) {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_RankingData_RepliesWithAnEmptyRanking(t *testing.T) {
	game := startGame(t)
	request := []byte("\x0a\x00\x00\x00\x00\x00\x00\x00\x0b\x00\x00\x00tenpo_id=1\x00\x00\x00\x00\x00")

	reply := exchange(t, game, wire.RankingData, request)

	want := "\x01\x00\x00\x00\x06\x00\x00\x001\n1\n0\n"
	if reply.Command != wire.RankingDataReply || string(reply.Payload) != want {
		t.Fatalf("reply = %+v", reply)
	}
}

type fakeRankings map[[2]uint32]string

func (r fakeRankings) Ranking(kind, store uint32) (string, error) {
	return r[[2]uint32{kind, store}], nil
}

func TestServe_RankingData_RepliesWithTheRankingOfTheRequestedKindAndStore(t *testing.T) {
	game := serveWith(t, backend{Rankings: fakeRankings{{30, 2}: "1\n1\n1\nentry\n"}})
	request := []byte("\x1e\x00\x00\x00\x00\x00\x00\x00\x0b\x00\x00\x00tenpo_id=2\x00\x00\x00\x00\x00")

	reply := exchange(t, game, wire.RankingData, request)

	want := "\x01\x00\x00\x00\x0c\x00\x00\x001\n1\n1\nentry\n"
	if reply.Command != wire.RankingDataReply || string(reply.Payload) != want {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_IncomeEnd_RepliesWithPlayerAndStatus(t *testing.T) {
	game := startGame(t)

	reply := exchange(t, game, wire.IncomeEnd, make([]byte, 27))

	if reply.Command != wire.IncomeStatusReply || len(reply.Payload) != 8 {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_CardInsert_RepliesWithTheCardsBody(t *testing.T) {
	game := serveWith(t, backend{Cards: fakeCards{}})
	payload := make([]byte, 32)
	copy(payload, "7020392000000001")
	binary.LittleEndian.PutUint32(payload[28:], 5)
	payload = append(payload, "a=1,b"...)

	reply := exchange(t, game, wire.CardInsert, payload)

	want := wire.CardReply{Status: 1, Body: "1\n1\n7\n"}.Encode()
	if reply.Command != wire.CardInsertReply || string(reply.Payload) != string(want) {
		t.Fatalf("reply = %+v", reply)
	}
}

func TestServe_CardReissueTest_RepliesWithTheCardsReply(t *testing.T) {
	game := serveWith(t, backend{Cards: fakeCards{}})
	payload := make([]byte, 0x14)
	copy(payload, "7020392000000001")

	reply := exchange(t, game, wire.CardReissueTest, payload)

	want := wire.CardReply{Status: 1, Body: "0\n1\n7020392000000001\n"}.Encode()
	if reply.Command != wire.CardReissueTestReply || string(reply.Payload) != string(want) {
		t.Fatalf("reply = %+v", reply)
	}
}
