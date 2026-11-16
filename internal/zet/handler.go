// Package zet implements the ZET game server: the HTTP endpoints the game reaches through
// the NESYS tunnel.
package zet

import "net/http"

// Handler returns the ZET game server's HTTP handler.
func Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /sendversion", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /version", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /login", reply("1\n1\n0\n"))
	mux.HandleFunc("POST /svp", svp)
	return mux
}

// reply answers every request with a fixed body.
func reply(body string) http.HandlerFunc {
	return func(w http.ResponseWriter, _ *http.Request) {
		w.Write([]byte(body))
	}
}

// svp answers with a success header followed by the player id the game sent.
func svp(w http.ResponseWriter, r *http.Request) {
	w.Write([]byte("1\n1\n" + r.PostFormValue("player_id") + "\n"))
}
