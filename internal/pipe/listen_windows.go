// Package pipe creates the Windows named pipe the game connects to.
package pipe

import (
	"net"

	"github.com/Microsoft/go-winio"
)

// DefaultPath is the pipe name the game's NESYS library opens.
const DefaultPath = `\\.\pipe\nesys_games`

// authenticatedUsersSDDL lets any signed-in local user, including the game, connect to the pipe.
const authenticatedUsersSDDL = "D:P(A;;GA;;;AU)"

// Listen creates the message-mode pipe at path. The pipe must not already exist, so it fails
// while Taito's own service or another emulator holds the name.
func Listen(path string) (net.Listener, error) {
	return winio.ListenPipe(path, &winio.PipeConfig{
		SecurityDescriptor: authenticatedUsersSDDL,
		MessageMode:        true,
		InputBufferSize:    8192,
		OutputBufferSize:   8192,
	})
}
