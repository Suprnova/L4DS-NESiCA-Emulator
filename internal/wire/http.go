package wire

import "encoding/binary"

const (
	httpRequestHeaderSize = 0xC5B
	httpReplyHeaderSize   = 0x414
	httpURLSize           = 0x401
)

// HTTPRequest is the payload of the HTTPPost command: a request the game wants the service
// to make on its behalf.
type HTTPRequest struct {
	URL      string
	Sequence uint32
	Body     []byte
}

// ParseHTTPRequest decodes the payload of an HTTPPost command.
func ParseHTTPRequest(payload []byte) (HTTPRequest, error) {
	if len(payload) < httpRequestHeaderSize {
		return HTTPRequest{}, ErrBadFrame
	}

	bodyLength := binary.LittleEndian.Uint32(payload[0x854:])
	if uint64(len(payload)-httpRequestHeaderSize) != uint64(bodyLength) {
		return HTTPRequest{}, ErrBadFrame
	}

	return HTTPRequest{
		URL:      getString(payload[:httpURLSize]),
		Sequence: binary.LittleEndian.Uint32(payload[0x848:]),
		Body:     payload[httpRequestHeaderSize:],
	}, nil
}

// HTTPResponse is the payload of the HTTPReply command. A nonzero ErrorCode reports that the
// request could not be made at all.
type HTTPResponse struct {
	StatusCode uint32
	ErrorCode  uint32
	Sequence   uint32
	Body       []byte
}

// Encode returns the payload bytes.
func (r HTTPResponse) Encode() []byte {
	buf := make([]byte, httpReplyHeaderSize, httpReplyHeaderSize+len(r.Body))
	binary.LittleEndian.PutUint32(buf[0x404:], r.StatusCode)
	binary.LittleEndian.PutUint32(buf[0x408:], r.ErrorCode)
	binary.LittleEndian.PutUint32(buf[0x40C:], r.Sequence)
	binary.LittleEndian.PutUint32(buf[0x410:], uint32(len(r.Body)))
	return append(buf, r.Body...)
}

// HTTPStartPayload is the payload of the HTTPStart command, which announces that the service
// has accepted a request.
func HTTPStartPayload() []byte {
	return binary.LittleEndian.AppendUint32(nil, 1)
}
