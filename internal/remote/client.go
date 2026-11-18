package remote

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// errNotFound reports a 404 from the server.
var errNotFound = errors.New("remote: not found")

// Client is a nesys.Backend that sends the game's requests to a server.
type Client struct {
	base    string
	cabinet string
	http    *http.Client
}

// NewClient returns a client of the server at base, such as http://192.168.1.10:8080, that
// names itself cabinet in every request.
func NewClient(base, cabinet string) *Client {
	return &Client{base: base, cabinet: cabinet, http: &http.Client{Timeout: 30 * time.Second}}
}

// Post sends a tunnelled game request to the server's game server. Only the path and query of
// rawURL are kept: the server answers for whichever lobby host the game named.
func (c *Client) Post(rawURL string, body []byte) (int, []byte, error) {
	parsed, err := url.Parse(rawURL)
	if err != nil {
		return 0, nil, err
	}
	target := c.base + "/zet" + parsed.EscapedPath()
	if parsed.RawQuery != "" {
		target += "?" + parsed.RawQuery
	}
	response, err := c.do(http.MethodPost, target, "application/x-www-form-urlencoded", body)
	if err != nil {
		return 0, nil, err
	}
	defer response.Body.Close()
	reply, err := io.ReadAll(response.Body)
	return response.StatusCode, reply, err
}

// Select looks a card up on the server.
func (c *Client) Select(cardID string) (wire.CardReply, error) {
	var reply cardReply
	err := c.call("/nesys/card/select", cardRequest{Card: cardID}, &reply)
	return reply.card(), err
}

// ReissueTest asks the server whether a reissue is reserved for a card.
func (c *Client) ReissueTest(cardID string) (wire.CardReply, error) {
	var reply cardReply
	err := c.call("/nesys/card/reissue-test", cardRequest{Card: cardID}, &reply)
	return reply.card(), err
}

// Insert registers a player to a card on the server.
func (c *Client) Insert(cardID, data string) (string, error) {
	var reply cardReply
	err := c.call("/nesys/card/insert", cardRequest{Card: cardID, Data: data}, &reply)
	return reply.Body, err
}

// Update stores a card update on the server.
func (c *Client) Update(cardID string, dataType uint32, data string) error {
	return c.call("/nesys/card/update", cardRequest{Card: cardID, Type: dataType, Data: data}, &struct{}{})
}

// Ranking fetches a ranking from the server.
func (c *Client) Ranking(kind uint32) (string, error) {
	var reply cardReply
	err := c.call("/nesys/ranking", rankingRequest{Kind: kind}, &reply)
	return reply.Body, err
}

// Hello checks that the server is reachable and speaks this protocol.
func (c *Client) Hello() error {
	var reply hello
	if err := c.get("/nesys/hello", &reply); err != nil {
		return err
	}
	if reply.Protocol != Protocol {
		return fmt.Errorf("%w: the server speaks %d, this shim %d", ErrProtocol, reply.Protocol, Protocol)
	}
	return nil
}

// Store returns the store the server lists under id, or ErrUnknownStore.
func (c *Client) Store(id uint32) (Store, error) {
	var store Store
	err := c.get("/nesys/store/"+strconv.FormatUint(uint64(id), 10), &store)
	if errors.Is(err, errNotFound) {
		return Store{}, ErrUnknownStore
	}
	return store, err
}

// call posts request as JSON to path and decodes the JSON reply into reply.
func (c *Client) call(path string, request, reply any) error {
	body, err := json.Marshal(request)
	if err != nil {
		return err
	}
	response, err := c.do(http.MethodPost, c.base+path, "application/json", body)
	if err != nil {
		return err
	}
	return decode(response, reply)
}

// get fetches path and decodes the JSON reply into reply.
func (c *Client) get(path string, reply any) error {
	response, err := c.do(http.MethodGet, c.base+path, "", nil)
	if err != nil {
		return err
	}
	return decode(response, reply)
}

func (c *Client) do(method, target, contentType string, body []byte) (*http.Response, error) {
	request, err := http.NewRequest(method, target, bytes.NewReader(body))
	if err != nil {
		return nil, err
	}
	if contentType != "" {
		request.Header.Set("Content-Type", contentType)
	}
	request.Header.Set(cabinetHeader, c.cabinet)
	return c.http.Do(request)
}

// decode reads a JSON reply, or turns an error status into an error.
func decode(response *http.Response, reply any) error {
	defer response.Body.Close()
	switch {
	case response.StatusCode == http.StatusNotFound:
		return fmt.Errorf("%w: %s", errNotFound, response.Request.URL.Path)
	case response.StatusCode != http.StatusOK:
		message, _ := io.ReadAll(io.LimitReader(response.Body, 512))
		return fmt.Errorf("remote: %s: %s", response.Status, bytes.TrimSpace(message))
	}
	return json.NewDecoder(response.Body).Decode(reply)
}
