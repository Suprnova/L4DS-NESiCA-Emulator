package zet_test

import (
	"encoding/base64"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"

	"l4ds-nesica-emulator/internal/game"
	"l4ds-nesica-emulator/internal/store"
	"l4ds-nesica-emulator/internal/wire"
	"l4ds-nesica-emulator/internal/zet"
)

func newServer(t *testing.T) (*zet.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return zet.NewServer(s, nil), s
}

func TestServer_Select_ReturnsTheStoredPlayer(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	s.RecordResult(player.ID, store.Result{Zet: 568, Score: 195})

	reply, err := server.Select("7020392000000001")

	body := reply.Body
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

	reply, _ := server.Select("7020392000000001")

	lines := strings.Split(reply.Body, "\n")
	if lines[29] != "222" || lines[31] != "1" || len(lines) != 35 {
		t.Fatalf("experience line = %q, login_num line = %q, %d lines", lines[29], lines[31], len(lines))
	}
}

func TestServer_Select_ReportsTheClasses(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	s.RecordResult(player.ID, store.Result{CoopClassPoint: 120, CoopClassID: 3})

	reply, _ := server.Select("7020392000000001")

	got := strings.Split(reply.Body, "\n")[10:14]
	if want := []string{"120", "0", "3", "1"}; !slices.Equal(got, want) {
		t.Fatalf("class lines = %q, want %q", got, want)
	}
}

func TestServer_Select_UnregisteredCard_ReportsTheUnregisteredStatus(t *testing.T) {
	server, _ := newServer(t)

	reply, err := server.Select("7020392000000009")

	if err != nil || reply.Status != wire.CardStatusUnregistered || reply.Body != "0\n1\n7020392000000009\n" {
		t.Fatalf("Select() = %+v, %v", reply, err)
	}
}

func TestServer_ReissueTest_ReportsNoReservation(t *testing.T) {
	server, _ := newServer(t)

	reply, err := server.ReissueTest("7020392000000009")

	if err != nil || reply.Status != wire.CardStatusNoReissue || reply.Body != "0\n1\n7020392000000009\n" {
		t.Fatalf("ReissueTest() = %+v, %v", reply, err)
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

func TestServer_Select_ReportsTheLevelFromExperience(t *testing.T) {
	levels, _ := game.ParseLevelTable(strings.NewReader(`"LevelTable" { "Lv1" { "BaseExp" "0" } "Lv2" { "BaseExp" "150" } }`))
	s, _ := store.Open(":memory:")
	t.Cleanup(func() { s.Close() })
	server := zet.NewServer(s, levels)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	s.RecordResult(player.ID, store.Result{Exp: 150})

	reply, _ := server.Select("7020392000000001")

	if level := strings.Split(reply.Body, "\n")[6]; level != "2" {
		t.Fatalf("level line = %q, want 2", level)
	}
}
