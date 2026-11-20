//go:build windows

// Command l4ds-aio runs the whole server on one PC, with the game's NESYS pipe served in the same
// process: the pipe service, the game server, the matching master, and the dedicated servers that
// host rooms.
package main

import (
	"context"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/pipe"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/server"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/shim"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

func main() {
	if err := run(); err != nil {
		slog.Error("l4ds-aio", "error", err)
		os.Exit(1)
	}
}

func run() error {
	path := flag.String("pipe", pipe.DefaultPath, "named pipe the game connects to")
	flags := server.NewFlags(flag.CommandLine)
	flag.Parse()
	settings, options, err := flags.Load()
	if err != nil {
		return err
	}
	store, listed := settings.Server.Store(settings.Shim.Store)
	if !listed {
		return fmt.Errorf("shim.store %d is not one of server.stores", settings.Shim.Store)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	s, err := server.Start(ctx, options)
	if err != nil {
		stop()
		return err
	}
	defer func() {
		stop()
		s.Close()
	}()

	go shim.RunCardKeys(ctx, settings.Shim.Cards)
	cabinet := wire.StoreInfo{ID: store.ID, Name: store.Name, Prefecture: store.Prefecture}
	return shim.Run(ctx, *path, shim.Service(cabinet, settings.Shim.ServerKey, s.Backend))
}
