package wire

import (
	"encoding/binary"
	"strconv"
	"strings"
)

const cardIDSize = 16

// CardStatusUnregistered is the card reply status of the card server for a card with no
// player.
const CardStatusUnregistered uint32 = 0x17

// CardStatusNoReissue is the card reply status of the card server for a card that is not the
// target of a reissue reservation.
const CardStatusNoReissue uint32 = 0x1b

// ParseCardID reads the card number that starts every card command payload.
func ParseCardID(payload []byte) (string, error) {
	if len(payload) < cardIDSize {
		return "", ErrBadFrame
	}
	return getString(payload[:cardIDSize]), nil
}

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

// RankingRequest is a RankingData request.
type RankingRequest struct {
	// Kind is the ranking asked for: 10, 20, 30 or 40.
	Kind uint32
	// Store is the asking cabinet's store id (tenpo_id), or 0 when the request names none.
	Store uint32
}

// ParseRankingRequest reads a RankingData payload: u32 kind, u32 0, u32 n, then n bytes of
// key=value text ending in a NUL, such as "tenpo_id=1".
func ParseRankingRequest(payload []byte) (RankingRequest, error) {
	if len(payload) < 4 {
		return RankingRequest{}, ErrBadFrame
	}
	request := RankingRequest{Kind: binary.LittleEndian.Uint32(payload)}
	if len(payload) < 12 {
		return request, nil
	}
	n := min(int(binary.LittleEndian.Uint32(payload[8:])), len(payload)-12)
	for pair := range strings.SplitSeq(getString(payload[12:12+n]), ",") {
		if value, found := strings.CutPrefix(pair, "tenpo_id="); found {
			store, _ := strconv.ParseUint(value, 10, 32)
			request.Store = uint32(store)
		}
	}
	return request, nil
}

// RankingReply is the payload of RankingDataReply: a status and a line-oriented body of
// result, API_ID, the entry count and the entries.
type RankingReply struct {
	Status uint32
	Body   string
}

// Encode returns the payload bytes.
func (r RankingReply) Encode() []byte {
	buf := binary.LittleEndian.AppendUint32(nil, r.Status)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(r.Body)))
	return append(buf, r.Body...)
}

// CardWriteRequest is the payload of the CardInsert and CardUpdate commands: text the game
// stores on a card.
type CardWriteRequest struct {
	CardID   string
	DataType uint32
	Data     []byte
}

// ParseCardWriteRequest decodes the payload of a CardInsert or CardUpdate command.
func ParseCardWriteRequest(payload []byte) (CardWriteRequest, error) {
	const headerSize = cardIDSize + 16
	if len(payload) < headerSize {
		return CardWriteRequest{}, ErrBadFrame
	}

	dataLength := binary.LittleEndian.Uint32(payload[cardIDSize+12:])
	if uint64(len(payload)-headerSize) < uint64(dataLength) {
		return CardWriteRequest{}, ErrBadFrame
	}

	return CardWriteRequest{
		CardID:   getString(payload[:cardIDSize]),
		DataType: binary.LittleEndian.Uint32(payload[cardIDSize+8:]),
		Data:     payload[headerSize : headerSize+int(dataLength)],
	}, nil
}
