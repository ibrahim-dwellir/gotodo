// Command ui serves the GoTodo web front end on its own and proxies /api/
// (and Swagger) to a separately running API server, so the browser talks to a
// single origin. The all-in-one gotodo binary (repo root) serves the same
// front end without a proxy.
//
// Configuration (environment):
//
//	UI_ADDR  listen address        (default ":3000")
//	API_URL  base URL of the API   (default "http://localhost:8080")
package main

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/url"
	"os"
	"os/signal"
	"syscall"
	"time"

	"gotodo/ui/web"
)

func env(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func main() {
	if err := run(); err != nil {
		slog.Error("fatal", "err", err)
		os.Exit(1)
	}
}

func run() error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	addr := env("UI_ADDR", ":3000")
	apiURL, err := url.Parse(env("API_URL", "http://localhost:8080"))
	if err != nil {
		return err
	}
	if apiURL.Scheme == "" || apiURL.Host == "" {
		return errors.New("API_URL must be an absolute URL such as http://localhost:8080")
	}

	srv := &http.Server{
		Addr:              addr,
		Handler:           web.NewHandler(apiURL),
		ReadHeaderTimeout: 5 * time.Second,
	}

	errCh := make(chan error, 1)
	go func() {
		slog.Info("ui listening", "addr", addr, "api", apiURL.String())
		errCh <- srv.ListenAndServe()
	}()

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			return err
		}
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	return srv.Shutdown(shutdownCtx)
}
