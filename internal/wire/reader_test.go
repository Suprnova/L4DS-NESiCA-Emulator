package wire_test

import (
	"bytes"
	"io"
	"testing"

	"l4ds-nesica-emulator/internal/wire"
)

func TestReadFrame_BackToBackFrames_ReadsEachInOrder(t *testing.T) {
	first := wire.Frame{Command: wire.ConnectRequest}
	second := wire.Frame{Command: wire.HTTPPost, Payload: []byte{9, 8, 7}}
	stream := bytes.NewReader(append(first.Encode(), second.Encode()...))

	gotFirst, err1 := wire.ReadFrame(stream)
	gotSecond, err2 := wire.ReadFrame(stream)

	if err1 != nil || err2 != nil || gotFirst.Command != first.Command ||
		gotSecond.Command != second.Command || !bytes.Equal(gotSecond.Payload, second.Payload) {
		t.Fatalf("ReadFrame() = %+v, %+v, errors %v, %v", gotFirst, gotSecond, err1, err2)
	}
}

func TestReadFrame_TruncatedPayload_ReturnsUnexpectedEOF(t *testing.T) {
	message := wire.Frame{Command: wire.HTTPPost, Payload: []byte{1, 2, 3}}.Encode()

	_, err := wire.ReadFrame(bytes.NewReader(message[:len(message)-1]))

	if err != io.ErrUnexpectedEOF {
		t.Fatalf("ReadFrame() error = %v, want io.ErrUnexpectedEOF", err)
	}
}

func TestReadFrame_OversizedLength_ReturnsErrBadFrame(t *testing.T) {
	header := []byte{0x01, 0, 0, 0, 0xFF, 0xFF, 0xFF, 0x7F}

	_, err := wire.ReadFrame(bytes.NewReader(header))

	if err != wire.ErrBadFrame {
		t.Fatalf("ReadFrame() error = %v, want ErrBadFrame", err)
	}
}
