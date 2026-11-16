package wire

import "encoding/binary"

const (
	storeInfoSize     = 0x78
	newsInfoSize      = 0x408
	adapterInfoSize   = 0xE8
	globalAddressSize = 0x28
	versionSize       = 0x21
)

// StoreInfo describes the shop a cabinet belongs to. The game treats the network as usable
// only when Prefecture is not empty.
type StoreInfo struct {
	ID         uint32
	Name       string
	Address    string
	Ticket     string
	Prefecture string
}

func (s StoreInfo) encode() []byte {
	buf := make([]byte, storeInfoSize)
	binary.LittleEndian.PutUint32(buf, s.ID)
	putString(buf[4:35], s.Name)
	putString(buf[35:68], s.Address)
	putString(buf[68:101], s.Ticket)
	putString(buf[101:118], s.Prefecture)
	return buf
}

// NewsImage points at the news picture the game shows on its attract screen.
type NewsImage struct {
	Type uint32
	Size uint32
	Path string
}

func (n NewsImage) encode() []byte {
	buf := make([]byte, newsInfoSize)
	binary.LittleEndian.PutUint32(buf, n.Type)
	binary.LittleEndian.PutUint32(buf[4:], n.Size)
	putString(buf[8:], n.Path)
	return buf
}

// CertInit is the payload of the CertInitNotice command, which tells the game the network is usable.
// ServerKey must be one of the host strings the game recognises; it selects the master and lobby hosts.
type CertInit struct {
	Store     StoreInfo
	News      NewsImage
	ServerKey string
}

// Encode returns the payload bytes.
func (c CertInit) Encode() []byte {
	key := append([]byte(c.ServerKey), 0)
	buf := append(c.Store.encode(), c.News.encode()...)
	buf = binary.LittleEndian.AppendUint32(buf, uint32(len(key)))
	return append(buf, key...)
}

// AdapterDetails is the payload of the AdapterInfoReply command: the cabinet's network adapter.
type AdapterDetails struct {
	DHCPEnabled bool
	Gateway     string
	IP          string
	Mask        string
	DHCPServer  string
	DNSServer   string
	MAC         string
}

// Encode returns the payload bytes.
func (a AdapterDetails) Encode() []byte {
	buf := make([]byte, adapterInfoSize)
	binary.LittleEndian.PutUint32(buf, 1)
	if a.DHCPEnabled {
		binary.LittleEndian.PutUint32(buf[4:], 1)
	}
	putString(buf[0x08:0x28], a.Gateway)
	putString(buf[0x28:0x48], a.IP)
	putString(buf[0x48:0x68], a.Mask)
	putString(buf[0x68:0x88], a.DHCPServer)
	putString(buf[0x88:0xA8], a.DNSServer)
	putString(buf[0xA8:], a.MAC)
	return buf
}

// PublicAddress is the payload of the GlobalAddressReply command: the cabinet's public address.
type PublicAddress struct {
	ResponseMillis uint32
	IP             string
}

// Encode returns the payload bytes.
func (g PublicAddress) Encode() []byte {
	buf := make([]byte, globalAddressSize)
	binary.LittleEndian.PutUint32(buf, 1)
	binary.LittleEndian.PutUint32(buf[4:], g.ResponseMillis)
	putString(buf[8:], g.IP)
	return buf
}

// EncodeServiceVersion returns the payload of the ServiceVersionReply command.
func EncodeServiceVersion(version string) []byte {
	buf := make([]byte, versionSize)
	putString(buf, version)
	return buf
}

// EmptyEventList is the payload of a RowEventDataReply that contains no events.
func EmptyEventList() []byte {
	return make([]byte, 4)
}
