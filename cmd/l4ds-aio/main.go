//go:build windows

// Command l4ds-aio runs the NESYS pipe service for local development.
package main

import (
	"bytes"
	"context"
	"flag"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"

	"l4ds-nesica-emulator/internal/game"
	"l4ds-nesica-emulator/internal/matching"
	"l4ds-nesica-emulator/internal/nesys"
	"l4ds-nesica-emulator/internal/pipe"
	"l4ds-nesica-emulator/internal/store"
	"l4ds-nesica-emulator/internal/wire"
	"l4ds-nesica-emulator/internal/zet"
)

func main() {
	path := flag.String("pipe", pipe.DefaultPath, "named pipe the game connects to")
	serverKey := flag.String("server-key", "dev.game.zet.square-enix.co.jp", "master and lobby host string sent to the game")
	prefecture := flag.String("prefecture", "Tokyo", "prefecture reported for the store; must not be empty")
	dbPath := flag.String("db", "l4ds.db", "SQLite database holding the players")
	gameDir := flag.String("game-dir", "", "game install folder; its level table sets player levels (all players are level 1 without it)")
	masterAddr := flag.String("master", ":27010", "UDP address of the matching master")
	flag.Parse()

	var levels *game.LevelTable
	if *gameDir != "" {
		var err error
		if levels, err = loadLevels(*gameDir); err != nil {
			slog.Error("load level table", "game-dir", *gameDir, "error", err)
			os.Exit(1)
		}
	}

	players, err := store.Open(*dbPath)
	if err != nil {
		slog.Error("open database", "path", *dbPath, "error", err)
		os.Exit(1)
	}
	defer players.Close()
	server := zet.NewServer(players, levels)

	config := nesys.Config{
		Store:          wire.StoreInfo{ID: 1, Name: "L4DS Emulator", Prefecture: *prefecture},
		ServerKey:      *serverKey,
		Adapter:        wire.AdapterDetails{IP: "127.0.0.1", MAC: "020000000001"},
		Public:         wire.PublicAddress{IP: "127.0.0.1"},
		ServiceVersion: "2.79 2014/07/08",
		HTTP:           logRequests(server.Handler()),
		Cards:          server,
	}

	listener, err := pipe.Listen(*path)
	if err != nil {
		slog.Error("listen", "pipe", *path, "error", err)
		os.Exit(1)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()
	go func() {
		<-ctx.Done()
		listener.Close()
	}()

	master, err := net.ListenPacket("udp", *masterAddr)
	if err != nil {
		slog.Error("listen for matching", "address", *masterAddr, "error", err)
		os.Exit(1)
	}
	defer master.Close()
	go matching.NewMaster().Serve(master)

	slog.Info("serving", "pipe", *path, "server-key", *serverKey)
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("accept", "error", err)
			}
			return
		}
		go serve(conn, config)
	}
}

func serve(conn net.Conn, config nesys.Config) {
	defer conn.Close()
	slog.Info("game connected")
	if err := nesys.Serve(conn, config); err != nil {
		slog.Warn("session ended", "error", err)
		return
	}
	slog.Info("game disconnected")
}

func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		slog.Info("http", "path", r.URL.Path, "body", string(body))
		next.ServeHTTP(w, r)
	})
}

func loadLevels(gameDir string) (*game.LevelTable, error) {
	file, err := os.Open(filepath.Join(gameDir, "left4dead2", "resource", "LevelTable.txt"))
	if err != nil {
		return nil, err
	}
	defer file.Close()
	return game.ParseLevelTable(file)
}
