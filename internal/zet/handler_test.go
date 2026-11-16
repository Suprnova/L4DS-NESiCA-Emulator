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

func TestHandler_Login_SucceedsWithNoBonuses(t *testing.T) {
	got := post("/login", "mac_addr=020000000001&player_id=1&chara_id=3&svp=0&rev_customize=0")

	if got != "1\n1\n0\n" {
		t.Fatalf("body = %q", got)
	}
}
