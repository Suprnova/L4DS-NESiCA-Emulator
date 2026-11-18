// Package shim serves the game's NESYS pipe in place of Taito's NesysService, with a backend
// that answers the game's network requests: a server in the same process for l4ds-aio, or a
// remote server for l4ds-shim.
package shim

import (
	"context"
	"log/slog"
	"net"

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

	slog.Info("serving the game", "pipe", path, "store", service.Store.ID, "server-key", service.ServerKey)
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

func serve(conn net.Conn, service nesys.Config) {
	defer conn.Close()
	slog.Info("game connected")
	if err := nesys.Serve(conn, service); err != nil {
		slog.Warn("session ended", "error", err)
		return
	}
	slog.Info("game disconnected")
}
