package server

import (
	"fmt"
	"net/netip"
	"path/filepath"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/dedicated"
)

// newPool checks the pool's addresses and folder and returns the pool.
func newPool(rooms Rooms) (*dedicated.Pool, error) {
	config := rooms.Pool
	var err error
	if config.IP, err = netip.ParseAddr(rooms.IP); err != nil || !config.IP.Is4() {
		return nil, fmt.Errorf("dedicated server address %q is not an IPv4 address", rooms.IP)
	}
	if rooms.FirstPort == 0 || rooms.FirstPort+uint(config.Max) > 65535 {
		return nil, fmt.Errorf("dedicated server port %d leaves no room for %d servers", rooms.FirstPort, config.Max)
	}
	config.FirstPort = uint16(rooms.FirstPort)
	if config.Dir, err = filepath.Abs(config.Dir); err != nil {
		return nil, err
	}
	return dedicated.NewPool(config)
}
