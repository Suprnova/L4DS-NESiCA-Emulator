package server

import (
	"flag"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/dedicated"
)

// Flags are a server's command-line flags, shared by l4ds-server and l4ds-aio.
type Flags struct {
	config, database, master, gameServerLog *string
	roomServer, dsDir, dsIP                 *string
	dsPort                                  *uint
	dsMax, dsSpares                         *int
	dsConsole                               *bool
}

// NewFlags defines a server's flags on flags.
func NewFlags(flags *flag.FlagSet) *Flags {
	return &Flags{
		config:        flags.String("config", "", "YAML configuration file (see l4ds.example.yaml); without it, one store, \"L4DS Emulator\" in Tokyo, and no calendar, so nothing scheduled"),
		database:      flags.String("db", "l4ds.db", "SQLite database holding the players"),
		master:        flags.String("master", ":27010", "UDP address of the matching master"),
		gameServerLog: flags.String("game-server", "", "UDP address to log game server queries on, such as :27015; it blocks the game's own listen server there"),
		roomServer:    flags.String("room-server", "", "IPv4 address and port of a dedicated server, started by hand, that hosts every new room, such as 127.0.0.2:27020"),
		dsDir:         flags.String("ds-dir", "", "server tree built by l4ds-tree; the server then runs a dedicated server for each new room (Windows only; without it or -room-server, rooms are on the creating machine's port 27015)"),
		dsIP:          flags.String("ds-ip", "127.0.0.2", "IPv4 address the dedicated servers bind to and rooms are advertised at; not 127.0.0.1, which the game never sends to over the network"),
		dsPort:        flags.Uint("ds-port", 27020, "game port of the first dedicated server; the others use the ports after it"),
		dsMax:         flags.Int("ds-max", 4, "most dedicated servers running at once"),
		dsSpares:      flags.Int("ds-spares", 1, "dedicated servers kept ready for new rooms"),
		dsConsole:     flags.Bool("ds-console", false, "show each dedicated server's console window"),
	}
}

// Load reads the configuration file the flags name, or the defaults without one, and returns it
// with the server's options.
func (f *Flags) Load() (config.Config, Options, error) {
	settings := config.Default()
	if *f.config != "" {
		var err error
		if settings, err = config.Load(*f.config); err != nil {
			return config.Config{}, Options{}, err
		}
	}
	return settings, Options{
		Config:        settings.Server,
		Database:      *f.database,
		Master:        *f.master,
		GameServerLog: *f.gameServerLog,
		Rooms: Rooms{
			Pool:      dedicated.Config{Dir: *f.dsDir, Max: *f.dsMax, Spares: *f.dsSpares, ShowConsoles: *f.dsConsole},
			IP:        *f.dsIP,
			FirstPort: *f.dsPort,
			Fixed:     *f.roomServer,
		},
	}, nil
}
