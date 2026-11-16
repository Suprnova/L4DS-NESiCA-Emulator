package zet_test

import (
	"net/http/httptest"
	"strings"
	"testing"

	"l4ds-nesica-emulator/internal/zet"
)

func post(path, body string) string {
	request := httptest.NewRequest("POST", "http://lobby"+path, strings.NewReader(body))
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	recorder := httptest.NewRecorder()
	zet.Handler().ServeHTTP(recorder, request)
	return recorder.Body.String()
}

func TestHandler_SendVersion_AcceptsTheVersion(t *testing.T) {
	got := post("/sendversion", "tenpo_id=1&version=1.2604")

	if got != "1\n1\n0\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Version_RepliesWithHeaderOnly(t *testing.T) {
	got := post("/version", "")

	if got != "1\n1\n0\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Svp_ReturnsThePlayerId(t *testing.T) {
	got := post("/svp", "credit=1&player_id=42")

	if got != "1\n1\n0\n42\n" {
		t.Fatalf("body = %q", got)
	}
}

func TestHandler_Login_ReturnsTheDefaultSettingsAfterTheEmptyGroups(t *testing.T) {
	got := strings.Split(post("/login", "mac_addr=020000000001&player_id=1"), "\n")

	if got[0] != "1" || got[3] != "0" || got[7] != "0" || got[8] != "5" || got[12] != "3" {
		t.Fatalf("lines = %q", got)
	}
}

func TestHandler_ChapterLifecycle_AcceptsEachStage(t *testing.T) {
	for _, path := range []string{"/loading_start", "/chapter_start", "/result"} {
		got := post(path, "player_id=1")

		if got != "1\n1\n0\n" {
			t.Fatalf("%s body = %q", path, got)
		}
	}
}
