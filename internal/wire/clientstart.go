package wire

import "encoding/binary"

// clientStartSize is the payload size of the ClientStart command: a process id,
// a 261-byte executable path and a 33-byte library version, padded to 4-byte alignment.
const clientStartSize = 0x12C

// ClientStartRequest is the payload of the ClientStart command, sent when the game opens the pipe.
type ClientStartRequest struct {
	ProcessID  uint32
	Executable string
	Version    string
}

// ParseClientStart decodes the payload of a ClientStart command.
func ParseClientStart(payload []byte) (ClientStartRequest, error) {
	if len(payload) != clientStartSize {
		return ClientStartRequest{}, ErrBadFrame
	}

	return ClientStartRequest{
		ProcessID:  binary.LittleEndian.Uint32(payload),
		Executable: getString(payload[4:265]),
		Version:    getString(payload[265:298]),
	}, nil
}
