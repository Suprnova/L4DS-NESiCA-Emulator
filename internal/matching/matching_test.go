package matching_test

import (
	"net"
	"slices"
	"testing"

	"l4ds-nesica-emulator/internal/matching"
)

func TestParseQuery_ReadsTheTypeAndItems(t *testing.T) {
	datagram := []byte("2\x00" + `seqNo\7` + "\x00" + `version\1.2604` + "\x00" + `player_id\42` + "\xff")

	query, ok := matching.ParseQuery(datagram)

	if !ok || query.Type != '2' || query.Fields["seqNo"] != "7" || query.Fields["version"] != "1.2604" || query.Fields["player_id"] != "42" {
		t.Fatalf("ParseQuery() = %+v, %v", query, ok)
	}
}

func TestParseQuery_EmptyDatagram_IsRejected(t *testing.T) {
	_, ok := matching.ParseQuery(nil)

	if ok {
		t.Fatal("ParseQuery() ok = true")
	}
}

func TestMaster_JoinInProgressSearch_AnswersWithAnEmptyList(t *testing.T) {
	query := matching.Query{Type: '2', Fields: map[string]string{"seqNo": "258", "room": "5"}}

	got := matching.NewMaster().Reply(net.IPv4(127, 0, 0, 1), query)

	want := []byte{0, 0, 1, 2, 0x7e}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_NewRoomQuery_CreatesARoomOnTheAskingMachine(t *testing.T) {
	master := matching.NewMaster()
	query := matching.Query{Type: '2', Fields: map[string]string{"seqNo": "13", "room": "2"}}

	first := master.Reply(net.IPv4(192, 168, 1, 20), query)
	second := master.Reply(net.IPv4(192, 168, 1, 20), query)

	want := []byte{0, 0, 0, 13, 1, 1, 0, 192, 168, 1, 20, 0x6d, 0x6f, 0, 0, 0, 1, 0, 0, 0}
	if !slices.Equal(first, want) {
		t.Fatalf("Reply() = %v, want %v", first, want)
	}
	if second[len(second)-4] != 2 {
		t.Fatalf("second room id byte = %d, want 2", second[len(second)-4])
	}
}

func TestMaster_OtherQueries_NeedNoAnswer(t *testing.T) {
	got := matching.NewMaster().Reply(net.IPv4(127, 0, 0, 1), matching.Query{Type: '0', Fields: map[string]string{}})

	if got != nil {
		t.Fatalf("Reply() = %v, want nil", got)
	}
}
