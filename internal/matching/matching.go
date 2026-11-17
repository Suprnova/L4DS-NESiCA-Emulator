// Package matching implements the arcade matching master: the UDP service the game queries to
// find rooms to join.
package matching

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log/slog"
	"net"
	"net/netip"
	"slices"
	"strconv"
	"strings"
	"sync"
	"time"
)

const (
	listQuery = '2'

	// Reply error codes. The matching window treats errOK as success, errEmptyList as "no room
	// found" and every other value as a failure (errNoServer, 0x7c room full, errRoomLost).
	errOK        = 1
	errNoServer  = 0x7a
	errRoomLost  = 0x7d
	errEmptyList = 0x7e

	// Values of the room field. For national rooms, a search for a room to join that finds one is
	// followed by a request to enter it, with the same fields. In a store or clan lobby, opening
	// the lobby sends lobbyEntry, refreshing the list sends joinSearch, joining someone else's room
	// sends lobbyJoin with its id, and creating a room, or waiting in a joined one for its stage,
	// sends lobbyRoom with the room's id.
	lobbyJoin  = "1"
	newRoom    = "2"
	lobbyEntry = "2"
	enterRoom  = "3"
	lobbyRoom  = "4"
	joinSearch = "5"

	// Values of the rule field.
	ruleClan  = 2
	ruleStore = 3

	// Values of the mode field.
	modeNationalEvent = 2
	modeVersus        = 7

	// Values of the room field in versus. Joining sends versusJoin, the window then polls the
	// room with versusPoll while it gathers players, and asks for the match with versusDecide
	// once the room's gathering time is up.
	versusJoin   = "3"
	versusDecide = "4"
	versusPoll   = "5"

	// coopPlayers is how many players a co-op room holds, and versusPlayers a versus room, four
	// a side.
	coopPlayers   = 4
	versusPlayers = 8

	// gamePort is the port of a cabinet's own listen server.
	gamePort = 27015

	// advertisedPortOffset is added to a room's game port in the room entry; the game subtracts
	// it again before connecting.
	advertisedPortOffset = 1000

	maxDatagram = 2048
)

// Query is a datagram the game sent: a type byte followed by NUL-separated key\value items.
type Query struct {
	Type   byte
	Fields map[string]string
}

