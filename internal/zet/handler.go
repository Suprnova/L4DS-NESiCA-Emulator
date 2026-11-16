// Package zet implements the ZET game server: the HTTP endpoints the game reaches through
// the NESYS tunnel, and the card lookups that share its player data.
package zet

import (
	"net/http"

	"l4ds-nesica-emulator/internal/store"
)

// Server is the ZET game server.
type Server struct {
	store *store.Store
}

// NewServer returns a server that keeps its players in s.
func NewServer(s *store.Store) *Server { return &Server{store: s} }

// Handler returns the server's HTTP handler.
func (s *Server) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sendversion", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /version", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /login", login)
	mux.HandleFunc("POST /loading_start", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /chapter_start", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /result", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /svp", svp)
	return mux
}

// reply answers every request with a fixed body.
func reply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}
}

// svp answers with a success header followed by the player id the game sent, which the game
// uses as its player id from then on.
func svp(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("1\n1\n0\n" + r.PostFormValue("player_id") + "\n"))
}
