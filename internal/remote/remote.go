// Package remote carries a game's network requests from a shim to a server over HTTP. The
// server side, [Handler], answers them with a nesys.Backend; the shim side, [Client], is a
// nesys.Backend that asks a server.
//
// Endpoints, all relative to the server's base URL:
//
//	POST /zet/<path>                  a tunnelled game request; the reply is the game server's
//	POST /nesys/card/select           {card} -> {status, body}
//	POST /nesys/card/reissue-test     {card} -> {status, body}
//	POST /nesys/card/insert           {card, data} -> {body}
//	POST /nesys/card/update           {card, type, data} -> {}
//	POST /nesys/ranking               {kind} -> {body}
//	GET  /nesys/hello                 -> {protocol}
//	GET  /nesys/store/<id>            -> {id, name, prefecture}, or 404 for a store not listed
//
// Every request names its cabinet in the X-L4DS-Cabinet header, which the server logs.
package remote

import (
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"strconv"

	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/nesys"
	"github.com/Suprnova/L4DS-NESiCA-Emulator/internal/wire"
)

// Protocol is the version of this protocol. A shim and a server speak to each other only when
// their versions are equal.
const Protocol = 1

// cabinetHeader names the cabinet a request comes from.
const cabinetHeader = "X-L4DS-Cabinet"

// ErrUnknownStore reports that the server does not list a store.
var ErrUnknownStore = errors.New("remote: the server does not list this store")

// Store is a store the server lists: the arcades whose cabinets it serves.
type Store struct {
	ID         uint32 `json:"id"`
	Name       string `json:"name"`
	Prefecture string `json:"prefecture"`
}

type cardRequest struct {
	Card string `json:"card"`
	Type uint32 `json:"type,omitempty"`
	Data string `json:"data,omitempty"`
}

type cardReply struct {
	Status uint32 `json:"status,omitempty"`
	Body   string `json:"body"`
}

type rankingRequest struct {
	Kind uint32 `json:"kind"`
}

type hello struct {
	Protocol int `json:"protocol"`
}

// Handler answers shims' requests with backend, and lists stores to them.
func Handler(backend nesys.Backend, stores []Store) http.Handler {
	byID := map[uint32]Store{}
	for _, store := range stores {
		byID[store.ID] = store
	}

	mux := http.NewServeMux()
	mux.HandleFunc("POST /zet/{path...}", func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		status, reply, err := backend.Post("http://zet/"+r.PathValue("path"), body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusBadGateway)
			return
		}
		w.WriteHeader(status)
		w.Write(reply)
	})
	mux.HandleFunc("POST /nesys/card/select", card(func(request cardRequest) (any, error) {
		reply, err := backend.Select(request.Card)
		return cardReply{reply.Status, reply.Body}, err
	}))
	mux.HandleFunc("POST /nesys/card/reissue-test", card(func(request cardRequest) (any, error) {
		reply, err := backend.ReissueTest(request.Card)
		return cardReply{reply.Status, reply.Body}, err
	}))
	mux.HandleFunc("POST /nesys/card/insert", card(func(request cardRequest) (any, error) {
		body, err := backend.Insert(request.Card, request.Data)
		return cardReply{Body: body}, err
	}))
	mux.HandleFunc("POST /nesys/card/update", card(func(request cardRequest) (any, error) {
		return struct{}{}, backend.Update(request.Card, request.Type, request.Data)
	}))
	mux.HandleFunc("POST /nesys/ranking", func(w http.ResponseWriter, r *http.Request) {
		var request rankingRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		body, err := backend.Ranking(request.Kind)
		respond(w, cardReply{Body: body}, err)
	})
	mux.HandleFunc("GET /nesys/hello", func(w http.ResponseWriter, r *http.Request) {
		respond(w, hello{Protocol}, nil)
	})
	mux.HandleFunc("GET /nesys/store/{id}", func(w http.ResponseWriter, r *http.Request) {
		id, err := strconv.ParseUint(r.PathValue("id"), 10, 32)
		store, listed := byID[uint32(id)]
		if err != nil || !listed {
			http.NotFound(w, r)
			return
		}
		respond(w, store, nil)
	})
	return logCabinet(mux)
}

// card decodes a card request and answers with what handle returns.
func card(handle func(cardRequest) (any, error)) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		var request cardRequest
		if err := json.NewDecoder(r.Body).Decode(&request); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		reply, err := handle(request)
		respond(w, reply, err)
	}
}

// respond writes reply as JSON, or err as a server error.
func respond(w http.ResponseWriter, reply any, err error) {
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

// logCabinet logs every request with the cabinet it came from.
func logCabinet(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		slog.Debug("shim request", "cabinet", r.Header.Get(cabinetHeader), "method", r.Method, "path", r.URL.Path)
		next.ServeHTTP(w, r)
	})
}

// card is the reply as the pipe carries it.
func (r cardReply) card() wire.CardReply { return wire.CardReply{Status: r.Status, Body: r.Body} }
