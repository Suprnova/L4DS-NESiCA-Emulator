package wire_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"l4ds-nesica-emulator/internal/wire"
)

func httpRequestPayload(url string, sequence uint32, body string) []byte {
	payload := make([]byte, 0xC5B+len(body))
	copy(payload, url)
	binary.LittleEndian.PutUint32(payload[0x848:], sequence)
	binary.LittleEndian.PutUint32(payload[0x854:], uint32(len(body)))
	copy(payload[0xC5B:], body)
	return payload
}

func TestParseHTTPRequest_ValidPayload_ReadsUrlSequenceAndBody(t *testing.T) {
	payload := httpRequestPayload("http://lobby/sendversion", 7, "tenpo_id=1&version=1.2604")

	got, err := wire.ParseHTTPRequest(payload)

	if err != nil || got.URL != "http://lobby/sendversion" || got.Sequence != 7 || string(got.Body) != "tenpo_id=1&version=1.2604" {
		t.Fatalf("ParseHTTPRequest() = %+v, %v", got, err)
	}
}

func TestParseHTTPRequest_BodyLengthMismatch_ReturnsErrBadFrame(t *testing.T) {
	payload := httpRequestPayload("http://lobby/x", 1, "abc")
	binary.LittleEndian.PutUint32(payload[0x854:], 99)

	_, err := wire.ParseHTTPRequest(payload)

	if err != wire.ErrBadFrame {
		t.Fatalf("ParseHTTPRequest() error = %v, want ErrBadFrame", err)
	}
}

func TestHTTPResponseEncode_Response_PlacesFieldsAndBody(t *testing.T) {
	response := wire.HTTPResponse{StatusCode: 200, Sequence: 7, Body: []byte("1\n0\n")}

	payload := response.Encode()

	if binary.LittleEndian.Uint32(payload[0x404:]) != 200 || binary.LittleEndian.Uint32(payload[0x408:]) != 0 ||
		binary.LittleEndian.Uint32(payload[0x40C:]) != 7 || binary.LittleEndian.Uint32(payload[0x410:]) != 4 ||
		!bytes.Equal(payload[0x414:], []byte("1\n0\n")) {
		t.Fatalf("payload tail = % X", payload[0x400:])
	}
}
