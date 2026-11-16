package wire_test

import (
	"bytes"
	"testing"

	"l4ds-nesica-emulator/internal/wire"
)

func TestEncode_FrameWithoutPayload_IsHeaderOnly(t *testing.T) {
	frame := wire.Frame{Command: wire.ClientStartReply}

	message := frame.Encode()

	want := []byte{0x0D, 0x01, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
	if !bytes.Equal(message, want) {
		t.Fatalf("Encode() = % X, want % X", message, want)
	}
}

func TestDecode_EncodedFrame_RoundTrips(t *testing.T) {
	frame := wire.Frame{Command: wire.HTTPPost, Payload: []byte{1, 2, 3}}

	got, err := wire.Decode(frame.Encode())

	if err != nil || got.Command != frame.Command || !bytes.Equal(got.Payload, frame.Payload) {
		t.Fatalf("Decode() = %+v, %v, want %+v", got, err, frame)
	}
}

func TestDecode_MalformedMessage_ReturnsErrBadFrame(t *testing.T) {
	tests := map[string][]byte{
		"shorter than header": {0x01, 0x00, 0x00},
		"length too large":    {0x01, 0, 0, 0, 0x05, 0, 0, 0, 0xAA},
		"length too small":    {0x01, 0, 0, 0, 0x00, 0, 0, 0, 0xAA},
	}

	for name, message := range tests {
		t.Run(name, func(t *testing.T) {
			_, err := wire.Decode(message)

			if err != wire.ErrBadFrame {
				t.Fatalf("Decode() error = %v, want ErrBadFrame", err)
			}
		})
	}
}
