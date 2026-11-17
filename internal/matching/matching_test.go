package matching_test

import (
	"bytes"
	"net/netip"
	"slices"
	"strconv"
	"testing"
	"time"

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
	return matching.Query{Type: '2', Fields: map[string]string{"seqNo": sequence, "room": room, "mode": "1", "rule": "1", "difficulty": "1", "stage": "1", "chapter": "1", "player_id": "1"}}
}

// storeQuery is a store co-op query (mode 1, rule 3) from player in store 1, matching group 0,
// with the stage and difficulty still unselected, as the game sends them.
func storeQuery(sequence, room, player string) matching.Query {
	return matching.Query{Type: '2', Fields: map[string]string{"seqNo": sequence, "room": room, "mode": "1", "rule": "3", "difficulty": "0", "stage": "0", "chapter": "0", "player_id": player, "tenpo": "1", "machine": "0", "room_id": "0", "password": ""}}
}

// single returns the only datagram of a reply.
func single(t *testing.T, replies [][]byte) []byte {
	t.Helper()
	if len(replies) != 1 {
		t.Fatalf("Reply() returned %d datagrams, want 1", len(replies))
	}
	return replies[0]
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

func servers(ports ...uint16) *fakeHosts {
	h := &fakeHosts{stopped: map[netip.AddrPort]bool{}}
	for _, port := range ports {
		h.free = append(h.free, netip.AddrPortFrom(netip.AddrFrom4([4]byte{127, 0, 0, 2}), port))
	}
	return h
}

func TestMaster_JoinSearchWithNoRooms_AnswersWithAnEmptyList(t *testing.T) {
	got := single(t, matching.NewMaster(matching.Config{}).Reply(lan, searchQuery("258", "5")))

	want := []byte{0, 0, 1, 2, 0x7e}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_NewRoomQuery_CreatesARoomOnTheAskingMachine(t *testing.T) {
	master := matching.NewMaster(matching.Config{})

	first := single(t, master.Reply(lan, searchQuery("13", "2")))
	second := single(t, master.Reply(lan, searchQuery("14", "2")))

	want := []byte{0, 0, 0, 13, 1, 1, 0, 192, 168, 1, 20, 0x6d, 0x6f, 0, 0, 0, 1, 1, 1, 1}
	if !slices.Equal(first, want) {
		t.Fatalf("Reply() = %v, want %v", first, want)
	}
	if second[len(second)-4] != 2 {
		t.Fatalf("second room id byte = %d, want 2", second[len(second)-4])
	}
}

func TestMaster_NewRoomQueryWithAGameServer_CreatesTheRoomOnTheGameServer(t *testing.T) {
	master := matching.NewMaster(matching.Config{Hosts: matching.FixedHost(netip.MustParseAddrPort("10.0.0.5:27020"))})

	got := single(t, master.Reply(lan, searchQuery("13", "2")))

	want := []byte{0, 0, 0, 13, 1, 1, 0, 10, 0, 0, 5, 0x6d, 0x74, 0, 0, 0, 1, 1, 1, 1}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_NewRoomQueryWithNoServerAvailable_AnswersNoServer(t *testing.T) {
	master := matching.NewMaster(matching.Config{Hosts: servers()})

	got := single(t, master.Reply(lan, searchQuery("13", "2")))

	want := []byte{0, 0, 0, 13, 0x7a}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_ResentNewRoomQuery_ReturnsThePlayersRoom(t *testing.T) {
	hosts := servers(27020, 27021)
	master := matching.NewMaster(matching.Config{Hosts: hosts})
	master.Reply(lan, searchQuery("13", "2"))
	resent := searchQuery("14", "2")
	resent.Fields["resend"] = "1"

	got := single(t, master.Reply(lan, resent))

	if got[4] != 1 || got[len(got)-4] != 1 || len(hosts.free) != 1 {
		t.Fatalf("Reply() = %v; servers left = %d, want room 1 and 1 server left", got, len(hosts.free))
	}
}

func TestMaster_JoinSearch_SkipsRoomsWhoseServerStopped(t *testing.T) {
	hosts := servers(27020, 27021)
	master := matching.NewMaster(matching.Config{Hosts: hosts})
	master.Reply(lan, searchQuery("1", "2"))
	master.Reply(lan, searchQuery("2", "2"))
	hosts.stopped[netip.MustParseAddrPort("127.0.0.2:27021")] = true

	got := single(t, master.Reply(lan, searchQuery("3", "5")))

	want := []byte{0, 0, 0, 3, 1, 1, 0, 127, 0, 0, 2, 0x6d, 0x74, 0, 0, 0, 1, 1, 1, 1}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_EnterRoomAfterASearch_ReturnsTheFoundRoomWithoutCreatingOne(t *testing.T) {
	hosts := servers(27020, 27021)
	master := matching.NewMaster(matching.Config{Hosts: hosts})
	master.Reply(lan, searchQuery("1", "2"))
	master.Reply(lan, searchQuery("2", "5"))

	got := single(t, master.Reply(lan, searchQuery("3", "3")))

	want := []byte{0, 0, 0, 3, 1, 1, 0, 127, 0, 0, 2, 0x6d, 0x74, 0, 0, 0, 1, 1, 1, 1}
	if !slices.Equal(got, want) || len(hosts.free) != 1 {
		t.Fatalf("Reply() = %v, want %v; servers left = %d, want 1", got, want, len(hosts.free))
	}
}

func TestMaster_EnterRoomWithNoRoom_AnswersRoomLost(t *testing.T) {
	got := single(t, matching.NewMaster(matching.Config{}).Reply(lan, searchQuery("3", "3")))

	want := []byte{0, 0, 0, 3, 0x7d}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_UnknownRoomRequest_NeedsNoAnswer(t *testing.T) {
	got := matching.NewMaster(matching.Config{}).Reply(lan, searchQuery("3", "9"))

	if got != nil {
		t.Fatalf("Reply() = %v, want nil", got)
	}
}

func TestMaster_JoinSearchAfterCreation_FindsTheNewestMatchingRoom(t *testing.T) {
	master := matching.NewMaster(matching.Config{})
	master.Reply(lan, searchQuery("1", "2"))
	master.Reply(lan, searchQuery("2", "2"))

	got := single(t, master.Reply(netip.AddrFrom4([4]byte{192, 168, 1, 21}), searchQuery("3", "5")))

	want := []byte{0, 0, 0, 3, 1, 1, 0, 192, 168, 1, 20, 0x6d, 0x6f, 0, 0, 0, 2, 1, 1, 1}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_JoinSearchWithADifferentDifficulty_FindsNothing(t *testing.T) {
	master := matching.NewMaster(matching.Config{})
	master.Reply(lan, searchQuery("1", "2"))
	other := searchQuery("2", "5")
	other.Fields["difficulty"] = "3"

	got := single(t, master.Reply(lan, other))

	if got[4] != 0x7e {
		t.Fatalf("Reply() = %v, want an empty list", got)
	}
}

func TestMaster_OtherQueries_NeedNoAnswer(t *testing.T) {
	got := matching.NewMaster(matching.Config{}).Reply(netip.AddrFrom4([4]byte{127, 0, 0, 1}), matching.Query{Type: '0', Fields: map[string]string{}})

	if got != nil {
		t.Fatalf("Reply() = %v, want nil", got)
	}
}

// Long room entry offsets after the 7-byte reply header and the 13-byte room header.
const (
	playersAt = 7 + 13
	storesAt  = playersAt + 0x100 + 0x190
	statusAt  = storesAt + 0x190 + 0x190
	entrySize = 7 + 0x5f7
)

func field(entry []byte, at int) string {
	text, _, _ := bytes.Cut(entry[at:], []byte{0})
	return string(text)
}

func storeMaster() *matching.Master {
	names := map[int64]string{1: "Boba", 3: "Ellis"}
	return matching.NewMaster(matching.Config{
		Hosts:      servers(27020, 27021, 27022),
		PlayerName: func(id int64) string { return names[id] },
		StoreName:  "L4DS Emulator",
	})
}

func storeRoom(sequence, player, roomID string) matching.Query {
	query := storeQuery(sequence, "4", player)
	query.Fields["room_id"] = roomID
	return query
}

func TestMaster_OpeningTheStoreLobby_ListsARoomOfTheirOwnWithoutAServer(t *testing.T) {
	hosts := servers(27020)
	master := matching.NewMaster(matching.Config{Hosts: hosts, PlayerName: func(int64) string { return "Boba" }, StoreName: "L4DS Emulator"})

	got := single(t, master.Reply(lan, storeQuery("11", "2", "1")))

	status := got[statusAt : statusAt+6]
	if len(got) != entrySize || !slices.Equal(got[7:13], make([]byte, 6)) || field(got, playersAt) != "Boba" || field(got, storesAt) != "L4DS Emulator" || !slices.Equal(status, []byte{0, 0xff, 0, 1, 0, 0}) || len(hosts.free) != 1 {
		t.Fatalf("entry size %d, address %v, players %q, stores %q, status %v, servers left %d", len(got), got[7:13], field(got, playersAt), field(got, storesAt), status, len(hosts.free))
	}
}

func TestMaster_OwnerCreatesTheirLobbyRoom_GivesItAServerButNoStage(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, storeQuery("11", "2", "1"))

	got := single(t, master.Reply(lan, storeRoom("12", "1", "1")))

	if !slices.Equal(got[7:13], []byte{127, 0, 0, 2, 0x6d, 0x74}) || !slices.Equal(got[17:20], []byte{0, 0, 0}) {
		t.Fatalf("address %v, campaign, chapter and difficulty %v", got[7:13], got[17:20])
	}
}

func TestMaster_OwnerSendsTheChosenStage_SetsTheRoomsStage(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, storeQuery("11", "2", "1"))
	master.Reply(lan, storeRoom("12", "1", "1"))
	chosen := storeRoom("13", "1", "1")
	chosen.Fields["stage"], chosen.Fields["chapter"], chosen.Fields["difficulty"] = "2", "3", "1"

	chosen.Fields["progress"] = "1"

	got := single(t, master.Reply(lan, chosen))

	if !slices.Equal(got[7:13], []byte{127, 0, 0, 2, 0x6d, 0x74}) || !slices.Equal(got[17:20], []byte{2, 3, 1}) || got[statusAt+1] != 1 {
		t.Fatalf("address %v, campaign, chapter and difficulty %v, permission %d", got[7:13], got[17:20], got[statusAt+1])
	}
}

func TestMaster_OwnerChoosesGathering_ShowsTheGatheringPermission(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, storeQuery("11", "2", "1"))
	before := single(t, master.Reply(lan, storeRoom("12", "1", "1")))
	chosen := storeRoom("13", "1", "1")
	chosen.Fields["stage"], chosen.Fields["chapter"], chosen.Fields["difficulty"], chosen.Fields["progress"] = "1", "1", "1", "0"

	got := single(t, master.Reply(lan, chosen))

	if before[statusAt+1] != 0xff || got[statusAt+1] != 0 {
		t.Fatalf("permission before a stage %d, after %d", before[statusAt+1], got[statusAt+1])
	}
}

func TestMaster_PlayerJoinsALobbyRoom_BecomesAMemberAndLosesTheirOwnRoom(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, storeQuery("11", "2", "1"))
	master.Reply(lan, storeQuery("12", "2", "3"))
	master.Reply(lan, storeRoom("13", "1", "1"))

	join := storeRoom("14", "3", "1")
	join.Fields["room"] = "1"

	got := single(t, master.Reply(lan, join))
	listed := master.Reply(lan, storeQuery("15", "5", "3"))

	status := got[statusAt : statusAt+6]
	if field(got, playersAt) != "Boba,Ellis" || field(got, storesAt) != "L4DS Emulator,L4DS Emulator" || !slices.Equal(status, []byte{0, 0xff, 0, 0, 1, 0}) || len(listed) != 1 {
		t.Fatalf("players %q, stores %q, status %v, rooms listed %d", field(got, playersAt), field(got, storesAt), status, len(listed))
	}
}

func TestMaster_JoiningALobbyRoomThatIsGone_AnswersRoomLost(t *testing.T) {
	got := single(t, storeMaster().Reply(lan, storeRoom("11", "3", "7")))

	want := []byte{0, 0, 0, 11, 0x7d}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_StoreLobby_ListsTheStoresRoomsNewestFirst(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, storeQuery("11", "2", "1"))
	master.Reply(lan, storeQuery("12", "2", "3"))
	otherStore := storeQuery("13", "2", "4")
	otherStore.Fields["tenpo"] = "2"
	master.Reply(lan, otherStore)

	got := master.Reply(lan, storeQuery("14", "5", "1"))

	if len(got) != 2 || got[0][5] != 2 || got[0][6] != 0 || got[0][16] != 2 || got[1][6] != 1 || got[1][16] != 1 {
		t.Fatalf("Reply() returned %d datagrams", len(got))
	}
}

// eventQuery is a national event query (mode 2, rule 1) for event 1 on the first chapter of
// campaign 1 at normal difficulty.
func eventQuery(sequence, room, player string) matching.Query {
	return matching.Query{Type: '2', Fields: map[string]string{"seqNo": sequence, "room": room, "mode": "2", "rule": "1", "difficulty": "2", "stage": "1", "chapter": "1", "event": "1", "player_id": player, "progress": "20", "room_id": "0"}}
}

func TestMaster_EnteringANationalEventWithNoRoom_CreatesOneOnAServer(t *testing.T) {
	got := single(t, storeMaster().Reply(lan, eventQuery("13", "3", "1")))

	if len(got) != entrySize || !slices.Equal(got[7:20], []byte{127, 0, 0, 2, 0x6d, 0x74, 0, 0, 0, 1, 1, 1, 2}) || field(got, playersAt) != "Boba" {
		t.Fatalf("entry size %d, room %v, players %q", len(got), got[7:20], field(got, playersAt))
	}
}

func TestMaster_EnteringANationalEvent_JoinsTheEventsRoom(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, eventQuery("13", "3", "1"))

	got := single(t, master.Reply(lan, eventQuery("14", "3", "3")))

	if got[16] != 1 || field(got, playersAt) != "Boba,Ellis" {
		t.Fatalf("room id %d, players %q", got[16], field(got, playersAt))
	}
}

func TestMaster_EnteringAnotherNationalEvent_CreatesItsOwnRoom(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, eventQuery("13", "3", "1"))
	other := eventQuery("14", "3", "3")
	other.Fields["event"] = "2"

	got := single(t, master.Reply(lan, other))

	if got[16] != 2 || field(got, playersAt) != "Ellis" {
		t.Fatalf("room id %d, players %q", got[16], field(got, playersAt))
	}
}

func TestMaster_EnteringAFullNationalEventRoom_CreatesAnotherRoom(t *testing.T) {
	master := storeMaster()
	for player := range 4 {
		master.Reply(lan, eventQuery("13", "3", strconv.Itoa(player+1)))
	}

	got := single(t, master.Reply(lan, eventQuery("14", "3", "5")))

	if got[16] != 2 {
		t.Fatalf("room id %d, want 2", got[16])
	}
}

func TestMaster_ResentNationalEventEntry_ReturnsTheSameRoom(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, eventQuery("13", "3", "1"))

	got := single(t, master.Reply(lan, eventQuery("14", "3", "1")))

	if got[16] != 1 || field(got, playersAt) != "Boba" {
		t.Fatalf("room id %d, players %q", got[16], field(got, playersAt))
	}
}

