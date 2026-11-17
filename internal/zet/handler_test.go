package zet_test

import (
	"fmt"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"l4ds-nesica-emulator/internal/game"
	"l4ds-nesica-emulator/internal/store"
	"l4ds-nesica-emulator/internal/zet"
)

func post(t *testing.T, path, body string) string {
	t.Helper()
	server, _ := newServer(t)
	return postTo(server, path, body)
}

func postTo(server *zet.Server, path, body string) string {
	request := httptest.NewRequest("POST", "http://lobby"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	server.Handler().ServeHTTP(recorder, request)
	return recorder.Body.String()
}

func TestHandler_SendVersion_AcceptsTheVersion(t *testing.T) {
	got := post(t, "/sendversion", "tenpo_id=1&version=1.2604")

	if got != "1\n1\n0\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Version_RepliesWithHeaderOnly(t *testing.T) {
	got := post(t, "/version", "")

	if got != "1\n1\n0\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Svp_ReturnsThePlayerId(t *testing.T) {
	got := post(t, "/svp", "credit=1&player_id=42")

	if got != "1\n1\n0\n42\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Login_ReturnsTheDefaultSettingsAfterTheEmptyGroups(t *testing.T) {
	got := strings.Split(post(t, "/login", "mac_addr=020000000001&player_id=1"), "\n")

	if got[0] != "1" || got[3] != "0" || got[7] != "0" || got[8] != "5" || got[12] != "3" {
		t.Fatalf("lines = %q", got)
	}
}

func TestHandler_ChapterLifecycle_AcceptsEachStage(t *testing.T) {
	for _, path := range []string{"/loading_start", "/prestage", "/chapter_start", "/chapter_end", "/close"} {
		got := post(t, path, "player_id=1")

		if got != "1\n1\n0\n" {
			t.Fatalf("%s body = %q", path, got)
		}
	}
}

func TestHandler_Result_AddsTheChapterToThePlayersTotals(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)

	got := postTo(server, "/result", fmt.Sprintf("player_id=%d&zet=568&exp=222&TOTAL_SCORE=195", player.ID))

	updated, _ := s.Player(player.ID)
	if got != "1\n1\n0\n" || updated.Zet != 568 || updated.Exp != 222 || updated.Score != 195 {
		t.Fatalf("body = %q, player = %+v", got, updated)
	}
}

func TestHandler_Result_KeepsTheClassesTheGameReports(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	postTo(server, "/result", fmt.Sprintf("player_id=%d&class_point=40&coop_class_id=2&vs_class_id=1", player.ID))

	postTo(server, "/result", fmt.Sprintf("player_id=%d&class_point=-15&coop_class_id=2&vs_class_id=1", player.ID))

	updated, _ := s.Player(player.ID)
	if updated.CoopClassPoint != 25 || updated.CoopClassID != 2 || updated.VSClassID != 1 {
		t.Fatalf("player = %+v", updated)
	}
}

func TestHandler_Leave_TellsWhoLeftWhichRoom(t *testing.T) {
	s, _ := store.Open(":memory:")
	t.Cleanup(func() { s.Close() })
	var left [2]int64
	server := zet.NewServer(s, zet.Catalog{Leave: func(player int64, room uint32) { left = [2]int64{player, int64(room)} }})

	got := postTo(server, "/leave", "player_id=4&room_id=1")

	if got != "1\n1\n0\n" || left != [2]int64{4, 1} {
		t.Fatalf("body = %q, left = %v", got, left)
	}
}

func TestHandler_Select_IsAcknowledged(t *testing.T) {
	got := post(t, "/select", "player_id=4&room_id=1")

	if got != "1\n1\n0\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_VersusResult_AddsToTheVersusClassPoints(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)

	postTo(server, "/result", fmt.Sprintf("player_id=%d&game_mode=7&class_point=30&coop_class_id=1&vs_class_id=2", player.ID))

	updated, _ := s.Player(player.ID)
	if updated.VSClassPoint != 30 || updated.CoopClassPoint != 0 || updated.VSClassID != 2 {
		t.Fatalf("player = %+v", updated)
	}
}

