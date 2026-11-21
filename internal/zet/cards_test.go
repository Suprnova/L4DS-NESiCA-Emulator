package zet_test

import (
	"encoding/base64"
	"fmt"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/game"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/store"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

func newServer(t *testing.T) (*zet.Server, *store.Store) {
	t.Helper()
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return zet.NewServer(s, zet.Catalog{}), s
}

func TestServer_Select_ReturnsTheStoredPlayer(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	s.RecordResult(player.ID, store.Result{Zet: 568, Score: 195})

	reply, err := server.Select("7020392000000001")

	body := reply.Body
	lines := strings.Split(body, "\n")
	if err != nil || lines[0] != "1" || lines[2] != "7020392000000001" || lines[3] != "1" || lines[4] != "Survivor" || lines[5] != "568" || lines[7] != "195" {
		t.Fatalf("Select() = %q, %v", body, err)
	}
}

func TestServer_Select_ReportsExperienceAndLoginsAfterTheSlots(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	s.RecordResult(player.ID, store.Result{Exp: 222})
	s.RecordLogin(player.ID, time.Now())

	reply, _ := server.Select("7020392000000001")

	lines := strings.Split(reply.Body, "\n")
	if lines[69] != "222" || lines[71] != "1" || len(lines) != 75 {
		t.Fatalf("experience line = %q, login_num line = %q, %d lines", lines[69], lines[71], len(lines))
	}
}

func TestServer_Select_ReportsTheVersusRecordAfterTheLogins(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	s.RecordResult(player.ID, store.Result{VSPlays: 3, VSWins: 2})

	reply, _ := server.Select("7020392000000001")

	lines := strings.Split(reply.Body, "\n")
	if lines[72] != "3" || lines[73] != "2" {
		t.Fatalf("vs_plays line = %q, vs_wins line = %q", lines[72], lines[73])
	}
}

func TestServer_Select_ReportsTheClasses(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	s.RecordResult(player.ID, store.Result{CoopClassPoint: 120, CoopClassID: 3})

	reply, _ := server.Select("7020392000000001")

	got := strings.Split(reply.Body, "\n")[10:14]
	if want := []string{"120", "0", "3", "1"}; !slices.Equal(got, want) {
		t.Fatalf("class lines = %q, want %q", got, want)
	}
}

func TestServer_Select_ReportsTheHomeStore(t *testing.T) {
	s, err := store.Open(":memory:")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	server := zet.NewServer(s, zet.Catalog{StoreName: func(int64) string { return "Second Store" }})
	s.Register("7020392000000001", "Survivor", 0, 2)

	reply, _ := server.Select("7020392000000001")

	got := strings.Split(reply.Body, "\n")[16:18]
	if want := []string{"2", "Second Store"}; !slices.Equal(got, want) {
		t.Fatalf("home store lines = %q, want %q", got, want)
	}
}

func TestServer_Select_ListsEverySlotWithTheItemsInTheirSlots(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	give(t, s, player.ID, store.Owned{Kind: store.SkillItem, ItemID: 30, Name: "Counter", Count: 3}, store.Owned{Kind: store.Weapon, ItemID: 13, Name: "MG", Count: 5})
	s.Equip(player.ID, store.SkillItem, 2, 30, 3)
	s.Equip(player.ID, store.Weapon, 2, 13, 5)

	reply, _ := server.Select("7020392000000001")

	lines := strings.Split(reply.Body, "\n")
	skills := []string{"4", "1", "0", "", "0", "2", "30", "Counter", "3", "3", "0", "", "0", "4", "0", "", "0"}
	weapons := []string{"4", "1", "0", "", "0", "0", "0", "2", "13", "MG", "5", "13", "0",
		"3", "0", "", "0", "0", "0", "4", "0", "", "0", "0", "0"}
	if !slices.Equal(lines[25:42], skills) || !slices.Equal(lines[44:69], weapons) {
		t.Fatalf("skill lines = %q\nweapon lines = %q", lines[25:42], lines[44:69])
	}
}

func TestServer_Select_ListsTheAvatarItemsWithTheHairUnderTheHat(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 2, 1)
	hat, _ := game.Avatar("santahat_head_acc")
	beard, _ := game.Avatar("santamoustache_face_eqp")
	hatID, beardID := int64(hat.Resources[game.Sara]*100+1), int64(beard.Resources[game.Sara]*100)
	give(t, s, player.ID, store.Owned{Kind: store.Head, ItemID: hatID, Count: 1}, store.Owned{Kind: store.Accessory, ItemID: beardID, Count: 1})
	s.Equip(player.ID, store.Head, 0, hatID, 1)
	s.Equip(player.ID, store.Accessory, 1, beardID, 1)

	reply, _ := server.Select("7020392000000001")

	hair := fmt.Sprint(hat.Hair[game.Sara])
	got := strings.Split(reply.Body, "\n")[19:33]
	want := []string{
		fmt.Sprint(hatID), fmt.Sprint(hat.Resources[game.Sara]), "1", hair, hair, "0",
		"0", "0",
		fmt.Sprint(beardID), fmt.Sprint(beard.Resources[game.Sara]), "0", "0",
		"0", "0",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("avatar lines = %q, want %q", got, want)
	}
}