func TestMaster_EnteringANationalEventWithNoServerAvailable_AnswersNoServer(t *testing.T) {
	got := single(t, matching.NewMaster(matching.Config{Hosts: servers()}).Reply(lan, eventQuery("13", "3", "1")))

	want := []byte{0, 0, 0, 13, 0x7a}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_RefreshingAnEmptyStoreLobby_AnswersAnEmptySuccess(t *testing.T) {
	got := single(t, storeMaster().Reply(lan, storeQuery("11", "5", "1")))

	want := []byte{0, 0, 0, 11, 1, 0, 0}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_MemberWaitingInARoom_GetsItsServerAndStageOnceTheOwnerPicksOne(t *testing.T) {
	master := storeMaster()
	master.Reply(lan, storeQuery("11", "2", "1"))
	master.Reply(lan, storeQuery("12", "2", "3"))
	join := storeRoom("13", "3", "1")
	join.Fields["room"] = "1"
	master.Reply(lan, join)
	master.Reply(lan, storeRoom("14", "1", "1"))
	chosen := storeRoom("15", "1", "1")
	chosen.Fields["stage"], chosen.Fields["chapter"], chosen.Fields["difficulty"] = "1", "1", "1"
	master.Reply(lan, chosen)

	got := single(t, master.Reply(lan, storeRoom("16", "3", "1")))

	if !slices.Equal(got[7:13], []byte{127, 0, 0, 2, 0x6d, 0x74}) || !slices.Equal(got[17:20], []byte{1, 1, 1}) || field(got, playersAt) != "Boba,Ellis" {
		t.Fatalf("address %v, campaign, chapter and difficulty %v, players %q", got[7:13], got[17:20], field(got, playersAt))
	}
}

// Versus entry offsets after the status bytes.
const (
	classesAt = statusAt + 6 + 4
	numbersAt = classesAt + 8
	expireAt  = numbersAt + 8
)

// versusQuery is a versus query (mode 7, rule 1) for unit 1 on the first chapter of campaign 1
// at normal difficulty.
func versusQuery(sequence, room, player string) matching.Query {
	return matching.Query{Type: '2', Fields: map[string]string{"seqNo": sequence, "room": room, "mode": "7", "rule": "1", "difficulty": "2", "stage": "1", "chapter": "1", "event": "1", "player_id": player, "room_id": "0"}}
}

// versusMaster is a master with three servers, two named players and a clock the test moves.
func versusMaster() (*matching.Master, *time.Time) {
	now := time.Unix(1_800_000_000, 0)
	master := storeMaster()
	master.SetClock(func() time.Time { return now })
	return master, &now
}

func TestMaster_JoiningVersusWithNobodyGathering_OpensARoomThatGathersForAMinute(t *testing.T) {
	master, now := versusMaster()

	got := single(t, master.Reply(lan, versusQuery("11", "3", "1")))

	if len(got) != entrySize || !slices.Equal(got[7:13], make([]byte, 6)) || got[statusAt+1] != 1 || field(got, expireAt) != strconv.FormatInt(now.Unix()+60, 10) {
		t.Fatalf("entry size %d, address %v, permission %d, expire %q", len(got), got[7:13], got[statusAt+1], field(got, expireAt))
	}
}

func TestMaster_JoiningAGatheringVersusRoom_ListsItOncePerMember(t *testing.T) {
	master, _ := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))

	got := master.Reply(lan, versusQuery("12", "3", "3"))

	if len(got) != 2 || got[1][5] != 2 || got[1][6] != 1 || field(got[0], playersAt) != "Boba,Ellis" {
		t.Fatalf("Reply() returned %d datagrams, players %q", len(got), field(got[0], playersAt))
	}
}

