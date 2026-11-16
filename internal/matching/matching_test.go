package matching_test

import (
	"net/netip"
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

var lan = netip.AddrFrom4([4]byte{192, 168, 1, 20})

func searchQuery(sequence, room string) matching.Query {
	return matching.Query{Type: '2', Fields: map[string]string{"seqNo": sequence, "room": room, "mode": "1", "rule": "1", "difficulty": "1", "stage": "1", "chapter": "1"}}
}

func TestMaster_JoinSearchWithNoRooms_AnswersWithAnEmptyList(t *testing.T) {
	got := matching.NewMaster(nil).Reply(lan, searchQuery("258", "5"))

	want := []byte{0, 0, 1, 2, 0x7e}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_NewRoomQuery_CreatesARoomOnTheAskingMachine(t *testing.T) {
	master := matching.NewMaster(nil)

	first := master.Reply(lan, searchQuery("13", "2"))
	second := master.Reply(lan, searchQuery("14", "2"))

	want := []byte{0, 0, 0, 13, 1, 1, 0, 192, 168, 1, 20, 0x6d, 0x6f, 0, 0, 0, 1, 1, 1, 2}
	if !slices.Equal(first, want) {
		t.Fatalf("Reply() = %v, want %v", first, want)
	}
	if second[len(second)-4] != 2 {
		t.Fatalf("second room id byte = %d, want 2", second[len(second)-4])
	}
}

func TestMaster_NewRoomQueryWithAGameServer_CreatesTheRoomOnTheGameServer(t *testing.T) {
	master := matching.NewMaster(matching.FixedHost(netip.MustParseAddrPort("10.0.0.5:27020")))

	got := master.Reply(lan, searchQuery("13", "2"))

	want := []byte{0, 0, 0, 13, 1, 1, 0, 10, 0, 0, 5, 0x6d, 0x74, 0, 0, 0, 1, 1, 1, 2}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

// fakeHosts hands out its servers in order and hosts the ones not in stopped.
type fakeHosts struct {
	free    []netip.AddrPort
	stopped map[netip.AddrPort]bool
}

func (h *fakeHosts) Host() (netip.AddrPort, bool) {
	if len(h.free) == 0 {
		return netip.AddrPort{}, false
	}
	server := h.free[0]
	h.free = h.free[1:]
	return server, true
}

func (h *fakeHosts) Hosting(address netip.AddrPort) bool { return !h.stopped[address] }

func TestMaster_NewRoomQueryWithNoServerAvailable_AnswersNoServer(t *testing.T) {
	master := matching.NewMaster(&fakeHosts{})

	got := master.Reply(lan, searchQuery("13", "2"))

	want := []byte{0, 0, 0, 13, 0x7a}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_JoinSearch_SkipsRoomsWhoseServerStopped(t *testing.T) {
	first, second := netip.MustParseAddrPort("127.0.0.2:27020"), netip.MustParseAddrPort("127.0.0.2:27021")
	hosts := &fakeHosts{free: []netip.AddrPort{first, second}, stopped: map[netip.AddrPort]bool{}}
	master := matching.NewMaster(hosts)
	master.Reply(lan, searchQuery("1", "2"))
	master.Reply(lan, searchQuery("2", "2"))
	hosts.stopped[second] = true

	got := master.Reply(lan, searchQuery("3", "5"))

	want := []byte{0, 0, 0, 3, 1, 1, 0, 127, 0, 0, 2, 0x6d, 0x74, 0, 0, 0, 1, 1, 1, 2}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_JoinSearchAfterCreation_FindsTheNewestMatchingRoom(t *testing.T) {
	master := matching.NewMaster(nil)
	master.Reply(lan, searchQuery("1", "2"))
	master.Reply(lan, searchQuery("2", "2"))

	got := master.Reply(netip.AddrFrom4([4]byte{192, 168, 1, 21}), searchQuery("3", "5"))

	want := []byte{0, 0, 0, 3, 1, 1, 0, 192, 168, 1, 20, 0x6d, 0x6f, 0, 0, 0, 2, 1, 1, 2}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_JoinSearchWithADifferentDifficulty_FindsNothing(t *testing.T) {
	master := matching.NewMaster(nil)
	master.Reply(lan, searchQuery("1", "2"))
	other := searchQuery("2", "5")
	other.Fields["difficulty"] = "3"

	got := master.Reply(lan, other)

	if got[4] != 0x7e {
		t.Fatalf("Reply() = %v, want an empty list", got)
	}
}

func TestMaster_OtherQueries_NeedNoAnswer(t *testing.T) {
	got := matching.NewMaster(nil).Reply(netip.AddrFrom4([4]byte{127, 0, 0, 1}), matching.Query{Type: '0', Fields: map[string]string{}})

	if got != nil {
		t.Fatalf("Reply() = %v, want nil", got)
	}
}
