package zet_test

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"

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
	for _, path := range []string{"/loading_start", "/chapter_start"} {
		got := post(t, path, "player_id=1")

		if got != "1\n1\n0\n" {
			t.Fatalf("%s body = %q", path, got)
		}
	}
}

func TestHandler_Result_AddsTheChapterToThePlayersTotals(t *testing.T) {
	server, s := newServer(t)
	player, _ := s.PlayerByCard("7020392000000001")

	got := postTo(server, "/result", fmt.Sprintf("player_id=%d&zet=568&exp=222&TOTAL_SCORE=195", player.ID))

	updated, _ := s.Player(player.ID)
	if got != "1\n1\n0\n" || updated.Zet != 568 || updated.Exp != 222 || updated.Score != 195 {
		t.Fatalf("body = %q, player = %+v", got, updated)
	}
}