func TestMaster_AskingForTheVersusMatch_DecidesTheRoomWithASideForEachMember(t *testing.T) {
	master, _ := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))
	master.Reply(lan, versusQuery("12", "3", "3"))

	got := single(t, master.Reply(lan, versusQuery("13", "4", "3")))

	status := got[statusAt : statusAt+6]
	if !slices.Equal(got[7:13], []byte{127, 0, 0, 2, 0x6d, 0x74}) || !slices.Equal(status, []byte{0, 0, 0, 0, 4, 0}) || !slices.Equal(got[classesAt:numbersAt], []byte{1, 1, 0, 0, 0, 0, 0, 0}) || !slices.Equal(got[numbersAt:expireAt], []byte{1, 5, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("address %v, status %v, classes %v, numbers %v", got[7:13], status, got[classesAt:numbersAt], got[numbersAt:expireAt])
	}
}

func TestMaster_PollingADecidedVersusRoom_ReturnsItOnceWithThePlayersSlot(t *testing.T) {
	master, _ := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))
	master.Reply(lan, versusQuery("12", "3", "3"))
	master.Reply(lan, versusQuery("13", "4", "3"))

	got := single(t, master.Reply(lan, versusQuery("14", "5", "1")))

	if got[statusAt+1] != 0 || got[statusAt+4] != 0 || field(got, playersAt) != "Boba,Ellis" {
		t.Fatalf("permission %d, me %d, players %q", got[statusAt+1], got[statusAt+4], field(got, playersAt))
	}
}

