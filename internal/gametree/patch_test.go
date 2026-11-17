package gametree

import (
	"bytes"
	"debug/pe"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"
)

// testImage is 32 bytes of section data at RVA 0x1000, whose bytes 8 to 11 the loader rewrites.
func testImage() *image {
	return &image{
		data:      bytes.Repeat([]byte{0xcc}, 32),
		base:      0x10000000,
		sections:  []pe.SectionHeader{{VirtualAddress: 0x1000, Size: 32, Offset: 0}},
		relocated: map[uint32]bool{0x1008: true, 0x1009: true, 0x100a: true, 0x100b: true},
	}
}

func TestRelocatedBytes_ListsEachHighLowEntrysFourBytes(t *testing.T) {
	table := binary.LittleEndian.AppendUint32(nil, 0x2000)
	table = binary.LittleEndian.AppendUint32(table, 12)
	table = binary.LittleEndian.AppendUint16(table, 3<<12|0x010)
	table = binary.LittleEndian.AppendUint16(table, 0)

	got := relocatedBytes(table)

	if len(got) != 4 || !got[0x2010] || !got[0x2013] {
		t.Fatalf("relocated = %v", got)
	}
}

func TestReplace_ExpectedBytes_AreReplaced(t *testing.T) {
	img := testImage()

	err := img.replace(0x10001000, []byte{0xcc, 0xcc}, []byte{0xeb, 0x04})

	if err != nil || img.data[0] != 0xeb || img.data[1] != 0x04 {
		t.Fatalf("replace() = %v, data %x", err, img.data[:2])
	}
}

func TestReplace_AlreadyReplaced_IsLeftAlone(t *testing.T) {
	img := testImage()
	img.data[0] = 0xeb

	err := img.replace(0x10001000, []byte{0xcc}, []byte{0xeb})

	if err != nil || img.data[0] != 0xeb {
		t.Fatalf("replace() = %v, data %x", err, img.data[:1])
	}
}

func TestReplace_OtherBytes_ReturnErrUnexpectedBytes(t *testing.T) {
	img := testImage()

	err := img.replace(0x10001000, []byte{0x75}, []byte{0xeb})

	if !errors.Is(err, ErrUnexpectedBytes) || img.data[0] != 0xcc {
		t.Fatalf("replace() = %v, data %x", err, img.data[:1])
	}
}

func TestReplace_RelocatedBytes_AreRefused(t *testing.T) {
	img := testImage()

	err := img.replace(0x10001006, bytes.Repeat([]byte{0xcc}, 4), bytes.Repeat([]byte{0x90}, 4))

	if err == nil || img.data[6] != 0xcc {
		t.Fatalf("replace() = %v", err)
	}
}

func TestReplace_AddressOutsideTheFile_ReturnsAnError(t *testing.T) {
	img := testImage()

	err := img.replace(0x10003000, []byte{0xcc}, []byte{0x90})

	if err == nil {
		t.Fatal("replace() error = nil")
	}
}

func TestOthers_ClientPatches_AreTheServerPatches(t *testing.T) {
	got := others(ClientPatches)

	if len(got) != len(ServerPatches) || got[0].VA != ServerPatches[0].VA {
		t.Fatalf("others = %d patches", len(got))
	}
}

// TestPatches_MatchTheGame checks every patch against the installed game named by
// L4DS_GAME_DIR: each site holds its original bytes and avoids relocated bytes.
func TestPatches_MatchTheGame(t *testing.T) {
	gameDir := os.Getenv("L4DS_GAME_DIR")
	if gameDir == "" {
		t.Skip("L4DS_GAME_DIR is not set")
	}

	for _, p := range append(ServerPatches, ClientPatches...) {
		data, err := os.ReadFile(filepath.Join(gameDir, "bin", p.DLL))
		if err != nil {
			t.Fatal(err)
		}
		img, err := parseImage(data)
		if err != nil {
			t.Fatal(err)
		}

		if err := img.replace(p.VA, p.Original, p.Patched); err != nil {
			t.Errorf("%s %#x: %v", p.DLL, p.VA, err)
		}
	}
}
