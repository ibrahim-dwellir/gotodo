// Package api exposes the todo service over HTTP under the /api/ prefix.
package api

import (
	"net/http"

	"gotodo/internal/auth"
	"gotodo/internal/docs"
	"gotodo/internal/store"
)

// API holds the dependencies shared by all handlers.
type API struct {
	store   *store.Store
	tokens  *auth.Tokens
	metrics *Metrics
}

// New returns an API backed by the given store and token issuer, recording
// metrics into m (see NewMetrics).
func New(s *store.Store, t *auth.Tokens, m *Metrics) *API {
	return &API{store: s, tokens: t, metrics: m}
}

// Handler returns the root HTTP handler: JSON API under /api/ plus Swagger UI under /swagger/.
func (a *API) Handler() http.Handler {
	mux := http.NewServeMux()

	mux.HandleFunc("GET /api/health", a.health)

	mux.HandleFunc("POST /api/auth/register", a.register)
	mux.HandleFunc("POST /api/auth/login", a.login)

	mux.HandleFunc("GET /api/users/me", a.requireAuth(a.getMe))
	mux.HandleFunc("PUT /api/users/me", a.requireAuth(a.updateMe))

	mux.HandleFunc("GET /api/todos", a.requireAuth(a.listTodos))
	mux.HandleFunc("POST /api/todos", a.requireAuth(a.createTodo))
	mux.HandleFunc("GET /api/todos/{id}", a.requireAuth(a.getTodo))
	mux.HandleFunc("PUT /api/todos/{id}", a.requireAuth(a.updateTodo))
	mux.HandleFunc("DELETE /api/todos/{id}", a.requireAuth(a.deleteTodo))

	docs.Register(mux)

	// instrument is outermost so recovered panics are still counted (as 500s).
	return a.metrics.instrument(recoverPanics(logRequests(mux)))
}

func (a *API) health(w http.ResponseWriter, _ *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok"})
}
