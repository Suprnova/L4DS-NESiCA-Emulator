package wire_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"l4ds-nesica-emulator/internal/wire"
)

func TestParseCardSelectRequest_ValidPayload_ReadsCardAndData(t *testing.T) {
	payload := make([]byte, 24, 28)
	copy(payload, "7020392000000001")
	binary.LittleEndian.PutUint32(payload[16:], 2)
	binary.LittleEndian.PutUint32(payload[20:], 4)
	payload = append(payload, 1, 2, 3, 4)

	got, err := wire.ParseCardSelectRequest(payload)

	if err != nil || got.CardID != "7020392000000001" || got.Kind != 2 || !bytes.Equal(got.Data, []byte{1, 2, 3, 4}) {
		t.Fatalf("ParseCardSelectRequest() = %+v, %v", got, err)
	}
}

func TestParseCardSelectRequest_ShortPayload_ReturnsErrBadFrame(t *testing.T) {
	_, err := wire.ParseCardSelectRequest(make([]byte, 10))

	if err != wire.ErrBadFrame {
		t.Fatalf("error = %v, want ErrBadFrame", err)
	}
}

func TestCardReply_Encode_PrefixesStatusAndBodyLength(t *testing.T) {
	got := wire.CardReply{Status: 1, Body: "abc"}.Encode()

	want := []byte{1, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 0, 3, 0, 0, 0, 'a', 'b', 'c'}
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode() = %v, want %v", got, want)
	}
}
