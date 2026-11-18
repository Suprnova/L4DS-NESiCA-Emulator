//go:build windows

// Command l4ds-shim stands in for Taito's NesysService on a cabinet or player PC: it serves the
// game's NESYS pipe and sends the game's card, ranking, and game server requests to an
// l4ds-server. While the server cannot be reached, the game is told it is offline; a server that
// does not list the cabinet's store gets the game told the store is unregistered.
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

// checkInterval is how often the shim checks that the server answers.
const checkInterval = 10 * time.Second

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

	client := remote.NewClient(strings.TrimSuffix(settings.Shim.Server, "/"), fmt.Sprintf("%d/%s", settings.Shim.Store, shim.MAC))
	if err := client.Hello(); errors.Is(err, remote.ErrProtocol) {
		return err
	}
	link := shim.NewLink(client, settings.Shim.Store, settings.Shim.ServerKey)
	link.Check()

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go link.Run(ctx, checkInterval)

	service := shim.Service(wire.StoreInfo{}, settings.Shim.ServerKey, client)
	service.Status = link.Status()
	return shim.Run(ctx, *path, service)
}
