// Package nesys implements the service end of the NESYS pipe: the part of Taito's
// NesysService that answers the game's startup and network queries.
package nesys

import (
	"errors"
	"io"

	"l4ds-nesica-emulator/internal/wire"
)

// Config holds the values the service reports to the game.
type Config struct {
	Store          wire.StoreInfo
	News           wire.NewsImage
	ServerKey      string
	Adapter        wire.AdapterDetails
	Public         wire.PublicAddress
	ServiceVersion string
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
	case wire.ClientEnd:
		return []wire.Frame{{Command: wire.ClientEndReply}}
	default:
		return nil
	}
}
