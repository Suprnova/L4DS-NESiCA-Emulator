//go:build windows

package dedicated

import (
	"crypto/rand"
	"fmt"
	"net/netip"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"time"

	"l4ds-nesica-emulator/internal/rcon"
)

// NewPool returns a pool that runs config.Dir's srcds.exe. The stub passes its arguments to the
// engine as one space-separated line, so config.Dir must not contain spaces.
func NewPool(config Config) (*Pool, error) {
	if strings.ContainsAny(config.Dir, " \t") {
		return nil, fmt.Errorf("dedicated server folder %q contains a space", config.Dir)
	}
	if _, err := os.Stat(filepath.Join(config.Dir, "srcds.exe")); err != nil {
		return nil, err
	}

	start := func(port uint16, password string) (process, error) { return startServer(config, port, password) }
	return newPool(config, start, queryStatus, rand.Text), nil
}

// startServer runs a server on port. It loads c1m1_hotel; a room's reservation changes the map.
// Its console log goes to left4dead2\logs\server-<port>.log in the tree.
func startServer(config Config, port uint16, password string) (process, error) {
	cmd := exec.Command(filepath.Join(config.Dir, "srcds.exe"),
		"-console", "-game", "left4dead2", "-basedir", config.Dir, "-insecure",
		"+ip", config.IP.String(), "-port", strconv.Itoa(int(port)),
		"+sv_lan", "1", "+sv_allow_lobby_connect_only", "0",
		"+con_logfile", fmt.Sprintf("logs/server-%d.log", port),
		"+rcon_password", password, "+map", "c1m1_hotel")
	cmd.Dir = config.Dir
	cmd.Env = append(os.Environ(), "L4DS_DIR="+config.Dir)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if err := cmd.Start(); err != nil {
		return nil, err
	}

	p := &cmdProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

type cmdProcess struct {
	cmd  *exec.Cmd
	done chan struct{}
}

func (p *cmdProcess) Kill()                 { p.cmd.Process.Kill() }
func (p *cmdProcess) Done() <-chan struct{} { return p.done }

func queryStatus(address netip.AddrPort, password string) (Status, error) {
	client, err := rcon.Dial(address.String(), password, 2*time.Second)
	if err != nil {
		return Status{}, err
	}
	defer client.Close()

	output, err := client.Exec("status")
	if err != nil {
		return Status{}, err
	}
	return ParseStatus(output), nil
}
