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
	// found" and every other value as a failure (0x7a no server, 0x7c room full, 0x7d room lost).
	errOK        = 1
	errEmptyList = 0x7e

	// joinSearch is the value of the room field in a search for a room to join. The value 2
	// asks for a new room.
	joinSearch = "5"

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

// roomTTL is how long a room stays listed after it was created.
const roomTTL = 10 * time.Minute

// room is a room a cabinet has created, and the game server that hosts it.
type room struct {
	id         uint32
	server     netip.AddrPort
	mode       string
	rule       string
	campaign   byte
	chapter    byte
	difficulty byte
	created    time.Time
}

// Master tracks the rooms that cabinets have created and answers their queries.
type Master struct {
	mu         sync.Mutex
	rooms      []room
	nextRoomID uint32
	server     netip.AddrPort
	now        func() time.Time
}

// NewMaster returns a master with no rooms. New rooms are hosted by the game server at server,
// or by the asking machine's own listen server on port 27015 when server is the zero value.
func NewMaster(server netip.AddrPort) *Master {
	return &Master{nextRoomID: 1, server: server, now: time.Now}
}

// Reply returns the answer to a query from the machine at from, or nil when it needs none. A
// reply carries no type byte: the game reads it according to the type of its last request, so
// a list reply is the big-endian sequence number, the error code, the number of entries, the
// index of this entry and the entry itself.
//
// A query for a new room (room 2) creates a room and returns it. A search for a room to join
// (room 5) returns the newest listed room that fits the query's mode, rule and difficulty, or an
// empty list.
func (m *Master) Reply(from netip.Addr, query Query) []byte {
	if query.Type != listQuery {
		return nil
	}

	sequence, err := strconv.ParseUint(query.Fields["seqNo"], 10, 32)
	if err != nil || !from.Unmap().Is4() {
		return nil
	}
	reply := binary.BigEndian.AppendUint32(nil, uint32(sequence))

	var found room
	if query.Fields["room"] == joinSearch {
		var ok bool
		if found, ok = m.find(query); !ok {
			return append(reply, errEmptyList)
		}
	} else {
		found = m.create(from.Unmap(), query)
	}

	reply = append(reply, errOK, 1, 0)
	reply = append(reply, found.server.Addr().AsSlice()...)
	reply = binary.BigEndian.AppendUint16(reply, found.server.Port()+advertisedPortOffset)
	reply = binary.BigEndian.AppendUint32(reply, found.id)
	return append(reply, found.campaign, found.chapter, found.difficulty)
}

func (m *Master) create(host netip.Addr, query Query) room {
	m.mu.Lock()
	defer m.mu.Unlock()

	server := m.server
	if !server.IsValid() {
		server = netip.AddrPortFrom(host, gamePort)
	}
	created := room{
		id:         m.nextRoomID,
		server:     server,
		mode:       query.Fields["mode"],
		rule:       query.Fields["rule"],
		campaign:   max(number(query.Fields["stage"]), 1),
		chapter:    max(number(query.Fields["chapter"]), 1),
		difficulty: number(query.Fields["difficulty"]) + 1,
		created:    m.now(),
	}
	m.nextRoomID++
	m.rooms = append(m.rooms, created)
	return created
}

// number parses a small decimal query field, reading anything else as 0.
func number(text string) byte {
	value, _ := strconv.ParseUint(text, 10, 8)
	return byte(value)
}

func (m *Master) find(query Query) (room, bool) {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.rooms = slices.DeleteFunc(m.rooms, func(r room) bool { return m.now().Sub(r.created) > roomTTL })
	for _, candidate := range slices.Backward(m.rooms) {
		if candidate.mode == query.Fields["mode"] && candidate.rule == query.Fields["rule"] && candidate.difficulty == number(query.Fields["difficulty"])+1 {
			return candidate, true
		}
	}
	return room{}, false
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
			if reply := m.Reply(udp.AddrPort().Addr(), query); reply != nil {
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