func TestMaster_AskingForTheVersusMatchAlone_DecidesTheRoomWithoutAServer(t *testing.T) {
	hosts := servers(27020)
	master := matching.NewMaster(matching.Config{Hosts: hosts})
	master.Reply(lan, versusQuery("11", "3", "1"))

	got := single(t, master.Reply(lan, versusQuery("12", "4", "1")))

	if got[statusAt+1] != 0 || !slices.Equal(got[7:13], make([]byte, 6)) || len(hosts.free) != 1 {
		t.Fatalf("permission %d, address %v, servers left %d", got[statusAt+1], got[7:13], len(hosts.free))
	}
}

func TestMaster_EighthVersusPlayer_DecidesTheRoomAtOnce(t *testing.T) {
	master, _ := versusMaster()
	for player := range 7 {
		master.Reply(lan, versusQuery("11", "3", strconv.Itoa(player+1)))
	}

	got := single(t, master.Reply(lan, versusQuery("12", "3", "8")))

	if got[statusAt+1] != 0 || !slices.Equal(got[numbersAt:expireAt], []byte{1, 5, 2, 6, 3, 7, 4, 8}) {
		t.Fatalf("permission %d, numbers %v", got[statusAt+1], got[numbersAt:expireAt])
	}
}

func TestMaster_JoiningVersusAfterTheRoomExpired_OpensANewRoom(t *testing.T) {
	master, now := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))
	*now = now.Add(61 * time.Second)

	got := single(t, master.Reply(lan, versusQuery("12", "3", "3")))

	if got[16] != 2 || field(got, playersAt) != "Ellis" {
		t.Fatalf("room id %d, players %q", got[16], field(got, playersAt))
	}
}

