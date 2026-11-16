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
	"time"
	"unsafe"

	"golang.org/x/sys/windows"

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
	job, err := killOnCloseJob()
	if err != nil {
		return nil, err
	}

	start := func(port uint16, password string) (process, error) { return startServer(config, job, port, password) }
	return newPool(config, start, queryStatus, rand.Text), nil
}

// killOnCloseJob returns a job object whose processes the system kills when the job's last
// handle closes, which happens when this process exits, however it exits.
func killOnCloseJob() (windows.Handle, error) {
	job, err := windows.CreateJobObject(nil, nil)
	if err != nil {
		return 0, err
	}
	info := windows.JOBOBJECT_EXTENDED_LIMIT_INFORMATION{
		BasicLimitInformation: windows.JOBOBJECT_BASIC_LIMIT_INFORMATION{LimitFlags: windows.JOB_OBJECT_LIMIT_KILL_ON_JOB_CLOSE},
	}
	if _, err := windows.SetInformationJobObject(job, windows.JobObjectExtendedLimitInformation, uintptr(unsafe.Pointer(&info)), uint32(unsafe.Sizeof(info))); err != nil {
		windows.CloseHandle(job)
		return 0, err
	}
	return job, nil
}

// startServer runs a server on port in job, in a console window of its own. It loads
// c1m1_hotel; a room's reservation changes the map.
func startServer(config Config, job windows.Handle, port uint16, password string) (process, error) {
	cmd := exec.Command(filepath.Join(config.Dir, "srcds.exe"),
		"-console", "-game", "left4dead2", "-basedir", config.Dir, "-insecure",
		"+ip", config.IP.String(), "-port", strconv.Itoa(int(port)),
		"+sv_lan", "1", "+sv_allow_lobby_connect_only", "0",
		"+rcon_password", password, "+map", "c1m1_hotel")
	cmd.Dir = config.Dir
	cmd.Env = append(os.Environ(), "L4DS_DIR="+config.Dir)
	if err := cmd.Start(); err != nil {
		return nil, err
	}
	if err := assign(job, cmd.Process.Pid); err != nil {
		cmd.Process.Kill()
		cmd.Wait()
		return nil, fmt.Errorf("add server to job: %w", err)
	}

	p := &cmdProcess{cmd: cmd, done: make(chan struct{})}
	go func() {
		cmd.Wait()
		close(p.done)
	}()
	return p, nil
}

func assign(job windows.Handle, pid int) error {
	process, err := windows.OpenProcess(windows.PROCESS_SET_QUOTA|windows.PROCESS_TERMINATE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer windows.CloseHandle(process)
	return windows.AssignProcessToJobObject(job, process)
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
