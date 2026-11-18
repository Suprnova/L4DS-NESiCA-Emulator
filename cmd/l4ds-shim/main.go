//go:build windows

// Command l4ds-shim stands in for Taito's NesysService on a cabinet or player PC: it serves the
// game's NESYS pipe and sends the game's card, ranking, and game server requests to an
// l4ds-server.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"strings"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/pipe"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/remote"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/shim"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// retryInterval is how long the shim waits between attempts to reach the server at startup.
const retryInterval = 10 * time.Second

func main() {
	if err := run(); err != nil {
		slog.Error("l4ds-shim", "error", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("pipe", pipe.DefaultPath, "named pipe the game connects to")
	configPath := flag.String("config", "", "YAML configuration file (see l4ds.example.yaml); without it, store 1 on a server at http://127.0.0.1:8080")
	flag.Parse()
	settings := config.Default()
	if *configPath != "" {
		var err error
		if settings, err = config.Load(*configPath); err != nil {
			return err
		}
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	client := remote.NewClient(strings.TrimSuffix(settings.Shim.Server, "/"), fmt.Sprintf("%d/%s", settings.Shim.Store, shim.MAC))
	store, err := connect(ctx, client, settings.Shim.Store)
	if err != nil {
		return err
	}

	cabinet := wire.StoreInfo{ID: store.ID, Name: store.Name, Prefecture: store.Prefecture}
	return shim.Run(ctx, *path, shim.Service(cabinet, settings.Shim.ServerKey, client))
}

// connect waits until the server answers, and returns the store it lists under id. A server that
// does not list the store, or speaks another protocol, is an error; an unreachable one is retried.
func connect(ctx context.Context, client *remote.Client, id uint32) (remote.Store, error) {
	for {
		err := client.Hello()
		if err == nil {
			var store remote.Store
			if store, err = client.Store(id); err == nil {
				slog.Info("connected to the server", "store", store.ID, "name", store.Name)
				return store, nil
			}
		}
		if errors.Is(err, remote.ErrUnknownStore) || errors.Is(err, remote.ErrProtocol) {
			return remote.Store{}, fmt.Errorf("store %d: %w", id, err)
		}
		slog.Warn("waiting for the server", "error", err)
		select {
		case <-ctx.Done():
			return remote.Store{}, ctx.Err()
		case <-time.After(retryInterval):
		}
	}
}