func TestMaster_VersusMemberWhoStopsPolling_LeavesTheGatheringRoom(t *testing.T) {
	master, now := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))
	*now = now.Add(20 * time.Second)
	master.Reply(lan, versusQuery("12", "3", "3"))
	*now = now.Add(11 * time.Second)

	got := master.Reply(lan, versusQuery("13", "5", "3"))

	if len(got) != 1 || field(got[0], playersAt) != "Ellis" {
		t.Fatalf("Reply() returned %d datagrams, players %q", len(got), field(got[0], playersAt))
	}
}

func TestMaster_VersusMemberPollingEveryElevenSeconds_StaysInTheRoom(t *testing.T) {
	master, now := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))
	*now = now.Add(9 * time.Second)
	master.Reply(lan, versusQuery("12", "5", "1"))
	*now = now.Add(11 * time.Second)

	got := single(t, master.Reply(lan, versusQuery("13", "5", "1")))

	if got[4] != 1 || field(got, playersAt) != "Boba" {
		t.Fatalf("Reply() = %v, players %q", got[:7], field(got, playersAt))
	}
}

func TestMaster_JoiningVersusAfterAMatch_OpensANewRoom(t *testing.T) {
	master, _ := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))
	master.Reply(lan, versusQuery("12", "3", "3"))
	master.Reply(lan, versusQuery("13", "4", "1"))

	got := single(t, master.Reply(lan, versusQuery("14", "3", "1")))

	if got[16] != 2 || got[statusAt+1] != 1 {
		t.Fatalf("room id %d, permission %d", got[16], got[statusAt+1])
	}
}

