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
	// the lobby sends lobbyEntry, refreshing the list sends joinSearch, and creating or joining a
	// room sends lobbyRoom with the room's id.
	newRoom    = "2"
	lobbyEntry = "2"
	enterRoom  = "3"
	lobbyRoom  = "4"
	joinSearch = "5"

	// Values of the rule field.
	ruleClan  = 2
	ruleStore = 3

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

// lobby reports whether rooms for the query's mode and rule are picked from a lobby's room list,
// as store and clan rooms are. The game then expects the long room entry; the test mirrors the
// engine's (0x102115a0).
func (q Query) lobby() bool {
	mode, rule := q.number("mode"), q.number("rule")
	if rule == ruleClan || rule == ruleStore {
		return mode >= 1 && mode <= 4
	}
	return mode == 2 || mode >= 7 && mode <= 10
}

const (
	// roomTTL is how long a room stays listed after it was created.
	roomTTL = 10 * time.Minute

	// lobbyIdle is how long a lobby room without a server stays listed after its last query
	// from a member; the lobby's own timer is 180 seconds.
	lobbyIdle = 210 * time.Second
)

// room is a room a cabinet has created, the game server that hosts it, and its members, the
// first of whom owns it. A lobby room gets its server when its owner creates it for play.
type room struct {
	id         uint32
	server     netip.AddrPort
	mode       string
	rule       string
	tenpo      string
	machine    string
	password   string
	campaign   byte
	chapter    byte
	difficulty byte
	members    []int64
	created    time.Time
	lastSeen   time.Time
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
// Store and clan lobby queries (see [Query.lobby]) list every lobby room with the same mode and
// rule, and for store rooms the same store and matching group, newest first. Opening the lobby
// (room 2) also gives the asking player a room of their own, without a server, unless they are
// already in one. Creating or joining a room (room 4, with the room's id) gives the room a server
// when its owner asks, adds anyone else as a member, takes the stage from the owner's query once
// it has one, and returns the room; it answers "room lost" when the room is gone and "no server"
// when no server is available.
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

	var found []*room
	failure := byte(0)
	switch request := query.Fields["room"]; {
	case query.lobby() && request == lobbyEntry:
		m.enterLobby(query)
		found = m.search(query)
	case request == joinSearch:
		if found = m.search(query); len(found) == 0 && !query.lobby() {
			failure = errEmptyList
		}
	case query.lobby() && request == lobbyRoom:
		var r *room
		if r, failure = m.playLobbyRoom(from.Unmap(), query); r != nil {
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

func (m *Master) dropEnded() {
	now := m.now()
	m.rooms = slices.DeleteFunc(m.rooms, func(r *room) bool {
		if now.Sub(r.created) > roomTTL {
			return true
		}
		if !r.server.IsValid() {
			return now.Sub(r.lastSeen) > lobbyIdle
		}
		return m.config.Hosts != nil && !m.config.Hosts.Hosting(r.server)
	})
}

// enterLobby gives the asking player a lobby room of their own unless they are already in one,
// and marks their rooms as seen.
func (m *Master) enterLobby(query Query) {
	player := query.player()
	inRoom := false
	for _, r := range m.rooms {
		if slices.Contains(r.members, player) && r.rule == query.Fields["rule"] {
			r.lastSeen = m.now()
			inRoom = true
		}
	}
	if !inRoom {
		m.rooms = append(m.rooms, m.newRoom(netip.AddrPort{}, query))
	}
}

// playLobbyRoom handles a request to create or join the lobby room with the query's room_id: the
// owner's request gives it a server and its stage, anyone else's makes them a member and drops
// the empty lobby room of their own. It returns the room, or the failure code to answer with.
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
		target.difficulty = byte(query.number("difficulty") + 1)
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
		id:       m.nextRoomID,
		server:   server,
		mode:     query.Fields["mode"],
		rule:     query.Fields["rule"],
		tenpo:    query.Fields["tenpo"],
		machine:  query.Fields["machine"],
		password: query.Fields["password"],
		members:  []int64{query.player()},
		created:  m.now(),
		lastSeen: m.now(),
	}
	if !query.lobby() {
		created.campaign = byte(max(query.number("stage"), 1))
		created.chapter = byte(max(query.number("chapter"), 1))
		created.difficulty = byte(query.number("difficulty") + 1)
	}
	m.nextRoomID++
	return created
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
			if r.difficulty == byte(query.number("difficulty")+1) {
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
// and for a lobby query the members, their titles, stores and clans, six status bytes (class
// condition, permission, progress, whether the asking player owns the room, the asking player's
// member index and the owner's), the password, two eight-byte arrays (class ids and numbers) and
// an expiry date. Fields with unknown meaning are zero or empty.
func (m *Master) appendEntry(b []byte, r *room, query Query) []byte {
	if r.server.IsValid() {
		b = append(b, r.server.Addr().AsSlice()...)
		b = binary.BigEndian.AppendUint16(b, r.server.Port()+advertisedPortOffset)
	} else {
		b = append(b, 0, 0, 0, 0, 0, 0)
	}
	b = binary.BigEndian.AppendUint32(b, r.id)
	b = append(b, r.campaign, r.chapter, r.difficulty)
	if !query.lobby() {
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
	me := slices.Index(r.members, asker)
	if me < 0 {
		me = 0xff
	}
	var ownerIsMe byte
	if r.members[0] == asker {
		ownerIsMe = 1
	}

	b = appendField(b, strings.Join(names, ","), 0x100)
	b = appendField(b, "", 0x190)
	b = appendField(b, strings.Join(stores, ","), 0x190)
	b = appendField(b, "", 0x190)
	b = append(b, 0, 0, 0, ownerIsMe, byte(me), 0)
	b = appendField(b, r.password, 4)
	b = append(b, make([]byte, 16)...)
	return appendField(b, "0", 0x20)
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
