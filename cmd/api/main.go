// Command api runs the GoTodo JSON API with Swagger UI, without the web front
// end. For API + UI in one binary, use the gotodo command at the repo root.
//
// Configuration (environment):
//
//	ADDR        listen address            (default ":8080")
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
	"os"
	"os/signal"
	"syscall"

	"gotodo/internal/app"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := app.ConfigFromEnv()
	if err == nil {
		err = app.Run(ctx, cfg, nil)
	}
	if err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}
