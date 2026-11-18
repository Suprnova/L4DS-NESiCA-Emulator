package remote_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/remote"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

var _ nesys.Backend = (*remote.Client)(nil)

var testStore = remote.Store{ID: 7, Name: "Test Arcade", Prefecture: "Osaka"}

// connect runs a game server behind the remote handler and returns a client of it.
func connect(t *testing.T) *remote.Client {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	server := httptest.NewServer(remote.Handler(zet.NewServer(s, zet.Catalog{}), []remote.Store{testStore}))
	t.Cleanup(server.Close)
	return remote.NewClient(server.URL, "7/020000000001")
}

func TestClient_Post_ReachesTheGameServer(t *testing.T) {
	client := connect(t)

	status, reply, err := client.Post("http://dev.game.zet.square-enix.co.jp/sendversion", []byte("version=1.2604"))

	if err != nil || status != http.StatusOK || string(reply) != "1\n1\n0\n" {
		t.Fatalf("Post() = %d, %q, %v", status, reply, err)
	}
}

func TestClient_Post_UnknownPath_IsNotFound(t *testing.T) {
	client := connect(t)

	status, _, err := client.Post("http://dev.game.zet.square-enix.co.jp/mypage", nil)

	if err != nil || status != http.StatusNotFound {
		t.Fatalf("Post() = %d, %v", status, err)
	}
}

func TestClient_Select_UnregisteredCard_HasTheUnregisteredStatus(t *testing.T) {
	client := connect(t)

	reply, err := client.Select("7020392000000001")

	if err != nil || reply.Status != wire.CardStatusUnregistered {
		t.Fatalf("Select() = %+v, %v", reply, err)
	}
}

func TestClient_InsertThenSelect_FindsThePlayer(t *testing.T) {
	client := connect(t)
	body, err := client.Insert("7020392000000001", "tenpo_id=7,player_name=TWlrYQ%3D%3D,chara_id=2")
	if err != nil {
		t.Fatal(err)
	}

	reply, err := client.Select("7020392000000001")

	if err != nil || reply.Status != 1 || !strings.HasPrefix(body, "1\n1\n") || !strings.Contains(reply.Body, "\nMika\n") {
		t.Fatalf("Insert() = %q; Select() = %+v, %v", body, reply, err)
	}
}

func TestClient_Update_UnregisteredCard_IsAnError(t *testing.T) {
	client := connect(t)

	err := client.Update("7020392000000001", 1, "player_name=TWlrYQ%3D%3D")

	if err == nil {
		t.Fatal("Update() error = nil")
	}
}

func TestClient_Ranking_ReturnsTheBody(t *testing.T) {
	client := connect(t)

	body, err := client.Ranking(zet.RankingNationalCoop)

	if err != nil || body != "1\n1\n0\n" {
		t.Fatalf("Ranking() = %q, %v", body, err)
	}
}

func TestClient_Hello_SameProtocol_Succeeds(t *testing.T) {
	client := connect(t)

	err := client.Hello()

	if err != nil {
		t.Fatalf("Hello() = %v", err)
	}
}

func TestClient_Store_ListedStore_ReturnsIt(t *testing.T) {
	client := connect(t)

	got, err := client.Store(7)

	if err != nil || got != testStore {
		t.Fatalf("Store() = %+v, %v", got, err)
	}
}

func TestClient_Store_UnlistedStore_ReturnsErrUnknownStore(t *testing.T) {
	client := connect(t)

	_, err := client.Store(8)

	if !errors.Is(err, remote.ErrUnknownStore) {
		t.Fatalf("Store() error = %v, want ErrUnknownStore", err)
	}
}

func TestClient_ServerUnreachable_ReturnsAnError(t *testing.T) {
	server := httptest.NewServer(http.NotFoundHandler())
	server.Close()
	client := remote.NewClient(server.URL, "7/020000000001")

	_, err := client.Select("7020392000000001")

	if err == nil {
		t.Fatal("Select() error = nil")
	}
}
