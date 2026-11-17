// Package nesys implements the service end of the NESYS pipe: the part of Taito's
// NesysService that answers the game's startup and network queries.
package nesys

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"

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

	// HTTP answers the requests the game tunnels through the service. A nil handler makes
	// every tunnelled request fail with an error code.
	HTTP http.Handler

	// Cards answers the card commands. A nil value leaves them unanswered.
	Cards Cards

	// Rankings answers the ranking requests. A nil value answers every ranking with an empty
	// one.
	Rankings Rankings
}

// Rankings answers the game's ranking requests.
type Rankings interface {
	// Ranking returns the reply body of the ranking of the given kind: result, API_ID, the
	// entry count and the entries.
	Ranking(kind uint32) (string, error)
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
// connection fails. It returns nil after a clean ClientEnd.
func Serve(conn io.ReadWriter, config Config) error {
	for {
		request, err := wire.ReadFrame(conn)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}

		slog.Info("command", "id", fmt.Sprintf("%#x", uint32(request.Command)), "payload", len(request.Payload), "head", fmt.Sprintf("%x", request.Payload[:min(len(request.Payload), 32)]))

		for _, reply := range respond(config, request) {
			if _, err := conn.Write(reply.Encode()); err != nil {
				return err
			}
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
		init := wire.CertInit{Store: config.Store, News: config.News, ServerKey: config.ServerKey}
		return []wire.Frame{{Command: wire.ConnectReply}, {Command: wire.CertInitNotice, Payload: init.Encode()}}
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
		return tunnel(config.HTTP, request.Payload)
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
		return updateCard(config.Cards, request.Payload)
	case wire.RankingData:
		return ranking(config.Rankings, request.Payload)
	case wire.CardReissueTest:
		return reissueTest(config.Cards, request.Payload)
	case wire.CardInsert:
		return insertCard(config.Cards, request.Payload)
	case wire.CardSelect:
		return selectCard(config.Cards, request.Payload)
	case wire.ClientEnd:
		return []wire.Frame{{Command: wire.ClientEndReply}}
	default:
		slog.Warn("unanswered command", "id", fmt.Sprintf("%#x", uint32(request.Command)))
		return nil
	}
}

// tunnelFailure is the error code reported when a tunnelled request cannot be answered.
const tunnelFailure = 1

func tunnel(handler http.Handler, payload []byte) []wire.Frame {
	request, err := wire.ParseHTTPRequest(payload)
	if err != nil {
		return nil
	}

	response := wire.HTTPResponse{Sequence: request.Sequence, ErrorCode: tunnelFailure}
	if handler != nil {
		recorder := httptest.NewRecorder()
		httpRequest := httptest.NewRequest(http.MethodPost, request.URL, bytes.NewReader(request.Body))
		httpRequest.Header.Set("Content-Type", "application/x-www-form-urlencoded")
		handler.ServeHTTP(recorder, httpRequest)
		response = wire.HTTPResponse{StatusCode: uint32(recorder.Code), Sequence: request.Sequence, Body: recorder.Body.Bytes()}
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
	if kind, err := wire.ParseRankingKind(payload); err != nil || rankings == nil {
		slog.Info("stub reply", "command", "RANKING_DATA", "ignored", "rankings: no ranking source")
	} else if body, err = rankings.Ranking(kind); err != nil {
		slog.Error("ranking", "kind", kind, "error", err)
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
