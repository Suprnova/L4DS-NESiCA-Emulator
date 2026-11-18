package server_test

import (
	"context"
	"net/http"
	"testing"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/config"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/dedicated"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/server"
)

func options() server.Options {
	return server.Options{
		Config:   config.Default().Server,
		Database: ":memory:",
		Master:   "127.0.0.1:0",
	}
}

func TestStart_AnswersTheGameThroughItsBackend(t *testing.T) {
	s, err := server.Start(context.Background(), options())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })

	status, reply, err := s.Backend.Post("http://dev.game.zet.square-enix.co.jp/sendversion", []byte("version=1.2604"))

	if err != nil || status != http.StatusOK || string(reply) != "1\n1\n0\n" {
		t.Fatalf("Post() = %d, %q, %v", status, reply, err)
	}
}

func TestStart_FixedRoomServerAndPool_IsAnError(t *testing.T) {
	opts := options()
	opts.Rooms = server.Rooms{Fixed: "127.0.0.2:27020", Pool: dedicated.Config{Dir: "tree"}}

	_, err := server.Start(context.Background(), opts)

	if err == nil {
		t.Fatal("Start() error = nil")
	}
}

func TestStart_FixedRoomServerNotIPv4_IsAnError(t *testing.T) {
	opts := options()
	opts.Rooms = server.Rooms{Fixed: "[::1]:27020"}

	_, err := server.Start(context.Background(), opts)

	if err == nil {
		t.Fatal("Start() error = nil")
	}
}

func TestStart_ShimAddressUnusable_IsAnError(t *testing.T) {
	opts := options()
	opts.Config.Listen = "256.0.0.1:0"
	opts.ServeShims = true

	_, err := server.Start(context.Background(), opts)

	if err == nil {
		t.Fatal("Start() error = nil")
	}
}
