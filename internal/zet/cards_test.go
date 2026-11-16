package zet_test

import (
	"encoding/base64"
	"net/url"
	"strconv"
	"strings"
	"testing"

	"l4ds-nesica-emulator/internal/store"
	"l4ds-nesica-emulator/internal/zet"
)

func newServer(t *testing.T) (*zet.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return zet.NewServer(s), s
}

func TestServer_Select_ReturnsTheStoredPlayer(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	s.RecordResult(player.ID, store.Result{Zet: 568, Score: 195})

	body, err := server.Select("7020392000000001")

	lines := strings.Split(body, "\n")
	if err != nil || lines[0] != "1" || lines[2] != "7020392000000001" || lines[3] != "1" || lines[4] != "Survivor" || lines[5] != "568" || lines[7] != "195" {
		t.Fatalf("Select() = %q, %v", body, err)
	}
}

func TestServer_Select_ReportsExperienceAndLoginsAfterTheEmptyBlocks(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	s.RecordResult(player.ID, store.Result{Exp: 222})
	s.RecordLogin(player.ID)

	body, _ := server.Select("7020392000000001")

	lines := strings.Split(body, "\n")
	if lines[53] != "222" || lines[87] != "1" {
		t.Fatalf("experience line = %q, login_num line = %q", lines[53], lines[87])
	}
}

func TestServer_Select_UnregisteredCard_ReportsTheUnregisteredResult(t *testing.T) {
	server, _ := newServer(t)

	body, err := server.Select("7020392000000009")

	if err != nil || body != "23\n1\n7020392000000009\n" {
		t.Fatalf("Select() = %q, %v", body, err)
	}
}

func TestServer_Insert_RegistersTheDecodedPlayer(t *testing.T) {
	server, s := newServer(t)
	name := url.QueryEscape(base64.StdEncoding.EncodeToString([]byte("Nova ノヴァ")))

	body, err := server.Insert("7020392000000002", "tenpo_id=1,player_name="+name+",chara_id=3")

	player, _ := s.PlayerByCard("7020392000000002")
	if err != nil || body != "1\n1\n"+strconv.FormatInt(player.ID, 10)+"\n" || player.Name != "Nova ノヴァ" || player.CharaID != 3 {
		t.Fatalf("Insert() = %q, %v; player %+v", body, err, player)
	}
}

func TestServer_Insert_RegisteredCard_ReturnsAnError(t *testing.T) {
	server, s := newServer(t)
	s.Register("7020392000000002", "Nova", 0)

	_, err := server.Insert("7020392000000002", "tenpo_id=1,player_name=,chara_id=1")

	if err == nil {
		t.Fatal("Insert() error = nil")
	}
}