func TestMaster_PollingVersusWithoutARoom_AnswersRoomLost(t *testing.T) {
	master, _ := versusMaster()

	got := single(t, master.Reply(lan, versusQuery("11", "5", "1")))

	want := []byte{0, 0, 0, 11, 0x7d}
	if !slices.Equal(got, want) {
		t.Fatalf("Reply() = %v, want %v", got, want)
	}
}

func TestMaster_DecidedVersusRoom_ShowsEachMembersVersusClass(t *testing.T) {
	classes := map[int64]int{1: 3, 3: 5}
	master := matching.NewMaster(matching.Config{Hosts: servers(27020), VersusClass: func(id int64) int { return classes[id] }})
	master.Reply(lan, versusQuery("11", "3", "1"))
	master.Reply(lan, versusQuery("12", "3", "3"))

	got := single(t, master.Reply(lan, versusQuery("13", "4", "1")))

	if !slices.Equal(got[classesAt:numbersAt], []byte{3, 5, 0, 0, 0, 0, 0, 0}) {
		t.Fatalf("classes %v", got[classesAt:numbersAt])
	}
}

func TestMaster_PlayerWhoLeavesTheirVersusRoom_IsNoLongerListed(t *testing.T) {
	master, _ := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))
	master.Reply(lan, versusQuery("12", "3", "3"))

	master.Leave(1, 1)

	got := single(t, master.Reply(lan, versusQuery("13", "5", "3")))
	if field(got, playersAt) != "Ellis" {
		t.Fatalf("players %q, want Ellis", field(got, playersAt))
	}
}

func TestMaster_LastPlayerLeavingARoom_DropsIt(t *testing.T) {
	master, _ := versusMaster()
	master.Reply(lan, versusQuery("11", "3", "1"))

	master.Leave(1, 1)

	got := single(t, master.Reply(lan, versusQuery("12", "5", "1")))
	if got[4] != 0x7d {
		t.Fatalf("Reply() = %v, want room lost", got)
	}
}
