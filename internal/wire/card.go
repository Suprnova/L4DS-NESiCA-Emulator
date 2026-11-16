package wire

import "encoding/binary"

const cardIDSize = 16

// CardSelectRequest is the payload of the CardSelect command: a lookup of the player
// registered to a card.
type CardSelectRequest struct {
	CardID string
	Kind   uint32
	Data   []byte
}

// ParseCardSelectRequest decodes the payload of a CardSelect command.
func ParseCardSelectRequest(payload []byte) (CardSelectRequest, error) {
	const headerSize = cardIDSize + 8
	if len(payload) < headerSize {
		return CardSelectRequest{}, ErrBadFrame
	}

	dataLength := binary.LittleEndian.Uint32(payload[cardIDSize+4:])
	if uint64(len(payload)-headerSize) < uint64(dataLength) {
		return CardSelectRequest{}, ErrBadFrame
	}

	return CardSelectRequest{
		CardID: getString(payload[:cardIDSize]),
		Kind:   binary.LittleEndian.Uint32(payload[cardIDSize:]),
		Data:   payload[headerSize : headerSize+int(dataLength)],
	}, nil
}

// CardReply is the payload of the card reply commands: a status and a line-oriented body.
type CardReply struct {
	Status uint32
	Body   string
}

// Encode returns the payload bytes.
func (r CardReply) Encode() []byte {
	buf := binary.LittleEndian.AppendUint32(nil, r.Status)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = binary.LittleEndian.AppendUint32(buf, 0)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(r.Body)))
	return append(buf, r.Body...)
}
