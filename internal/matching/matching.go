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

	// Values of the room field. A search for a room to join that finds one is followed by a
	// request to enter it, with the same fields.
	newRoom    = "2"
	enterRoom  = "3"
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

// roomTTL is how long a room stays listed after it was created.
const roomTTL = 10 * time.Minute

// room is a room a cabinet has created, the game server that hosts it, and its members, the
// first of whom owns it.
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
// A query for a new room (room 2) creates a room owned by the asking player and returns it, or
// answers "no server" when no server is available; a resent query returns the room the player
// already owns. A search (room 5) returns the rooms that fit the query, or an empty list: for
// national rooms the newest one with the same mode, rule and difficulty, for store and clan
// rooms every one with the same mode and rule (and for store rooms the same store and matching
// group). A request to enter a room (room 3) adds the asking player to the room with the query's
// room_id, or to the room a search would find, and returns it; it answers "room lost" when the
// room is gone. Other room values get no answer.
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
	switch query.Fields["room"] {
	case joinSearch:
		if found = m.search(query); len(found) == 0 {
			return [][]byte{append(header, errEmptyList)}
		}
	case enterRoom:
		r := m.enter(query)
		if r == nil {
			return [][]byte{append(header, errRoomLost)}
		}
		found = []*room{r}
	case newRoom:
		r := m.create(from.Unmap(), query)
		if r == nil {
			return [][]byte{append(header, errNoServer)}
		}
		found = []*room{r}
	default:
		slog.Warn("unknown matching room request", "room", query.Fields["room"])
		return nil
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
	m.rooms = slices.DeleteFunc(m.rooms, func(r *room) bool {
		return m.now().Sub(r.created) > roomTTL || m.config.Hosts != nil && !m.config.Hosts.Hosting(r.server)
	})
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

	server := netip.AddrPortFrom(host, gamePort)
	if m.config.Hosts != nil {
		var ok bool
		if server, ok = m.config.Hosts.Host(); !ok {
			return nil
		}
	}
	created := &room{
		id:         m.nextRoomID,
		server:     server,
		mode:       query.Fields["mode"],
		rule:       query.Fields["rule"],
		tenpo:      query.Fields["tenpo"],
		machine:    query.Fields["machine"],
		password:   query.Fields["password"],
		campaign:   byte(max(query.number("stage"), 1)),
		chapter:    byte(max(query.number("chapter"), 1)),
		difficulty: byte(query.number("difficulty") + 1),
		members:    []int64{player},
		created:    m.now(),
	}
	m.nextRoomID++
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
	b = append(b, r.server.Addr().AsSlice()...)
	b = binary.BigEndian.AppendUint16(b, r.server.Port()+advertisedPortOffset)
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
