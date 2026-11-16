package dedicated

import (
	"errors"
	"net/netip"
	"strconv"
	"testing"
	"time"
)

type fakeProcess struct {
	killed bool
	done   chan struct{}
}

func (p *fakeProcess) Kill()                 { p.killed = true }
func (p *fakeProcess) Done() <-chan struct{} { return p.done }
func (p *fakeProcess) exit()                 { close(p.done) }

// harness drives a pool with fake processes, a fake clock and status replies set per port.
type harness struct {
	pool     *Pool
	clock    time.Time
	procs    map[uint16]*fakeProcess
	statuses map[uint16]Status
}

func newHarness(spares, max int) *harness {
	h := &harness{clock: time.Unix(0, 0), procs: map[uint16]*fakeProcess{}, statuses: map[uint16]Status{}}
	config := Config{IP: netip.MustParseAddr("127.0.0.2"), FirstPort: 27020, Max: max, Spares: spares}
	start := func(port uint16, _ string) (process, error) {
		p := &fakeProcess{done: make(chan struct{})}
		h.procs[port] = p
		return p, nil
	}
	status := func(address netip.AddrPort, _ string) (Status, error) {
		if s, ok := h.statuses[address.Port()]; ok {
			return s, nil
		}
		return Status{}, errors.New("no answer")
	}
	h.pool = newPool(config, start, status, func() string { return "pw" })
	h.pool.now = func() time.Time { return h.clock }
	return h
}

func (h *harness) after(d time.Duration) { h.clock = h.clock.Add(d); h.pool.poll() }

func (h *harness) ready(port uint16) { h.statuses[port] = Status{Map: "c1m1_hotel"} }

func address(port uint16) netip.AddrPort {
	return netip.MustParseAddrPort("127.0.0.2:" + strconv.Itoa(int(port)))
}

func TestParseStatus_ReadsTheMapAndPlayers(t *testing.T) {
	output := "hostname: Left 4 Dead 2\nversion : 2.1.3.0 6374 insecure  (unknown)\nudp/ip  : 127.0.0.2:27020 [ public n/a ]\nmap     : c1m1_hotel\nplayers : 2 humans, 0 bots (8 max) (not hibernating) (unreserved)\n"

	got := ParseStatus(output)

	if got != (Status{Map: "c1m1_hotel", Humans: 2}) {
		t.Fatalf("ParseStatus() = %+v", got)
	}
}

func TestPoll_StartsTheSpares(t *testing.T) {
	h := newHarness(2, 4)

	h.pool.poll()

	if len(h.procs) != 2 || h.procs[27020] == nil || h.procs[27021] == nil {
		t.Fatalf("started ports %v, want 27020 and 27021", h.procs)
	}
}

func TestHost_BeforeAServerIsReady_FindsNone(t *testing.T) {
	h := newHarness(1, 4)
	h.pool.poll()

	_, ok := h.pool.Host()

	if ok {
		t.Fatal("Host() ok = true")
	}
}

func TestHost_HandsOutAReadyServerAndAnotherSpareStarts(t *testing.T) {
	h := newHarness(1, 4)
	h.pool.poll()
	h.ready(27020)
	h.after(pollInterval)

	got, ok := h.pool.Host()
	h.after(pollInterval)

	if !ok || got != address(27020) || !h.pool.Hosting(got) {
		t.Fatalf("Host() = %v, %v; Hosting = %v", got, ok, h.pool.Hosting(got))
	}
	if h.procs[27021] == nil {
		t.Fatal("no spare started on 27021")
	}
}

func TestPoll_EmptyRoom_StopsItsServerAfterTheGrace(t *testing.T) {
	h := newHarness(1, 4)
	h.pool.poll()
	h.ready(27020)
	h.after(pollInterval)
	room, _ := h.pool.Host()
	h.statuses[27020] = Status{Map: "c1m1_hotel", Humans: 2}
	h.after(pollInterval)
	h.statuses[27020] = Status{Map: "c1m1_hotel"}
	h.after(pollInterval)

	stillUp := !h.procs[27020].killed
	h.after(emptyGrace)

	if !stillUp || !h.procs[27020].killed || h.pool.Hosting(room) {
		t.Fatalf("killed before grace = %v, after = %v, Hosting = %v", !stillUp, h.procs[27020].killed, h.pool.Hosting(room))
	}
}

func TestPoll_RoomNobodyJoins_StopsItsServer(t *testing.T) {
	h := newHarness(1, 4)
	h.pool.poll()
	h.ready(27020)
	h.after(pollInterval)
	h.pool.Host()

	h.after(joinTimeout + pollInterval)

	if !h.procs[27020].killed {
		t.Fatal("server not stopped")
	}
}

func TestPoll_ServerThatNeverLoads_IsStopped(t *testing.T) {
	h := newHarness(1, 4)
	h.pool.poll()

	h.after(startTimeout + pollInterval)

	if !h.procs[27020].killed {
		t.Fatal("server not stopped")
	}
}

func TestPoll_StoppedServerKeepsItsPortUntilItExits(t *testing.T) {
	h := newHarness(1, 1)
	h.pool.poll()
	first := h.procs[27020]
	h.after(startTimeout + pollInterval)
	h.after(pollInterval)
	restartedBeforeExit := h.procs[27020] != first

	first.exit()
	h.after(pollInterval)

	if restartedBeforeExit || h.procs[27020] == first {
		t.Fatalf("restarted before exit = %v, restarted after exit = %v", restartedBeforeExit, h.procs[27020] != first)
	}
}
