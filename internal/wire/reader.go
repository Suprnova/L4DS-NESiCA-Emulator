package wire

import (
	"encoding/binary"
	"io"
)

// MaxPayload is the largest payload ReadFrame accepts. The biggest message the game
// exchanges (an HTTP request) is a few kilobytes.
const MaxPayload = 1 << 20

// ReadFrame reads one frame from a pipe or stream. It reads the header and then exactly
// the number of payload bytes the header declares, so it does not depend on message boundaries.
func ReadFrame(r io.Reader) (Frame, error) {
	var header [HeaderSize]byte
	if _, err := io.ReadFull(r, header[:]); err != nil {
		return Frame{}, err
	}

	length := binary.LittleEndian.Uint32(header[4:])
	if length > MaxPayload {
		return Frame{}, ErrBadFrame
	}

	payload := make([]byte, length)
	if _, err := io.ReadFull(r, payload); err != nil {
		return Frame{}, err
	}

	return Frame{Command: Command(binary.LittleEndian.Uint32(header[:])), Payload: payload}, nil
}
