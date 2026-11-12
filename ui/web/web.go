// Package web holds the GoTodo front end (HTML + JS, embedded at build time)
// and the HTTP handlers that serve it.
//
// Use FileServer when the page is served from the same origin as the API (the
// all-in-one gotodo binary), and NewHandler when the UI runs on its own and
// must proxy /api/ to a separate API server.
package web

import (
	"embed"
	"io/fs"
	"log/slog"
	"net/http"
	"net/http/httputil"
	"net/url"
)

//go:embed static
var static embed.FS

// Assets returns the embedded front-end files (index.html, app.js).
func Assets() fs.FS {
	sub, err := fs.Sub(static, "static")
	if err != nil {
		panic(err) // the embedded path is fixed at build time
	}
	return sub
}

// FileServer serves the embedded front end.
func FileServer() http.Handler {
	return http.FileServerFS(Assets())
}

// NewHandler serves the front end and reverse-proxies /api/ and /swagger/ to
// apiURL, so the browser talks to a single origin.
func NewHandler(apiURL *url.URL) http.Handler {
	proxy := &httputil.ReverseProxy{
		Rewrite: func(r *httputil.ProxyRequest) {
			r.SetURL(apiURL)
			r.SetXForwarded()
		},
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			slog.Error("api proxy", "path", r.URL.Path, "err", err)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusBadGateway)
			_, _ = w.Write([]byte(`{"error":"the API server is unreachable"}`))
		},
	}

	mux := http.NewServeMux()
	mux.Handle("/api/", proxy)
	mux.Handle("/swagger/", proxy)
	mux.Handle("/", FileServer())
	return mux
}
