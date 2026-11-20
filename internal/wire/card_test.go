package wire_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
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

func TestParseCardWriteRequest_ValidPayload_ReadsCardTypeAndData(t *testing.T) {
	payload := make([]byte, 32)
	copy(payload, "7020392000000001")
	binary.LittleEndian.PutUint32(payload[24:], 3)
	binary.LittleEndian.PutUint32(payload[28:], 5)
	payload = append(payload, "hello"...)
	payload = append(payload, 0, 0, 0)

	got, err := wire.ParseCardWriteRequest(payload)

	if err != nil || got.CardID != "7020392000000001" || got.DataType != 3 || string(got.Data) != "hello" {
		t.Fatalf("ParseCardWriteRequest() = %+v, %v", got, err)
	}
}

func TestParseCardWriteRequest_DataLongerThanPayload_ReturnsErrBadFrame(t *testing.T) {
	payload := make([]byte, 32)
	binary.LittleEndian.PutUint32(payload[28:], 99)

	_, err := wire.ParseCardWriteRequest(payload)

	if err != wire.ErrBadFrame {
		t.Fatalf("error = %v, want ErrBadFrame", err)
	}
}

func TestParseCardID_ReadsTheLeadingCardNumber(t *testing.T) {
	payload := make([]byte, 20)
	copy(payload, "7020392000000001")

	got, err := wire.ParseCardID(payload)

	if err != nil || got != "7020392000000001" {
		t.Fatalf("ParseCardID() = %q, %v", got, err)
	}
}

func TestParseRankingRequest_GamePayload_ReadsKindAndStore(t *testing.T) {
	payload := []byte("\x14\x00\x00\x00\x00\x00\x00\x00\x0b\x00\x00\x00tenpo_id=7\x00\x00\x00\x00\x00")

	got, err := wire.ParseRankingRequest(payload)

	if err != nil || got != (wire.RankingRequest{Kind: 20, Store: 7}) {
		t.Fatalf("ParseRankingRequest() = %+v, %v", got, err)
	}
}

func TestParseRankingRequest_KindOnly_HasNoStore(t *testing.T) {
	got, err := wire.ParseRankingRequest([]byte{10, 0, 0, 0})

	if err != nil || got != (wire.RankingRequest{Kind: 10}) {
		t.Fatalf("ParseRankingRequest() = %+v, %v", got, err)
	}
}

func TestParseRankingRequest_LengthPastTheEnd_ReadsWhatIsThere(t *testing.T) {
	payload := []byte("\x28\x00\x00\x00\x00\x00\x00\x00\xff\x00\x00\x00tenpo_id=3")

	got, err := wire.ParseRankingRequest(payload)

	if err != nil || got != (wire.RankingRequest{Kind: 40, Store: 3}) {
		t.Fatalf("ParseRankingRequest() = %+v, %v", got, err)
	}
}

func TestParseRankingRequest_ShortPayload_ReturnsErrBadFrame(t *testing.T) {
	_, err := wire.ParseRankingRequest([]byte{10})

	if err != wire.ErrBadFrame {
		t.Fatalf("error = %v, want ErrBadFrame", err)
	}
}
