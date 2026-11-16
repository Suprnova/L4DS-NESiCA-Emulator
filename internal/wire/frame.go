// Package wire implements the binary protocol the game speaks over the NESYS named pipe.
package wire

import (
	"encoding/binary"
	"errors"
)

// HeaderSize is the size in bytes of the frame header: a command and a payload length.
const HeaderSize = 8

// ErrBadFrame is returned when a pipe message is not exactly one well-formed frame.
var ErrBadFrame = errors.New("wire: malformed frame")

// Frame is one message on the NESYS pipe. The game sends one frame per pipe message.
type Frame struct {
	Command Command
	Payload []byte
}

// Decode parses a single pipe message. It fails with [ErrBadFrame] unless the message
// is a complete header followed by exactly the declared number of payload bytes.
func Decode(message []byte) (Frame, error) {
	if len(message) < HeaderSize {
		return Frame{}, ErrBadFrame
	}

	command := Command(binary.LittleEndian.Uint32(message))
	length := binary.LittleEndian.Uint32(message[4:])
	if uint64(len(message)-HeaderSize) != uint64(length) {
		return Frame{}, ErrBadFrame
	}

	return Frame{Command: command, Payload: message[HeaderSize:]}, nil
}

// Encode returns the pipe message for the frame.
func (f Frame) Encode() []byte {
	message := make([]byte, HeaderSize+len(f.Payload))
	binary.LittleEndian.PutUint32(message, uint32(f.Command))
	binary.LittleEndian.PutUint32(message[4:], uint32(len(f.Payload)))
	copy(message[HeaderSize:], f.Payload)
	return message
}
