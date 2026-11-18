package zet

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
)

// Post answers a tunnelled request in process, by running the server's handler on a POST of
// body to url; the host in url is ignored. With [Server.Select], [Server.ReissueTest],
// [Server.Insert], [Server.Update], and [Server.Ranking], it makes the server a nesys.Backend.
func (s *Server) Post(url string, body []byte) (int, []byte, error) {
	request, err := http.NewRequest(http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return 0, nil, err
	}
	request.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	var reply response
	s.handler.ServeHTTP(&reply, request)
	return reply.status(), reply.body.Bytes(), nil
}

// response collects a handler's reply in memory.
type response struct {
	header http.Header
	code   int
	body   bytes.Buffer
}

func (r *response) Header() http.Header {
	if r.header == nil {
		r.header = http.Header{}
	}
	return r.header
}

func (r *response) Write(b []byte) (int, error) { return r.body.Write(b) }

func (r *response) WriteHeader(code int) {
	if r.code == 0 {
		r.code = code
	}
}

// status is the reply's status code: 200 unless the handler set another.
func (r *response) status() int {
	if r.code == 0 {
		return http.StatusOK
	}
	return r.code
}

// logRequests logs each request's path and body before handing it on.
func logRequests(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, _ := io.ReadAll(r.Body)
		r.Body = io.NopCloser(bytes.NewReader(body))
		slog.Info("http", "path", r.URL.Path, "body", string(body))
		next.ServeHTTP(w, r)
	})
}
