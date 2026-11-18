//go:build !windows

package server

import (
	"errors"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/dedicated"
)

// newPool refuses: the dedicated servers are the game's Windows binaries.
func newPool(Rooms) (*dedicated.Pool, error) {
	return nil, errors.New("dedicated servers run only on Windows; use a fixed room server instead")
}