// ParseQuery decodes a datagram. Items without a backslash are ignored, and the 0xFF
// terminator ends the datagram.
func ParseQuery(datagram []byte) (Query, bool) {
	if len(datagram) == 0 {
		return Query{}, false
	}

	body, _, _ := bytes.Cut(datagram[1:], []byte{0xFF})
	query := Query{Type: datagram[0], Fields: map[string]string{}}
	for item := range bytes.SplitSeq(body, []byte{0}) {
		if key, value, found := strings.Cut(string(item), `\`); found {
			query.Fields[key] = value
		}
	}
	return query, true
}

func (q Query) number(key string) int {
	value, _ := strconv.Atoi(q.Fields[key])
	return value
}

func (q Query) player() int64 {
	id, _ := strconv.ParseInt(q.Fields["player_id"], 10, 64)
	return id
}

// difficulty is the query's difficulty as a room entry carries it: 1 is easy, 2 normal. It is
// at least 1, because the game drops a room with difficulty 0.
func (q Query) difficulty() byte { return byte(max(q.number("difficulty"), 1)) }

// lobby reports whether rooms for the query's mode and rule are picked from a lobby's room list,
// as store and clan rooms are.
func (q Query) lobby() bool {
	mode, rule := q.number("mode"), q.number("rule")
	return (rule == ruleClan || rule == ruleStore) && mode >= 1 && mode <= 4
}

// versus reports whether the query is for a national versus room.
func (q Query) versus() bool { return q.number("mode") == modeVersus }

// nationalEvent reports whether the query is for a national event room, which a player enters
// without a search.
func (q Query) nationalEvent() bool { return q.number("mode") == modeNationalEvent && !q.lobby() }

// longEntry reports whether the game expects the long room entry, with members and status, in
// replies to the query. The test mirrors the engine's (0x102115a0): lobby rooms, national events
// and versus.
func (q Query) longEntry() bool {
	mode := q.number("mode")
	return q.lobby() || mode == 2 || mode >= 7 && mode <= 10
}

// Values of a lobby room's permission byte, which the room panel shows as an icon.
const (
	permissionGathering = 0    // "gathering" (待ち合わせ中)
	permissionAnyone    = 1    // "anyone may join" (誰でも参加OK)
	permissionNone      = 0xff // no icon: the room has no stage yet
)

// Values of a versus room's permission byte. The versus window starts the game when the reply is
// a single room with versusDecided and at least two members.
const (
	versusDecided = 0
	versusOpen    = 1
)

const (
	// roomTTL is how long a room stays listed after it was created.
	roomTTL = 10 * time.Minute

	// lobbyIdle is how long a lobby room without a server stays listed after its last query
	// from a member; the lobby's own timer is 180 seconds.
	lobbyIdle = 210 * time.Second

	// versusGathering is how long a versus room gathers players. The game counts down to the
	// room's expiry, and gives a room without one 60 seconds itself.
	versusGathering = 60 * time.Second

	// gatherIdle is how long a member of a versus room without a server stays in it after their
	// last query. The game polls about every 10 seconds, and gives up on a request after 15.
	gatherIdle = 30 * time.Second
)

// room is a room a cabinet has created, the game server that hosts it, and its members, the
// first of whom owns it. A lobby room gets its server when its owner creates it for play; a
// versus room gathers players until it expires, and gets its server and its members' slots when
// it is decided.
type room struct {
	id         uint32
	server     netip.AddrPort
	mode       string
	rule       string
	event      string
	tenpo      string
	machine    string
	password   string
	campaign   byte
	chapter    byte
	difficulty byte
	permission byte
	members    []int64
	created    time.Time
	lastSeen   time.Time
	expires    time.Time
	slots      []byte
	seen       map[int64]time.Time
}

// versus reports whether r is a versus room.
func (r *room) versus() bool { return !r.expires.IsZero() }

// listing returns the entries a versus room is listed as: one per member while it gathers
// players, which the game counts, and the room alone once it is decided.
func (r *room) listing() []*room {
	if r.slots != nil {
		return []*room{r}
	}
	return slices.Repeat([]*room{r}, len(r.members))
}

// Hosts provides the game servers that host new rooms.
type Hosts interface {
	// Host returns the address of a server for a new room, or false when none is available.
	Host() (netip.AddrPort, bool)
	// Hosting reports whether the server at address still hosts its room.
	Hosting(address netip.AddrPort) bool
}

// FixedHost hosts every room on one game server, such as a dedicated server started by hand.
type FixedHost netip.AddrPort

// Host returns the server's address.
func (h FixedHost) Host() (netip.AddrPort, bool) { return netip.AddrPort(h), true }

// Hosting reports true: the server is assumed to keep running.
func (h FixedHost) Hosting(netip.AddrPort) bool { return true }

// Config describes where a [Master] gets room servers and the names its room lists show.
type Config struct {
	// Hosts hosts new rooms. When nil, a room is hosted by the asking machine's own listen
	// server on port 27015.
	Hosts Hosts
	// PlayerName returns a player's name for room lists. When nil, lists show player ids.
	PlayerName func(playerID int64) string
	// StoreName is the store shown for every member in room lists.
	StoreName string
	// VersusClass returns a player's versus class id, counted from 1, for versus rooms. When
	// nil, every player is class 1.
	VersusClass func(playerID int64) int
}

// Master tracks the rooms that cabinets have created and answers their queries.
type Master struct {
	config     Config
	mu         sync.Mutex
	rooms      []*room
	nextRoomID uint32
	now        func() time.Time
}

// NewMaster returns a master with no rooms.
func NewMaster(config Config) *Master {
	return &Master{config: config, nextRoomID: 1, now: time.Now}
}

// Reply returns the datagrams that answer a query from the machine at from, or nil when it needs
// no answer. A reply carries no type byte: the game reads it according to the type of its last
// request, so each datagram is the big-endian sequence number, the error code, the number of
// entries, the index of this entry and the entry itself.
//
// National queries: a query for a new room (room 2) creates a room owned by the asking player
// and returns it, or answers "no server" when no server is available; a resent query returns the
// room the player already owns. A search (room 5) returns the newest room with the same mode,
// rule and difficulty, or an empty list. A request to enter a room (room 3) adds the asking
// player to the room a search would find and returns it, or answers "room lost".
//
// Versus queries gather up to eight players for a versus unit and stage. Joining (room 3) adds
// the asking player to the open room for the unit, or opens one that gathers players for 60
// seconds. While a room gathers, every reply lists it once per member, and the game shows the
// count; polling (room 5) returns the player's room. When the time is up the game asks for the
// match (room 4): the room is decided, with a server and each member's side, and listed once. A
// full room is decided at once. Polling or asking without a room answers "room lost".
//
// National event queries enter a room without a search: a request to enter (room 3) adds the
// asking player to the newest room for the same event and stage with fewer than four players, or
// creates one, and returns it; it answers "no server" when no server is available.
//
// Store and clan lobby queries (see [Query.lobby]) list every lobby room with the same mode and
// rule, and for store rooms the same store and matching group, newest first. Opening the lobby
// (room 2) also gives the asking player a room of their own, without a server, unless they are
// already in one. Creating or joining a room (room 4, with the room's id) gives the room a server
// when its owner asks, adds anyone else as a member, takes the stage from the owner's query once
// it has one (with the owner's gathering setting, which that query carries in progress: 1 lets
// anyone join), and returns the room; it answers "room lost" when the room is gone and "no server"
// when no server is available. Joining someone else's room (room 1, with its id) makes the
// asking player a member in the same way.
//
// Other room values get no answer.
func (m *Master) Reply(from netip.Addr, query Query) [][]byte {
	if query.Type != listQuery {
		return nil
	}

	sequence, err := strconv.ParseUint(query.Fields["seqNo"], 10, 32)
	if err != nil || !from.Unmap().Is4() {
		return nil
	}
	header := binary.BigEndian.AppendUint32(nil, uint32(sequence))

	m.mu.Lock()
	defer m.mu.Unlock()
	m.dropEnded()
	if query.lobby() {
		m.touch(query)
	}

	var found []*room
	failure := byte(0)
	switch request := query.Fields["room"]; {
	case query.versus() && (request == versusJoin || request == versusPoll || request == versusDecide):
		var r *room
		if r, failure = m.playVersus(from.Unmap(), query); r != nil {
			found = r.listing()
		}
	case query.lobby() && request == lobbyEntry:
		m.enterLobby(query)
		found = m.search(query)
	case request == joinSearch:
		if found = m.search(query); len(found) == 0 && !query.lobby() {
			failure = errEmptyList
		}
	case query.lobby() && (request == lobbyRoom || request == lobbyJoin):
		var r *room
		if r, failure = m.playLobbyRoom(from.Unmap(), query); r != nil {
			found = []*room{r}
		}
	case query.nationalEvent() && request == enterRoom:
		var r *room
		if r, failure = m.joinEvent(from.Unmap(), query); r != nil {
			found = []*room{r}
		}
	case !query.lobby() && request == enterRoom:
		if r := m.enter(query); r != nil {
			found = []*room{r}
		} else {
			failure = errRoomLost
		}
	case !query.lobby() && request == newRoom:
		if r := m.create(from.Unmap(), query); r != nil {
			found = []*room{r}
		} else {
			failure = errNoServer
		}
	default:
		slog.Warn("unknown matching room request", "room", request, "rule", query.Fields["rule"])
		return nil
	}
	if failure != 0 {
		return [][]byte{append(header, failure)}
	}
	if len(found) == 0 {
		return [][]byte{append(header, errOK, 0, 0)}
	}

	found = found[:min(len(found), 255)]
	replies := make([][]byte, len(found))
	for i, r := range found {
		reply := append(slices.Clone(header), errOK, byte(len(found)), byte(i))
		replies[i] = m.appendEntry(reply, r, query)
	}
	return replies
}

// Leave takes a player out of a room, as the game reports when the player leaves matching, and
// drops the room once nobody is left in it.
func (m *Master) Leave(playerID int64, roomID uint32) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rooms = slices.DeleteFunc(m.rooms, func(r *room) bool {
		if r.id != roomID {
			return false
		}
		r.members = slices.DeleteFunc(r.members, func(member int64) bool { return member == playerID })
		return len(r.members) == 0
	})
}

func (m *Master) dropEnded() {
	now := m.now()
	m.rooms = slices.DeleteFunc(m.rooms, func(r *room) bool {
		if now.Sub(r.created) > roomTTL {
			return true
		}
		if r.versus() && !r.server.IsValid() {
			r.members = slices.DeleteFunc(r.members, func(member int64) bool { return now.Sub(r.seen[member]) > gatherIdle })
			return len(r.members) == 0
		}
		if !r.server.IsValid() {
			return now.Sub(r.lastSeen) > lobbyIdle
		}
		return m.config.Hosts != nil && !m.config.Hosts.Hosting(r.server)
	})
}

// touch marks the lobby rooms the asking player is in as seen, and reports whether there are any.
func (m *Master) touch(query Query) bool {
	inRoom := false
	for _, r := range m.rooms {
		if slices.Contains(r.members, query.player()) && r.rule == query.Fields["rule"] {
			r.lastSeen = m.now()
			inRoom = true
		}
	}
	return inRoom
}

// enterLobby gives the asking player a lobby room of their own unless they are already in one.
func (m *Master) enterLobby(query Query) {
	if !m.touch(query) {
		m.rooms = append(m.rooms, m.newRoom(netip.AddrPort{}, query))
	}
}

// playLobbyRoom handles a request to create, join or wait in the lobby room with the query's
// room_id: the owner's request gives it a server and its stage, anyone else's makes them a member
// and drops the empty lobby room of their own. It returns the room, or the failure code to answer with.
func (m *Master) playLobbyRoom(host netip.Addr, query Query) (*room, byte) {
	id, _ := strconv.ParseUint(query.Fields["room_id"], 10, 32)
	i := slices.IndexFunc(m.rooms, func(r *room) bool { return uint64(r.id) == id })
	if i < 0 {
		return nil, errRoomLost
	}
	target, player := m.rooms[i], query.player()
	target.lastSeen = m.now()

	if target.members[0] != player {
		if !slices.Contains(target.members, player) {
			target.members = append(target.members, player)
			m.rooms = slices.DeleteFunc(m.rooms, func(r *room) bool {
				return r != target && len(r.members) == 1 && r.members[0] == player && !r.server.IsValid()
			})
		}
		return target, 0
	}

	if !target.server.IsValid() {
		server, ok := m.host(host)
		if !ok {
			return nil, errNoServer
		}
		target.server = server
	}
	if stage := query.number("stage"); stage > 0 {
		target.campaign = byte(stage)
		target.chapter = byte(max(query.number("chapter"), 1))
		target.difficulty = query.difficulty()
		target.permission = permissionGathering
		if query.number("progress") == permissionAnyone {
			target.permission = permissionAnyone
		}
	}
	return target, 0
}

// host returns a server for a new room: one from Hosts, or the asking machine's listen server.
func (m *Master) host(asker netip.Addr) (netip.AddrPort, bool) {
	if m.config.Hosts == nil {
		return netip.AddrPortFrom(asker, gamePort), true
	}
	return m.config.Hosts.Host()
}

// newRoom returns a room owned by the asking player with the query's settings. A national room
// gets its stage from the query; a lobby room has none until its owner picks one.
func (m *Master) newRoom(server netip.AddrPort, query Query) *room {
	created := &room{
		id:         m.nextRoomID,
		server:     server,
		mode:       query.Fields["mode"],
		rule:       query.Fields["rule"],
		event:      query.Fields["event"],
		tenpo:      query.Fields["tenpo"],
		machine:    query.Fields["machine"],
		password:   query.Fields["password"],
		members:    []int64{query.player()},
		created:    m.now(),
		lastSeen:   m.now(),
		permission: permissionNone,
	}
	if !query.lobby() {
		created.campaign, created.chapter, created.difficulty = query.stage()
	}
	m.nextRoomID++
	return created
}

// stage returns the campaign, chapter and difficulty of the query as a room entry carries them,
// none of them 0.
func (q Query) stage() (campaign, chapter, difficulty byte) {
	return byte(max(q.number("stage"), 1)), byte(max(q.number("chapter"), 1)), q.difficulty()
}

// sameStage reports whether r has the query's mode, rule, campaign, chapter and difficulty.
func (r *room) sameStage(query Query) bool {
	campaign, chapter, difficulty := query.stage()
	return r.mode == query.Fields["mode"] && r.rule == query.Fields["rule"] &&
		r.campaign == campaign && r.chapter == chapter && r.difficulty == difficulty
}

func (m *Master) create(host netip.Addr, query Query) *room {
	player := query.player()
	if query.Fields["resend"] == "1" {
		for _, r := range slices.Backward(m.rooms) {
			if r.members[0] == player {
				return r
			}
		}
	}

	server, ok := m.host(host)
	if !ok {
		return nil
	}
	created := m.newRoom(server, query)
	m.rooms = append(m.rooms, created)
	return created
}

// search returns the rooms that fit query, newest first.
func (m *Master) search(query Query) []*room {
	var found []*room
	for _, r := range slices.Backward(m.rooms) {
		if r.mode != query.Fields["mode"] || r.rule != query.Fields["rule"] {
			continue
		}
		if !query.lobby() {
			if r.difficulty == query.difficulty() {
				return []*room{r}
			}
			continue
		}
		if query.number("rule") == ruleStore && (r.tenpo != query.Fields["tenpo"] || r.machine != query.Fields["machine"]) {
			continue
		}
		found = append(found, r)
	}
	return found
}

// joinEvent puts the asking player in the newest national event room for the query's event and
// stage that has a free place, or in a new one. It returns the room, or the failure code to
// answer with.
func (m *Master) joinEvent(host netip.Addr, query Query) (*room, byte) {
	player := query.player()
	for _, r := range slices.Backward(m.rooms) {
		if !r.sameStage(query) || r.event != query.Fields["event"] {
			continue
		}
		if slices.Contains(r.members, player) {
			return r, 0
		}
		if len(r.members) < coopPlayers {
			r.members = append(r.members, player)
			return r, 0
		}
	}

	server, ok := m.host(host)
	if !ok {
		return nil, errNoServer
	}
	created := m.newRoom(server, query)
	m.rooms = append(m.rooms, created)
	return created, 0
}

// playVersus handles a versus query: joining puts the asking player in a gathering room (see
// [Master.openVersusRoom]), and decides it once it is full; polling returns the player's room;
// asking for the match decides the player's room unless it already is. It returns the room, or
// the failure code to answer with.
func (m *Master) playVersus(host netip.Addr, query Query) (*room, byte) {
	player := query.player()
	var r *room
	switch query.Fields["room"] {
	case versusJoin:
		r = m.openVersusRoom(query)
		if !slices.Contains(r.members, player) {
			r.members = append(r.members, player)
		}
		if len(r.members) == versusPlayers {
			m.decide(host, r)
		}
	case versusDecide:
		if r = m.versusRoomOf(player); r != nil && r.slots == nil {
			if failure := m.decide(host, r); failure != 0 {
				return nil, failure
			}
		}
	default:
		r = m.versusRoomOf(player)
	}
	if r == nil {
		return nil, errRoomLost
	}
	r.seen[player] = m.now()
	return r, 0
}

// openVersusRoom returns the gathering versus room for the query's unit and stage that the
// asking player is in, or else the newest one with a free place, or else a new one.
func (m *Master) openVersusRoom(query Query) *room {
	var open *room
	for _, r := range slices.Backward(m.rooms) {
		if !r.versus() || r.slots != nil || !m.now().Before(r.expires) || !r.sameStage(query) || r.event != query.Fields["event"] {
			continue
		}
		if slices.Contains(r.members, query.player()) {
			return r
		}
		if open == nil && len(r.members) < versusPlayers {
			open = r
		}
	}
	if open == nil {
		open = m.newRoom(netip.AddrPort{}, query)
		open.permission = versusOpen
		open.expires = m.now().Add(versusGathering)
		open.seen = map[int64]time.Time{}
		m.rooms = append(m.rooms, open)
	}
	return open
}

// versusRoomOf returns the newest versus room the player is in, or nil.
func (m *Master) versusRoomOf(player int64) *room {
	for _, r := range slices.Backward(m.rooms) {
		if r.versus() && slices.Contains(r.members, player) {
			return r
		}
	}
	return nil
}

// decide closes a gathering versus room and gives each member a slot, alternating between the
// survivors (slots 0 to 3) and the infected (4 to 7). A room with players on both sides gets a
// server; a room left with one member gets none, and the game then reports that not enough
// players were found. It returns the failure code to answer with, or 0.
func (m *Master) decide(host netip.Addr, r *room) byte {
	if len(r.members) > 1 {
		server, ok := m.host(host)
		if !ok {
			return errNoServer
		}
		r.server = server
	}
	r.permission = versusDecided
	r.slots = make([]byte, len(r.members))
	for i := range r.slots {
		r.slots[i] = byte(i/2 + i%2*versusPlayers/2)
	}
	return 0
}

func (m *Master) enter(query Query) *room {
	var target *room
	if id, _ := strconv.ParseUint(query.Fields["room_id"], 10, 32); id != 0 {
		if i := slices.IndexFunc(m.rooms, func(r *room) bool { return uint64(r.id) == id }); i >= 0 {
			target = m.rooms[i]
		}
	} else if found := m.search(query); len(found) > 0 {
		target = found[0]
	}
	if target != nil && !slices.Contains(target.members, query.player()) {
		target.members = append(target.members, query.player())
	}
	return target
}

// appendEntry appends r's room entry: the address, port, id, campaign, chapter and difficulty,
// and when the query expects the long entry the members, their titles, stores and clans, six status bytes (class
// condition, permission, progress, whether the asking player owns the room, the asking player's
// member index and the owner's), the password, two eight-byte arrays (class ids and numbers) and
// an expiry date. In a decided versus room, the asking player's index is their slot, and the
// arrays hold each member's versus class and slot plus 1; the game puts slots 1 to 4 on the
// survivor side. A versus room's expiry is when it stops gathering, in Unix seconds. Fields with
// unknown meaning are zero or empty.
func (m *Master) appendEntry(b []byte, r *room, query Query) []byte {
	if r.server.IsValid() {
		b = append(b, r.server.Addr().AsSlice()...)
		b = binary.BigEndian.AppendUint16(b, r.server.Port()+advertisedPortOffset)
	} else {
		b = append(b, 0, 0, 0, 0, 0, 0)
	}
	b = binary.BigEndian.AppendUint32(b, r.id)
	b = append(b, r.campaign, r.chapter, r.difficulty)
	if !query.longEntry() {
		return b
	}

	names := make([]string, len(r.members))
	stores := make([]string, len(r.members))
	for i, member := range r.members {
		names[i] = strconv.FormatInt(member, 10)
		if m.config.PlayerName != nil {
			names[i] = m.config.PlayerName(member)
		}
		stores[i] = m.config.StoreName
	}
	asker := query.player()
	me := 0xff
	if i := slices.Index(r.members, asker); i >= 0 {
		me = i
		if r.slots != nil {
			me = int(r.slots[i])
		}
	}
	var ownerIsMe byte
	if r.members[0] == asker {
		ownerIsMe = 1
	}
	classes, numbers := make([]byte, versusPlayers), make([]byte, versusPlayers)
	if r.slots != nil {
		for i, member := range r.members[:min(len(r.members), versusPlayers)] {
			classes[i] = m.versusClass(member)
			numbers[i] = r.slots[i] + 1
		}
	}
	expire := "0"
	if r.versus() {
		expire = strconv.FormatInt(r.expires.Unix(), 10)
	}

	b = appendField(b, strings.Join(names, ","), 0x100)
	b = appendField(b, "", 0x190)
	b = appendField(b, strings.Join(stores, ","), 0x190)
	b = appendField(b, "", 0x190)
	b = append(b, 0, r.permission, 0, ownerIsMe, byte(me), 0)
	b = appendField(b, r.password, 4)
	b = append(b, classes...)
	b = append(b, numbers...)
	return appendField(b, expire, 0x20)
}

// versusClass returns a player's versus class id for room entries, 1 when unknown.
func (m *Master) versusClass(player int64) byte {
	if m.config.VersusClass == nil {
		return 1
	}
	return byte(max(m.config.VersusClass(player), 1))
}

// appendField appends text as a NUL-padded field of size bytes, cut to leave room for a NUL.
func appendField(b []byte, text string, size int) []byte {
	field := make([]byte, size)
	copy(field[:size-1], text)
	return append(b, field...)
}

// Serve answers queries arriving on conn until it is closed.
func (m *Master) Serve(conn net.PacketConn) {
	buffer := make([]byte, maxDatagram)
	for {
		n, from, err := conn.ReadFrom(buffer)
		if err != nil {
			return
		}

		query, ok := ParseQuery(buffer[:n])
		if !ok {
			continue
		}
		slog.Info("matching query", "from", from.String(), "type", string(query.Type), "fields", query.Fields)

		if udp, ok := from.(*net.UDPAddr); ok {
			for _, reply := range m.Reply(udp.AddrPort().Addr(), query) {
				conn.WriteTo(reply, from)
			}
		}
	}
}

// ServeGameServer logs every datagram arriving on conn, the port rooms are advertised on, until
// it is closed.
func ServeGameServer(conn net.PacketConn) {
	buffer := make([]byte, maxDatagram)
	for {
		n, from, err := conn.ReadFrom(buffer)
		if err != nil {
			return
		}
		slog.Info("game server datagram", "from", from.String(), "bytes", n, "hex", fmt.Sprintf("%x", buffer[:n]), "text", fmt.Sprintf("%q", buffer[:n]))
	}
}
