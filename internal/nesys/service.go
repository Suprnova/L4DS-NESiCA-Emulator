// Package nesys implements the service end of the NESYS pipe: the part of Taito's
// NesysService that answers the game's startup and network queries.
package nesys

import (
	"errors"
	"fmt"
	"io"
	"log/slog"
	"sync"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// Config holds the values the service reports to the game.
type Config struct {
	Store          wire.StoreInfo
	News           wire.NewsImage
	ServerKey      string
	Adapter        wire.AdapterDetails
	Public         wire.PublicAddress
	ServiceVersion string

	// Backend answers the tunnelled HTTP requests, the card commands, and the rankings. Without
	// one, every tunnelled request fails with an error code, card commands go unanswered, and
	// every ranking is empty.
	Backend Backend

	// Status, when set, decides what a game is told after it connects, and tells connected games
	// when it changes. Without one, a game is always certified in Store.
	Status *Status
}

// Backend answers what the game asks of the network: the HTTP requests it tunnels through the
// service, its card commands, and its ranking requests. The game server implements it in
// process; the shim implements it with requests to a server.
type Backend interface {
	// Post answers a tunnelled HTTP POST to url, a URL on the game's lobby host, with the reply's
	// status code and body. An error means no reply could be had.
	Post(url string, body []byte) (status int, reply []byte, err error)
	Cards
	Rankings
}

// Rankings answers the game's ranking requests.
type Rankings interface {
	// Ranking returns the reply body of the ranking of the given kind, asked for by a cabinet of
	// the given store (0 when the request names none): result, API_ID, the entry count and the
	// entries.
	Ranking(kind, store uint32) (string, error)
}

// Cards answers the game's card commands.
type Cards interface {
	// Select returns the reply describing the player registered to a card. A card with no
	// player gets wire.CardStatusUnregistered.
	Select(cardID string) (wire.CardReply, error)

	// ReissueTest answers whether a card was reissued from another card.
	ReissueTest(cardID string) (wire.CardReply, error)

	// Insert registers a player to a card from the game's comma-separated key=value text and
	// returns the reply body.
	Insert(cardID, data string) (string, error)

	// Update stores the comma-separated key=value text the game writes to a card; dataType
	// says what the text holds.
	Update(cardID string, dataType uint32, data string) error
}

// Serve answers the game's commands on conn until the game ends the session or the
// connection fails, and passes on the status's changes meanwhile. It returns nil after a clean
// ClientEnd.
func Serve(conn io.ReadWriter, config Config) error {
	var mu sync.Mutex
	write := func(frames []wire.Frame) error {
		mu.Lock()
		defer mu.Unlock()
		for _, frame := range frames {
			if _, err := conn.Write(frame.Encode()); err != nil {
				return err
			}
		}
		return nil
	}

	if config.Status != nil {
		changes, unsubscribe := config.Status.subscribe()
		defer unsubscribe()
		go func() {
			for change := range changes {
				for _, notice := range change {
					slog.Info("status notice", "id", fmt.Sprintf("%#x", uint32(notice.Command)))
				}
				write(change)
			}
		}()
	}

	for {
		request, err := wire.ReadFrame(conn)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		slog.Info("command", "id", fmt.Sprintf("%#x", uint32(request.Command)), "payload", len(request.Payload), "head", fmt.Sprintf("%x", request.Payload[:min(len(request.Payload), 32)]))

		if err := write(respond(config, request)); err != nil {
			return err
		}
		if request.Command == wire.ClientEnd {
			return nil
		}
	}
}

func respond(config Config, request wire.Frame) []wire.Frame {
	switch request.Command {
	case wire.ClientStart:
		return []wire.Frame{{Command: wire.ClientStartReply}}
	case wire.ConnectRequest:
		if config.Status != nil {
			return append([]wire.Frame{{Command: wire.ConnectReply}}, config.Status.Current()...)
		}
		return []wire.Frame{{Command: wire.ConnectReply}, CertInit(config.Store, config.News, config.ServerKey)}
	case wire.DisconnectRequest:
		return []wire.Frame{{Command: wire.DisconnectReply}}
	case wire.SetIncomeMode:
		return []wire.Frame{{Command: wire.SetIncomeModeReply, Payload: []byte{1, 0, 0, 0}}}
	case wire.ServiceVersion:
		return []wire.Frame{{Command: wire.ServiceVersionReply, Payload: wire.EncodeServiceVersion(config.ServiceVersion)}}
	case wire.AdapterInfo:
		return []wire.Frame{{Command: wire.AdapterInfoReply, Payload: config.Adapter.Encode()}}
	case wire.GlobalAddress:
		return []wire.Frame{{Command: wire.GlobalAddressReply, Payload: config.Public.Encode()}}
	case wire.RowEventDataList:
		return []wire.Frame{{Command: wire.RowEventDataReply, Payload: wire.EmptyEventList()}}
	case wire.HTTPPost:
		return tunnel(config.Backend, request.Payload)
	case wire.GameStart, wire.GameEnd, wire.GameContinue:
		slog.Info("stub reply", "command", fmt.Sprintf("%#x", uint32(request.Command)), "ignored", "game status bookkeeping; reply layout guessed")
		return []wire.Frame{{Command: wire.GameStatusReply, Payload: []byte{1, 0, 0, 0}}}
	case wire.IncomeStart, wire.IncomeEnd:
		slog.Info("stub reply", "command", fmt.Sprintf("%#x", uint32(request.Command)), "ignored", "income bookkeeping; reply layout guessed")
		return []wire.Frame{{Command: wire.IncomeStatusReply, Payload: []byte{1, 0, 0, 0, 1, 0, 0, 0}}}
	case wire.Shopping:
		slog.Info("stub reply", "command", "SHOPPING", "ignored", "purchase; reply layout guessed")
		return []wire.Frame{{Command: wire.ShoppingReply, Payload: []byte{1, 0, 0, 0}}}
	case wire.CardUpdate:
		return updateCard(config.Backend, request.Payload)
	case wire.RankingData:
		return ranking(config.Backend, request.Payload)
	case wire.CardReissueTest:
		return reissueTest(config.Backend, request.Payload)
	case wire.CardInsert:
		return insertCard(config.Backend, request.Payload)
	case wire.CardSelect:
		return selectCard(config.Backend, request.Payload)
	case wire.ClientEnd:
		return []wire.Frame{{Command: wire.ClientEndReply}}
	default:
		slog.Warn("unanswered command", "id", fmt.Sprintf("%#x", uint32(request.Command)))
		return nil
	}
}

// tunnelFailure is the error code reported when a tunnelled request cannot be answered.
const tunnelFailure = 1

func tunnel(backend Backend, payload []byte) []wire.Frame {
	request, err := wire.ParseHTTPRequest(payload)
	if err != nil {
		return nil
	}

	response := wire.HTTPResponse{Sequence: request.Sequence, ErrorCode: tunnelFailure}
	if backend != nil {
		if status, body, err := backend.Post(request.URL, request.Body); err != nil {
			slog.Warn("tunnelled request failed", "url", request.URL, "error", err)
		} else {
			response = wire.HTTPResponse{StatusCode: uint32(status), Sequence: request.Sequence, Body: body}
		}
	}

	return []wire.Frame{
		{Command: wire.HTTPStart, Payload: wire.HTTPStartPayload()},
		{Command: wire.HTTPReply, Payload: response.Encode()},
	}
}

func selectCard(cards Cards, payload []byte) []wire.Frame {
	request, err := wire.ParseCardSelectRequest(payload)
	if err != nil || cards == nil {
		return nil
	}

	slog.Info("card select", "card", request.CardID, "kind", request.Kind, "data", request.Data)
	reply, err := cards.Select(request.CardID)
	if err != nil {
		slog.Error("card select", "card", request.CardID, "error", err)
		return nil
	}
	return []wire.Frame{{Command: wire.CardSelectReply, Payload: reply.Encode()}}
}

// updateCard stores a card update and acknowledges it. The game has nothing to do on a failure,
// so a failed update is logged and acknowledged too.
func updateCard(cards Cards, payload []byte) []wire.Frame {
	request, err := wire.ParseCardWriteRequest(payload)
	if err != nil {
		slog.Warn("card update", "error", err, "payload", fmt.Sprintf("%q", payload))
	} else {
		slog.Info("card update", "card", request.CardID, "type", request.DataType, "data", string(request.Data))
		if cards != nil {
			if err := cards.Update(request.CardID, request.DataType, string(request.Data)); err != nil {
				slog.Error("card update", "card", request.CardID, "error", err)
			}
		}
	}

	reply := wire.CardReply{Status: 1, Body: "1\n1\n"}
	return []wire.Frame{{Command: wire.CardUpdateReply, Payload: reply.Encode()}}
}

func insertCard(cards Cards, payload []byte) []wire.Frame {
	request, err := wire.ParseCardWriteRequest(payload)
	if err != nil || cards == nil {
		return nil
	}

	slog.Info("card insert", "card", request.CardID, "data", string(request.Data))
	body, err := cards.Insert(request.CardID, string(request.Data))
	if err != nil {
		slog.Error("card insert", "card", request.CardID, "error", err)
		return nil
	}

	reply := wire.CardReply{Status: 1, Body: body}
	return []wire.Frame{{Command: wire.CardInsertReply, Payload: reply.Encode()}}
}

// emptyRanking is the body of a ranking with no entries.
const emptyRanking = "1\n1\n0\n"

// ranking answers a ranking request. The game fetches four rankings at startup and waits for
// each reply before it fetches the next startup data (the stage list, events, versus), so a
// request that cannot be answered gets an empty ranking.
func ranking(rankings Rankings, payload []byte) []wire.Frame {
	body := emptyRanking
	if request, err := wire.ParseRankingRequest(payload); err != nil || rankings == nil {
		slog.Info("stub reply", "command", "RANKING_DATA", "ignored", "rankings: no ranking source")
	} else if body, err = rankings.Ranking(request.Kind, request.Store); err != nil {
		slog.Error("ranking", "kind", request.Kind, "store", request.Store, "error", err)
		body = emptyRanking
	}
	reply := wire.RankingReply{Status: 1, Body: body}
	return []wire.Frame{{Command: wire.RankingDataReply, Payload: reply.Encode()}}
}

func reissueTest(cards Cards, payload []byte) []wire.Frame {
	cardID, err := wire.ParseCardID(payload)
	if err != nil || cards == nil {
		return nil
	}

	slog.Info("card reissue test", "card", cardID)
	reply, err := cards.ReissueTest(cardID)
	if err != nil {
		slog.Error("card reissue test", "card", cardID, "error", err)
		return nil
	}
	return []wire.Frame{{Command: wire.CardReissueTestReply, Payload: reply.Encode()}}
}
