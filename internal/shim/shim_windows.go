package shim

import (
	"context"
	"log/slog"
	"net"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/hotkey"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/pipe"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// MAC is the network adapter address the shim reports to the game.
const MAC = "020000000001"

// Service returns the pipe service's settings for a cabinet in store with serverKey, answered by
// backend.
func Service(store wire.StoreInfo, serverKey string, backend nesys.Backend) nesys.Config {
	return nesys.Config{
		Store:          store,
		ServerKey:      serverKey,
		Adapter:        wire.AdapterDetails{IP: "127.0.0.1", MAC: MAC},
		Public:         wire.PublicAddress{IP: "127.0.0.1"},
		ServiceVersion: "2.79 2014/07/08",
		Backend:        backend,
	}
}

// Run serves the pipe at path with service, one session per game connection, until ctx is done.
func Run(ctx context.Context, path string, service nesys.Config) error {
	listener, err := pipe.Listen(path)
	if err != nil {
		return err
	}
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	slog.Info("serving the game", "pipe", path, "server-key", service.ServerKey)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() != nil {
				return nil
			}
			return err
		}
		go serve(conn, service)
	}
}

// RunCardKeys puts each card on its game's card reader when its key is pressed, until ctx is
// done. A key another program holds leaves every card key off, which it logs.
func RunCardKeys(ctx context.Context, cards []config.CardKey) {
	if len(cards) == 0 {
		return
	}
	keys := make([]hotkey.Key, len(cards))
	for i, card := range cards {
		keys[i] = card.Key
		slog.Info("card key", "key", card.Key, "card", card.Card, "netcon", card.Netcon)
	}
	err := hotkey.Listen(ctx, keys, func(i int) {
		card := cards[i]
		if err := PutCard(card.Netcon, card.Card); err != nil {
			slog.Warn("card not put on the reader", "card", card.Card, "netcon", card.Netcon, "error", err)
			return
		}
		slog.Info("card put on the reader", "card", card.Card, "netcon", card.Netcon)
	})
	if err != nil {
		slog.Error("card keys are off", "error", err)
	}
}

func serve(conn net.Conn, service nesys.Config) {
	defer conn.Close()
	slog.Info("game connected")
	if err := nesys.Serve(conn, service); err != nil {
		slog.Warn("session ended", "error", err)
		return
	}
	slog.Info("game disconnected")
}
