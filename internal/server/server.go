// Package server runs the server: the database, the game server, the matching master, the room
// servers, and the endpoint shims connect to. l4ds-server and l4ds-aio both run one.
package server

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/netip"
	"strconv"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/dedicated"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/matching"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/remote"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

// Options says what a server runs and where.
type Options struct {
	// Config is the server section of the configuration file.
	Config config.Server
	// Database is the SQLite file holding the players.
	Database string
	// Master is the UDP address of the matching master, such as ":27010".
	Master string
	// ServeShims serves shims on Config.Listen; l4ds-aio, whose shim runs in process, leaves it off.
	ServeShims bool
	// Rooms says which game servers host new rooms.
	Rooms Rooms
	// GameServerLog, when set, is a UDP address whose datagrams are logged, for debugging.
	GameServerLog string
}

// Rooms says which game servers host new rooms: a pool of dedicated servers run from a server
// tree (Windows only), one dedicated server started by hand, or, with neither, the listen server
// of the machine that creates the room.
type Rooms struct {
	// Pool configures the dedicated servers; its Dir turns the pool on.
	Pool dedicated.Config
	// IP is the address the pool's servers bind to and rooms are advertised at.
	IP string
	// FirstPort is the game port of the pool's first server.
	FirstPort uint
	// Fixed is the address of a dedicated server started by hand, such as 127.0.0.2:27020.
	Fixed string
}

// Server is a running server.
type Server struct {
	// Backend answers the game's requests, for a shim running in the same process.
	Backend *zet.Server

	players  *store.Store
	closers  []func() error
	poolDone chan struct{}
}

// Start opens the database and starts the matching master, the room servers, and, when asked,
// the endpoint shims connect to. Cancelling ctx stops the room servers; Close stops the rest.
func Start(ctx context.Context, options Options) (*Server, error) {
	schedule, err := options.Config.Schedule()
	if err != nil {
		return nil, err
	}
	hosts, pool, err := roomHosts(options.Rooms)
	if err != nil {
		return nil, err
	}
	players, err := store.Open(options.Database)
	if err != nil {
		return nil, err
	}
	s := &Server{players: players, closers: []func() error{players.Close}}

	storeName := storeNames(options.Config.Stores)
	rooms := matching.NewMaster(matching.Config{
		Hosts:       hosts,
		PlayerName:  playerName(players),
		PlayerStore: playerStore(players, storeName),
		VersusClass: versusClass(players),
	})
	s.Backend = zet.NewServer(players, zet.Catalog{
		Levels:        game.Levels,
		Stages:        game.Stages,
		VersusClasses: game.VersusClasses,
		Schedule:      schedule,
		StoreName:     storeName,
		Leave:         rooms.Leave,
	})

	master, err := net.ListenPacket("udp", options.Master)
	if err != nil {
		s.Close()
		return nil, fmt.Errorf("matching master on %s: %w", options.Master, err)
	}
	s.closers = append(s.closers, master.Close)
	go rooms.Serve(master)

	if options.GameServerLog != "" {
		logged, err := net.ListenPacket("udp", options.GameServerLog)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("game server log on %s: %w", options.GameServerLog, err)
		}
		s.closers = append(s.closers, logged.Close)
		go matching.ServeGameServer(logged)
	}

	if options.ServeShims {
		listener, err := net.Listen("tcp", options.Config.Listen)
		if err != nil {
			s.Close()
			return nil, fmt.Errorf("shim endpoint on %s: %w", options.Config.Listen, err)
		}
		shims := &http.Server{Handler: remote.Handler(s.Backend, remoteStores(options.Config.Stores))}
		s.closers = append(s.closers, shims.Close)
		go shims.Serve(listener)
		slog.Info("serving shims", "address", options.Config.Listen)
	}

	if pool != nil {
		s.poolDone = make(chan struct{})
		go func() {
			pool.Run(ctx)
			close(s.poolDone)
		}()
	}
	slog.Info("server started", "stores", len(options.Config.Stores), "versus units", len(schedule.Versus), "events", len(schedule.Events))
	return s, nil
}

// Close stops the server, after waiting for the room servers to stop once ctx is cancelled.
func (s *Server) Close() error {
	if s.poolDone != nil {
		<-s.poolDone
	}
	var errs []error
	for _, closer := range s.closers {
		errs = append(errs, closer())
	}
	return errors.Join(errs...)
}

// roomHosts returns the hosts for new rooms and, when they are a pool of dedicated servers, the
// pool to run.
func roomHosts(rooms Rooms) (matching.Hosts, *dedicated.Pool, error) {
	switch {
	case rooms.Fixed != "" && rooms.Pool.Dir != "":
		return nil, nil, errors.New("a fixed room server and a dedicated server pool cannot be combined")
	case rooms.Fixed != "":
		address, err := netip.ParseAddrPort(rooms.Fixed)
		if err != nil || !address.Addr().Is4() {
			return nil, nil, fmt.Errorf("room server %q is not an IPv4 address and port", rooms.Fixed)
		}
		return matching.FixedHost(address), nil, nil
	case rooms.Pool.Dir != "":
		pool, err := newPool(rooms)
		if err != nil {
			return nil, nil, err
		}
		return pool, pool, nil
	}
	return nil, nil, nil
}

// remoteStores converts the configured stores for the shim endpoint.
func remoteStores(stores []config.Store) []remote.Store {
	converted := make([]remote.Store, len(stores))
	for i, s := range stores {
		converted[i] = remote.Store{ID: s.ID, Name: s.Name, Prefecture: s.Prefecture}
	}
	return converted
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

// storeNames returns a lookup of the names of the listed stores, "" for a store not listed.
func storeNames(stores []config.Store) func(int64) string {
	names := map[int64]string{}
	for _, s := range stores {
		names[int64(s.ID)] = s.Name
	}
	return func(id int64) string { return names[id] }
}

// playerStore returns a lookup of the names of players' home stores for room lists, "" for an
// unknown player or a store no longer listed.
func playerStore(players *store.Store, storeName func(int64) string) func(int64) string {
	return func(id int64) string {
		if player, err := players.Player(id); err == nil {
			return storeName(player.HomeStore)
		}
		return ""
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
