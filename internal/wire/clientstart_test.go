package wire_test

import (
	"encoding/binary"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

func TestParseClientStart_ValidPayload_ReadsFields(t *testing.T) {
	payload := make([]byte, 0x12C)
	binary.LittleEndian.PutUint32(payload, 4242)
	copy(payload[4:], `D:\Games\l4ds\left4dead2.exe`)
	copy(payload[265:], "2.79 2014/07/08")

	got, err := wire.ParseClientStart(payload)

	want := wire.ClientStartRequest{ProcessID: 4242, Executable: `D:\Games\l4ds\left4dead2.exe`, Version: "2.79 2014/07/08"}
	if err != nil || got != want {
		t.Fatalf("ParseClientStart() = %+v, %v, want %+v", got, err, want)
	}
}

func TestParseClientStart_WrongSize_ReturnsErrBadFrame(t *testing.T) {
	_, err := wire.ParseClientStart(make([]byte, 10))

	if err != wire.ErrBadFrame {
		t.Fatalf("ParseClientStart() error = %v, want ErrBadFrame", err)
	}
}
