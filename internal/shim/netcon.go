package shim

import (
	"fmt"
	"net"
	"strconv"
	"time"
)

// PutCard puts card on the card reader of the game on this machine whose netcon port (its
// -netconport) is port. The game reads it at its next card scan, on the title or card screen.
// It needs the client patches, which make change_debug_dummyCardID a release command.
func PutCard(port int, card string) error {
	conn, err := net.DialTimeout("tcp", net.JoinHostPort("127.0.0.1", strconv.Itoa(port)), time.Second)
	if err != nil {
		return err
	}
	defer conn.Close()
	conn.SetWriteDeadline(time.Now().Add(time.Second))
	_, err = fmt.Fprintf(conn, "change_debug_dummyCardID %s\n", card)
	return err
}
