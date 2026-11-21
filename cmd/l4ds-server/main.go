// Command l4ds-server runs the server a group of cabinets shares: the database, the game server,
// the matching master, the room servers, and the endpoint each cabinet's l4ds-shim connects to.
//
// "l4ds-server grant" instead queues a gift (ZG, an item, or both) for one card, delivered at its
// next login; "l4ds-server grant -h" lists its flags.
package main

import (
	"context"
	"flag"
	"log/slog"
	"os"
	"os/signal"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/server"
)

func main() {
	var err error
	if len(os.Args) > 1 && os.Args[1] == "grant" {
		err = grant(os.Args[2:])
	} else {
		err = run()
	}
	if err != nil {
		slog.Error("l4ds-server", "error", err)
		os.Exit(1)
	}
}

func run() error {
	flags := server.NewFlags(flag.CommandLine)
	flag.Parse()
	_, options, err := flags.Load()
	if err != nil {
		return err
	}
	options.ServeShims = true

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	s, err := server.Start(ctx, options)
	if err != nil {
		return err
	}
	<-ctx.Done()
	return s.Close()
}
