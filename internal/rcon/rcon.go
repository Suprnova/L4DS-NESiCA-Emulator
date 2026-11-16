// Package rcon is a client for the Source remote console protocol, which a dedicated server
// speaks on TCP at its game port.
package rcon

import (
	"bytes"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"strings"
	"time"
)

// Packet types. An exec request and an auth response share the value 2.
const (
	typeResponse = 0
	typeExec     = 2
	typeAuthResp = 2
	typeAuth     = 3
)

// maxPacket bounds the size field of a packet the server sends.
const maxPacket = 4096 + 10

// ErrAuth is returned by [Dial] when the server rejects the password.
var ErrAuth = errors.New("rcon: password rejected")

// Client is an authenticated remote console connection.
type Client struct {
	conn    net.Conn
	timeout time.Duration
	nextID  int32
}

// Dial connects to the server at address and authenticates with password. Each exchange,
// including the authentication, must finish within timeout.
func Dial(address, password string, timeout time.Duration) (*Client, error) {
	conn, err := net.DialTimeout("tcp", address, timeout)
	if err != nil {
		return nil, err
	}

	c := &Client{conn: conn, timeout: timeout, nextID: 1}
	if err := c.auth(password); err != nil {
		conn.Close()
		return nil, err
	}
	return c, nil
}

func (c *Client) auth(password string) error {
	c.conn.SetDeadline(time.Now().Add(c.timeout))
	id := c.id()
	if err := c.write(id, typeAuth, password); err != nil {
		return err
	}

	for {
		gotID, gotType, _, err := c.read()
		if err != nil {
			return err
		}
		if gotType != typeAuthResp {
			continue
		}
		if gotID == -1 {
			return ErrAuth
		}
		if gotID == id {
			return nil
		}
	}
}

// Exec runs command on the server and returns its console output. The server may split long
// output over several packets, so Exec follows the command with an empty response packet, which
// the server mirrors once the output is complete.
func (c *Client) Exec(command string) (string, error) {
	c.conn.SetDeadline(time.Now().Add(c.timeout))
	id, end := c.id(), c.id()
	if err := c.write(id, typeExec, command); err != nil {
		return "", err
	}
	if err := c.write(end, typeResponse, ""); err != nil {
		return "", err
	}

	var output strings.Builder
	for {
		gotID, _, body, err := c.read()
		if err != nil {
			return "", err
		}
		switch gotID {
		case id:
			output.WriteString(body)
		case end:
			return output.String(), nil
		}
	}
}

// Close closes the connection.
func (c *Client) Close() error { return c.conn.Close() }

func (c *Client) id() int32 {
	id := c.nextID
	c.nextID++
	return id
}

// write sends a packet: its size, id and type as little-endian int32s, then the body and two
// NUL bytes.
func (c *Client) write(id, packetType int32, body string) error {
	packet := binary.LittleEndian.AppendUint32(nil, uint32(10+len(body)))
	packet = binary.LittleEndian.AppendUint32(packet, uint32(id))
	packet = binary.LittleEndian.AppendUint32(packet, uint32(packetType))
	packet = append(packet, body...)
	_, err := c.conn.Write(append(packet, 0, 0))
	return err
}

func (c *Client) read() (id, packetType int32, body string, err error) {
	var size int32
	if err := binary.Read(c.conn, binary.LittleEndian, &size); err != nil {
		return 0, 0, "", err
	}
	if size < 10 || size > maxPacket {
		return 0, 0, "", fmt.Errorf("rcon: bad packet size %d", size)
	}

	data := make([]byte, size)
	if _, err := io.ReadFull(c.conn, data); err != nil {
		return 0, 0, "", err
	}
	id = int32(binary.LittleEndian.Uint32(data))
	packetType = int32(binary.LittleEndian.Uint32(data[4:]))
	text, _, _ := bytes.Cut(data[8:], []byte{0})
	return id, packetType, string(text), nil
}