func TestHandler_VersusResults_CountPlaysAndWins(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	postTo(server, "/result", fmt.Sprintf("player_id=%d&game_mode=7&vs_you_win=1", player.ID))
	postTo(server, "/result", fmt.Sprintf("player_id=%d&game_mode=7&vs_you_win=0", player.ID))

	postTo(server, "/result", fmt.Sprintf("player_id=%d&game_mode=1&vs_you_win=0", player.ID))

	updated, _ := s.Player(player.ID)
	if updated.VSPlays != 2 || updated.VSWins != 1 {
		t.Fatalf("player = %+v", updated)
	}
}

func TestHandler_Stages_ListsEveryStageInEachKind(t *testing.T) {
	s, _ := store.Open(":memory:")
	t.Cleanup(func() { s.Close() })
	stages := []game.Stage{{ID: 9, Name: "Crash Course", Chapters: []game.Chapter{{ID: 1, Name: "1: Alleys"}, {ID: 2, Name: "2: Truck Depot"}}}}
	server := zet.NewServer(s, zet.Catalog{Stages: stages})

	got := strings.Split(postTo(server, "/stages", ""), "\n")

	stage := func(kind string) []string {
		return []string{"9", kind, "1", "Crash Course", "2", "1", "1: Alleys", "2", "2: Truck Depot"}
	}
	want := slices.Concat([]string{"1", "1", "0", "4"}, stage("1"), stage("2"), stage("3"), stage("4"), []string{""})
	if !slices.Equal(got, want) {
		t.Fatalf("lines = %q, want %q", got, want)
	}
}

func TestHandler_Login_SendsTheSettingsTheCardStored(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	server.Update("7020392000000001", 3, "volume=2,volume_bgm=7,mouse_acce=1,mouse_horizon=1,mouse_vertical=0,mouse_sens=9,key_type=2")

	lines := strings.Split(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)), "\n")

	got := lines[3+5 : 3+5+25]
	want := slices.Concat([]string{"2", "7", "1", "0", "9", "1", "2"}, slices.Repeat([]string{"0"}, 18))
	if !slices.Equal(got, want) {
		t.Fatalf("config lines = %q, want %q", got, want)
	}
}

func TestHandler_Login_Guest_GetsTheDefaultsAndNoProgress(t *testing.T) {
	got := strings.Split(post(t, "/login", "player_id=-1"), "\n")

	if got[8] != "5" || got[14] != "1" || got[3+5+25+4] != "0" {
		t.Fatalf("lines = %q", got)
	}
}

func TestHandler_Result_Guest_IsAcknowledged(t *testing.T) {
	got := post(t, "/result", "player_id=-1&zet=10&stage_id=1&chapter_id=1&clear_flag=1")

	if got != "1\n1\n0\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_UnknownPath_IsNotFound(t *testing.T) {
	server, _ := newServer(t)
	request := httptest.NewRequest("POST", "http://lobby/news", strings.NewReader("t"))
	recorder := httptest.NewRecorder()

	server.Handler().ServeHTTP(recorder, request)

	if recorder.Code != 404 {
		t.Fatalf("status = %d", recorder.Code)
	}
}

func TestHandler_Login_CountsTheLogin(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)

	postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID))

	updated, _ := s.Player(player.ID)
	if updated.Logins != 1 {
		t.Fatalf("logins = %d", updated.Logins)
	}
}

func TestHandler_EventOrVersusResult_LeavesTheStageProgressAlone(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	postTo(server, "/result", fmt.Sprintf("player_id=%d&game_mode=2&stage_id=1&chapter_id=1&clear_flag=1&campaign_flag=0", player.ID))

	lines := strings.Split(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)), "\n")

	if got := lines[3+5+25+4]; got != "0" {
		t.Fatalf("stage count line = %q, want 0", got)
	}
}

func TestHandler_Login_ReportsClearedStagesAfterTheConfig(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.Register("7020392000000001", "Survivor", 0)
	postTo(server, "/result", fmt.Sprintf("player_id=%d&stage_id=1&chapter_id=1&clear_flag=1&campaign_flag=1", player.ID))
	postTo(server, "/result", fmt.Sprintf("player_id=%d&stage_id=1&chapter_id=2&clear_flag=1&campaign_flag=1", player.ID))

	lines := strings.Split(postTo(server, "/login", fmt.Sprintf("player_id=%d", player.ID)), "\n")

	got := lines[3+5+25+4:]
	want := []string{"1", "1", "1", "2", "0", "2", "1", "1", "0", "2", "1", "0", ""}
	if !slices.Equal(got, want) {
		t.Fatalf("stage lines = %q, want %q", got, want)
	}
}
