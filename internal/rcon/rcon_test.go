package rcon_test

import (
	"bytes"
	"encoding/binary"
	"errors"
	"io"
	"net"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/rcon"
)

// fakeServer accepts one connection and answers like a Source server: an empty response and an
// auth response to the password, the command's output in the given parts, and a mirror of each
// empty response packet.
func fakeServer(t *testing.T, password string, parts ...string) string {
	t.Helper()
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })

	go func() {
		conn, err := listener.Accept()
		if err != nil {
			return
		}
		defer conn.Close()
		for {
			id, packetType, body, err := readPacket(conn)
			if err != nil {
				return
			}
			switch packetType {
			case 3:
				writePacket(conn, id, 0, "")
				if body != password {
					id = -1
				}
				writePacket(conn, id, 2, "")
			case 2:
				for _, part := range parts {
					writePacket(conn, id, 0, part)
				}
			case 0:
				writePacket(conn, id, 0, "")
			}
		}
	}()
	return listener.Addr().String()
}

func readPacket(r io.Reader) (id, packetType int32, body string, err error) {
	var size int32
	if err := binary.Read(r, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", err
	}
	data := make([]byte, size)
	if _, err := io.ReadFull(r, data); err != nil {
		return 0, 0, "", err
	}
	text, _, _ := bytes.Cut(data[8:], []byte{0})
	return int32(binary.LittleEndian.Uint32(data)), int32(binary.LittleEndian.Uint32(data[4:])), string(text), nil
}

func writePacket(w io.Writer, id, packetType int32, body string) {
	packet := binary.LittleEndian.AppendUint32(nil, uint32(10+len(body)))
	packet = binary.LittleEndian.AppendUint32(packet, uint32(id))
	packet = binary.LittleEndian.AppendUint32(packet, uint32(packetType))
	w.Write(append(append(packet, body...), 0, 0))
}

func TestExec_ReturnsTheWholeOutput(t *testing.T) {
	address := fakeServer(t, "secret", "map     : c1m1_hotel\n", "players : 1 humans\n")
	client, err := rcon.Dial(address, "secret", time.Second)
	if err != nil {
		t.Fatal(err)
	}
	defer client.Close()

	output, err := client.Exec("status")

	if err != nil || output != "map     : c1m1_hotel\nplayers : 1 humans\n" {
		t.Fatalf("Exec() = %q, %v", output, err)
	}
}

func TestDial_WrongPassword_IsRejected(t *testing.T) {
	address := fakeServer(t, "secret")

	_, err := rcon.Dial(address, "wrong", time.Second)

	if !errors.Is(err, rcon.ErrAuth) {
		t.Fatalf("Dial() error = %v, want %v", err, rcon.ErrAuth)
	}
}
