// Command gotodo is the all-in-one GoTodo server: web UI, JSON API and
// Swagger UI on one port, plus metrics and pprof on a separate ops port.
// Everything (UI files, migrations, Swagger assets) is embedded, so the
// binary runs on its own.
//
//	/            web UI
//	/api/        JSON API
//	/swagger/    Swagger UI and OpenAPI spec
//
// Configuration (environment):
//
//	ADDR        listen address for UI + API (default ":8080")
//	DB_PATH     SQLite database file      (default "gotodo.db")
//	JWT_SECRET  HMAC key for access tokens (default: random per process; tokens die on restart)
//	TOKEN_TTL   token lifetime, Go duration (default "24h")
//
// Observability (served on a separate port, keep it off the public internet):
//
//	OPS_ADDR                ops listen address for /metrics and /debug/pprof/* (default ":6060"; "off" disables)
//	MUTEX_PROFILE_FRACTION  sample 1/n mutex contention events (default 5; 0 disables)
//	BLOCK_PROFILE_RATE      sample one blocking event per n ns blocked (default 5; 0 disables)
package main

import (
	"context"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"gotodo/internal/app"
	"gotodo/ui/web"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := app.ConfigFromEnv()
	if err == nil {
		err = app.Run(ctx, cfg, mount)
	}
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

// mount serves the API (which owns /api/ and /swagger/) and the embedded web
// UI from one origin, so the browser calls /api/ directly with no proxy.
func mount(api http.Handler) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/api/", api)
	mux.Handle("/swagger/", api)
	mux.Handle("/", web.FileServer())
	return mux
}
