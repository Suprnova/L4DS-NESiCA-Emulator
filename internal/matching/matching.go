// Package matching implements the arcade matching master: the UDP service the game queries to
// find rooms to join.
package matching

import (
	"bytes"
	"log/slog"
	"net"
	"strconv"
	"strings"
)

const (
	listQuery = '2'

	// errEmptyList is the reply error code meaning that no rooms matched.
	errEmptyList = 0x7e

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

// Reply returns the answer to a query, or nil when it needs none. Every list query is answered
// with an empty list.
func Reply(query Query) []byte {
	if query.Type != listQuery {
		return nil
	}

	sequence, err := strconv.ParseUint(query.Fields["seqNo"], 10, 32)
	if err != nil {
		return nil
	}
	return []byte{listQuery, byte(sequence >> 24), byte(sequence >> 16), byte(sequence >> 8), byte(sequence), errEmptyList}
}

// Serve answers queries arriving on conn until it is closed.
func Serve(conn net.PacketConn) {
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

		if reply := Reply(query); reply != nil {
			conn.WriteTo(reply, from)
		}
	}
}
