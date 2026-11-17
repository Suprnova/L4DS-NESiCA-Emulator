//go:build windows

// Command l4ds-aio runs the whole server on one PC: the NESYS pipe service, the game server, the
// matching master and the dedicated servers that host rooms.
package main

import (
	"bytes"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"os"
	"os/signal"
	"path/filepath"
	"strconv"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/dedicated"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/matching"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/pipe"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

func main() {
	path := flag.String("pipe", pipe.DefaultPath, "named pipe the game connects to")
	configPath := flag.String("config", "", "YAML configuration file with the store, the server key and the versus and event schedule (see l4ds.example.yaml); without it the store is \"L4DS Emulator\" in Tokyo and nothing is scheduled")
	dbPath := flag.String("db", "l4ds.db", "SQLite database holding the players")
	gameDir := flag.String("game-dir", "", "game install folder; its level table sets player levels, its map table the stage list, and its versus class table the class table versus results need (without it every player is level 1 and a versus round crashes the game when it ends)")
	masterAddr := flag.String("master", ":27010", "UDP address of the matching master")
	gameServerAddr := flag.String("game-server", "", "UDP address to log game server queries on, such as :27015; it blocks the game's own listen server there")
	roomServer := flag.String("room-server", "", "IPv4 address and port of a dedicated server, started by hand, that hosts every new room, such as 127.0.0.2:27020")
	dsDir := flag.String("ds-dir", "", "server tree built by l4ds-tree; l4ds-aio then runs a dedicated server for each new room (without it or -room-server, rooms are on the creating machine's port 27015)")
	dsIP := flag.String("ds-ip", "127.0.0.2", "IPv4 address the dedicated servers bind to and rooms are advertised at; not 127.0.0.1, which the game never sends to over the network")
	dsPort := flag.Uint("ds-port", 27020, "game port of the first dedicated server; the others use the ports after it")
	dsMax := flag.Int("ds-max", 4, "most dedicated servers running at once")
	dsSpares := flag.Int("ds-spares", 1, "dedicated servers kept ready for new rooms")
	dsConsole := flag.Bool("ds-console", false, "show each dedicated server's console window")
	flag.Parse()

	settings := config.Default()
	if *configPath != "" {
		var err error
		if settings, err = config.Load(*configPath); err != nil {
			slog.Error("load configuration", "error", err)
			os.Exit(1)
		}
	}
	schedule, err := settings.Schedule()
	if err != nil {
		slog.Error("load configuration", "error", err)
		os.Exit(1)
	}

	dsConfig := dedicated.Config{Dir: *dsDir, Max: *dsMax, Spares: *dsSpares, ShowConsoles: *dsConsole}
	hosts, pool, err := roomHosts(*roomServer, *dsIP, *dsPort, dsConfig)
	if err != nil {
		slog.Error("room servers", "error", err)
		os.Exit(1)
	}

	catalog := zet.Catalog{StoreName: settings.Store.Name, Schedule: schedule}
	if *gameDir != "" {
		folder, err := game.LoadFolder(*gameDir)
		if err != nil {
			slog.Error("load game folder", "game-dir", *gameDir, "error", err)
			os.Exit(1)
		}
		catalog.Levels, catalog.VersusClasses, catalog.Stages = folder.Levels, folder.VersusClasses, folder.Stages
	}

	players, err := store.Open(*dbPath)
	if err != nil {
		slog.Error("open database", "path", *dbPath, "error", err)
		os.Exit(1)
	}
	defer players.Close()
	rooms := matching.NewMaster(matching.Config{
		Hosts:       hosts,
		PlayerName:  playerName(players),
		StoreName:   settings.Store.Name,
		VersusClass: versusClass(players),
	})
	catalog.Leave = rooms.Leave
	server := zet.NewServer(players, catalog)

	service := nesys.Config{
		Store:          wire.StoreInfo{ID: 1, Name: settings.Store.Name, Prefecture: settings.Store.Prefecture},
		ServerKey:      settings.ServerKey,
		Adapter:        wire.AdapterDetails{IP: "127.0.0.1", MAC: "020000000001"},
		Public:         wire.PublicAddress{IP: "127.0.0.1"},
		ServiceVersion: "2.79 2014/07/08",
		HTTP:           logRequests(server.Handler()),
		Cards:          server,
		Rankings:       server,
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
	go rooms.Serve(master)

	if *gameServerAddr != "" {
		gameServer, err := net.ListenPacket("udp", *gameServerAddr)
		if err != nil {
			slog.Error("listen for game server queries", "address", *gameServerAddr, "error", err)
			os.Exit(1)
		}
		defer gameServer.Close()
		go matching.ServeGameServer(gameServer)
	}

	if pool != nil {
		poolDone := make(chan struct{})
		go func() {
			pool.Run(ctx)
			close(poolDone)
		}()
		defer func() {
			stop()
			<-poolDone
		}()
	}

	slog.Info("serving", "pipe", *path, "server-key", settings.ServerKey, "versus units", len(schedule.Versus), "events", len(schedule.Events))
	for {
		conn, err := listener.Accept()
		if err != nil {
			if ctx.Err() == nil {
				slog.Error("accept", "error", err)
			}
			return
		}
		go serve(conn, service)
	}
}

// roomHosts returns the servers that host new rooms: a pool of dedicated servers run from
// config.Dir on ip from firstPort, a server started by hand at roomServer, or nil for the
// creating machine's own listen server.
func roomHosts(roomServer, ip string, firstPort uint, config dedicated.Config) (matching.Hosts, *dedicated.Pool, error) {
	switch {
	case roomServer != "" && config.Dir != "":
		return nil, nil, errors.New("-room-server and -ds-dir cannot be combined")
	case roomServer != "":
		address, err := netip.ParseAddrPort(roomServer)
		if err != nil || !address.Addr().Is4() {
			return nil, nil, fmt.Errorf("-room-server %q is not an IPv4 address and port", roomServer)
		}
		return matching.FixedHost(address), nil, nil
	case config.Dir != "":
		var err error
		if config.IP, err = netip.ParseAddr(ip); err != nil || !config.IP.Is4() {
			return nil, nil, fmt.Errorf("-ds-ip %q is not an IPv4 address", ip)
		}
		if firstPort == 0 || firstPort+uint(config.Max) > 65535 {
			return nil, nil, fmt.Errorf("-ds-port %d leaves no room for %d servers", firstPort, config.Max)
		}
		config.FirstPort = uint16(firstPort)
		if config.Dir, err = filepath.Abs(config.Dir); err != nil {
			return nil, nil, err
		}
		pool, err := dedicated.NewPool(config)
		if err != nil {
			return nil, nil, err
		}
		return pool, pool, nil
	}
	return nil, nil, nil
}

// playerName returns a lookup of player names for room lists, falling back to the player id.
func playerName(players *store.Store) func(int64) string {
	return func(id int64) string {
		if player, err := players.Player(id); err == nil && player.Name != "" {
			return player.Name
		}
		return strconv.FormatInt(id, 10)
	}
}

// versusClass returns a lookup of players' versus classes for versus rooms, 0 for an unknown
// player.
func versusClass(players *store.Store) func(int64) int {
	return func(id int64) int {
		if player, err := players.Player(id); err == nil {
			return int(player.VSClassID)
		}
		return 0
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
