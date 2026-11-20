package wire_test

import (
	"bytes"
	"encoding/binary"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

func TestCertInitEncode_Store_PlacesPrefectureAtOffset0x65(t *testing.T) {
	init := wire.CertInit{Store: wire.StoreInfo{ID: 119, Prefecture: "Tokyo"}, ServerKey: "222.158.239.24"}

	payload := init.Encode()

	if binary.LittleEndian.Uint32(payload) != 119 || string(payload[0x65:0x6A]) != "Tokyo" {
		t.Fatalf("store block = % X", payload[:0x78])
	}
}

func TestCertInitEncode_ServerKey_FollowsNewsAsLengthPrefixedString(t *testing.T) {
	init := wire.CertInit{ServerKey: "222.158.239.24"}

	payload := init.Encode()

	tail := payload[0x78+0x408:]
	if binary.LittleEndian.Uint32(tail) != 15 || !bytes.Equal(tail[4:], []byte("222.158.239.24\x00")) {
		t.Fatalf("tail = % X", tail)
	}
}

func TestAdapterInfoEncode_Fields_AreMarkedValidAndPlaced(t *testing.T) {
	adapter := wire.AdapterDetails{IP: "10.0.0.5", MAC: "001C42888475"}

	payload := adapter.Encode()

	if len(payload) != 0xE8 || binary.LittleEndian.Uint32(payload) != 1 ||
		string(payload[0x28:0x30]) != "10.0.0.5" || string(payload[0xA8:0xB4]) != "001C42888475" {
		t.Fatalf("payload = % X", payload)
	}
}

func TestGlobalAddressEncode_Address_IsOkWithResponseTime(t *testing.T) {
	address := wire.PublicAddress{ResponseMillis: 12, IP: "203.0.113.7"}

	payload := address.Encode()

	if len(payload) != 0x28 || binary.LittleEndian.Uint32(payload) != 1 ||
		binary.LittleEndian.Uint32(payload[4:]) != 12 || string(payload[8:19]) != "203.0.113.7" {
		t.Fatalf("payload = % X", payload)
	}
}

func TestEncodeServiceVersion_LongVersion_KeepsTerminator(t *testing.T) {
	payload := wire.EncodeServiceVersion("2.79 2014/07/08")

	if len(payload) != 0x21 || payload[len(payload)-1] != 0 || string(payload[:15]) != "2.79 2014/07/08" {
		t.Fatalf("payload = % X", payload)
	}
}

func TestMaintenanceWarningEncode_Minutes_AreTextAfterTheFlag(t *testing.T) {
	got := wire.MaintenanceWarning{Soon: true, Minutes: 25}.Encode()

	want := append([]byte{1, 0, 0, 0, '2', '5'}, make([]byte, 14)...)
	if !bytes.Equal(got, want) {
		t.Fatalf("Encode() = %v, want %v", got, want)
	}
}

func TestMaintenanceWarningEncode_NoMinutes_LeavesTheTextEmpty(t *testing.T) {
	got := wire.MaintenanceWarning{Soon: true}.Encode()

	if want := append([]byte{1, 0, 0, 0}, make([]byte, 16)...); !bytes.Equal(got, want) {
		t.Fatalf("Encode() = %v, want %v", got, want)
	}
}

func TestMaintenanceWarningEncode_NotSoon_ClearsTheWarning(t *testing.T) {
	got := wire.MaintenanceWarning{Minutes: 25}.Encode()

	if !bytes.Equal(got, make([]byte, 20)) {
		t.Fatalf("Encode() = %v, want 20 zero bytes", got)
	}
}
