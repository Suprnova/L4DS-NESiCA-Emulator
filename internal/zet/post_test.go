package zet_test

import (
	"net/http"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/zet"
)

var _ nesys.Backend = (*zet.Server)(nil)

func TestServer_Post_AnswersWithTheHandler(t *testing.T) {
	server, _ := newServer(t)

	status, reply, err := server.Post("http://dev.game.zet.square-enix.co.jp/sendversion", []byte("version=1.2604"))

	if err != nil || status != http.StatusOK || string(reply) != "1\n1\n0\n" {
		t.Fatalf("Post() = %d, %q, %v", status, reply, err)
	}
}

func TestServer_Post_UnknownPath_IsNotFound(t *testing.T) {
	server, _ := newServer(t)

	status, _, err := server.Post("http://dev.game.zet.square-enix.co.jp/mypage", nil)

	if err != nil || status != http.StatusNotFound {
		t.Fatalf("Post() = %d, %v", status, err)
	}
}