func TestServer_Select_ListsTheTitleWorn(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	give(t, s, player.ID, store.Owned{Kind: store.Title, ItemID: 12, Name: "Rookie", Count: 1})
	s.Equip(player.ID, store.Title, 0, 12, 1)

	reply, _ := server.Select("7020392000000001")

	if got := strings.Split(reply.Body, "\n")[42:44]; !slices.Equal(got, []string{"12", "Rookie"}) {
		t.Fatalf("title lines = %q", got)
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
	name := url.QueryEscape(base64.StdEncoding.EncodeToString([]byte("Mika ミカ")))

	body, err := server.Insert("7020392000000002", "tenpo_id=2,player_name="+name+",chara_id=3")

	player, _ := s.PlayerByCard("7020392000000002")
	if err != nil || body != "1\n1\n"+strconv.FormatInt(player.ID, 10)+"\n" || player.Name != "Mika ミカ" || player.CharaID != 3 || player.HomeStore != 2 {
		t.Fatalf("Insert() = %q, %v; player %+v", body, err, player)
	}
}

func TestServer_Update_Name_RenamesThePlayer(t *testing.T) {
	server, s := newServer(t)
	s.Register("7020392000000002", "Mika", 0, 1)
	name := url.QueryEscape(base64.StdEncoding.EncodeToString([]byte("ノヴァ")))

	err := server.Update("7020392000000002", 1, "request_flag=0,player_name="+name)

	player, _ := s.PlayerByCard("7020392000000002")
	if err != nil || player.Name != "ノヴァ" {
		t.Fatalf("Update() = %v; name %q", err, player.Name)
	}
}

func TestServer_Update_UnknownType_ChangesNothing(t *testing.T) {
	server, s := newServer(t)
	s.Register("7020392000000002", "Mika", 0, 1)

	err := server.Update("7020392000000002", 2, "player_name=x")

	player, _ := s.PlayerByCard("7020392000000002")
	if err != nil || player.Name != "Mika" || player.Settings != store.DefaultSettings {
		t.Fatalf("Update() = %v; player %+v", err, player)
	}
}

func TestServer_Insert_RegisteredCard_ReturnsAnError(t *testing.T) {
	server, s := newServer(t)
	s.Register("7020392000000002", "Mika", 0, 1)

	_, err := server.Insert("7020392000000002", "tenpo_id=1,player_name=,chara_id=1")

	if err == nil {
		t.Fatal("Insert() error = nil")
	}
}

func TestServer_Select_ReportsTheLevelFromExperience(t *testing.T) {
	s, _ := store.Open(":memory:")
	t.Cleanup(func() { s.Close() })
	server := zet.NewServer(s, zet.Catalog{Levels: game.Levels})
	player, _ := s.Register("7020392000000001", "Survivor", 0, 1)
	s.RecordResult(player.ID, store.Result{Exp: 150})

	reply, _ := server.Select("7020392000000001")

	if level := strings.Split(reply.Body, "\n")[6]; level != "2" {
		t.Fatalf("level line = %q, want 2", level)
	}
}
