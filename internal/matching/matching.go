// Package matching implements the arcade matching master: the UDP service the game queries to
// find rooms to join.
package matching

import (
	"bytes"
	"encoding/binary"
	"log/slog"
	"net"
	"strconv"
	"strings"
	"sync"
)

const (
	listQuery = '2'

	// Reply error codes. The window that sent a query acts on them: errEmptyList offers to
	// create a room, and errRoomCreated enters the state that recruits members for the room in
	// the reply.
	errRoomCreated = 0x7c
	errEmptyList   = 0x7e

	// joinInProgress is the value of the room field in a search for a game already started.
	joinInProgress = "5"

	// advertisedPort is the port a room is advertised on: the game port 27015 plus 1000.
	advertisedPort = 28015

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

// Master tracks the rooms that cabinets have created and answers their queries.
type Master struct {
	mu         sync.Mutex
	nextRoomID uint32
}

// NewMaster returns a master with no rooms.
func NewMaster() *Master { return &Master{nextRoomID: 1} }

// Reply returns the answer to a query from the machine at from, or nil when it needs none. A
// reply carries no type byte: the game reads it according to the type of its last request, so
// a list reply is the big-endian sequence number, the error code, the number of entries, the
// index of this entry and the entry itself.
//
// A search for a game in progress finds nothing. Any other list query is a request for a new
// room, which the master creates on the asking machine's listen server.
func (m *Master) Reply(from net.IP, query Query) []byte {
	if query.Type != listQuery {
		return nil
	}

	sequence, err := strconv.ParseUint(query.Fields["seqNo"], 10, 32)
	if err != nil {
		return nil
	}
	reply := binary.BigEndian.AppendUint32(nil, uint32(sequence))

	host := from.To4()
	if query.Fields["room"] == joinInProgress || host == nil {
		return append(reply, errEmptyList)
	}

	reply = append(reply, errRoomCreated, 1, 0)
	reply = append(reply, host...)
	reply = binary.BigEndian.AppendUint16(reply, advertisedPort)
	reply = binary.BigEndian.AppendUint32(reply, m.newRoomID())
	return append(reply, 0, 0, 0)
}

func (m *Master) newRoomID() uint32 {
	m.mu.Lock()
	defer m.mu.Unlock()
	id := m.nextRoomID
	m.nextRoomID++
	return id
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
			if reply := m.Reply(udp.IP, query); reply != nil {
				conn.WriteTo(reply, from)
			}
		}
	}
}
