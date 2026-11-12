// Package app wires configuration, the database, the API and the ops
// (metrics + pprof) server into a running process. Both the API-only command
// (cmd/api) and the all-in-one gotodo binary (repo root) use it.
package app

import (
	"context"
	"crypto/rand"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"strconv"
	"time"

	"gotodo/internal/api"
	"gotodo/internal/auth"
	"gotodo/internal/db"
	"gotodo/internal/store"
	"gotodo/internal/telemetry"
)

// Config holds the process settings.
type Config struct {
	Addr                 string        // main listen address
	OpsAddr              string        // /metrics + /debug/pprof listen address; "off" disables
	DBPath               string        // SQLite database file
	JWTSecret            []byte        // HMAC key for access tokens
	TokenTTL             time.Duration // access token lifetime
	MutexProfileFraction int           // sample 1/n mutex contention events; <= 0 disables
	BlockProfileRate     int           // sample one blocking event per n ns blocked; <= 0 disables
}

// ConfigFromEnv reads ADDR, OPS_ADDR, DB_PATH, JWT_SECRET, TOKEN_TTL,
// MUTEX_PROFILE_FRACTION and BLOCK_PROFILE_RATE. When JWT_SECRET is unset a
// random secret is generated, so tokens do not survive a restart.
func ConfigFromEnv() (Config, error) {
	cfg := Config{
		Addr:      envOr("ADDR", ":8080"),
		OpsAddr:   envOr("OPS_ADDR", ":6060"),
		DBPath:    envOr("DB_PATH", "gotodo.db"),
		JWTSecret: []byte(os.Getenv("JWT_SECRET")),
	}

	var err error
	if cfg.TokenTTL, err = time.ParseDuration(envOr("TOKEN_TTL", "24h")); err != nil {
		return cfg, fmt.Errorf("parse TOKEN_TTL: %w", err)
	}
	if cfg.MutexProfileFraction, err = envInt("MUTEX_PROFILE_FRACTION", 5); err != nil {
		return cfg, err
	}
	if cfg.BlockProfileRate, err = envInt("BLOCK_PROFILE_RATE", 5); err != nil {
		return cfg, err
	}

	if len(cfg.JWTSecret) == 0 {
		cfg.JWTSecret = make([]byte, 32)
		if _, err := rand.Read(cfg.JWTSecret); err != nil {
			return cfg, fmt.Errorf("generate jwt secret: %w", err)
		}
		slog.Warn("JWT_SECRET not set; using a random secret, tokens will not survive a restart")
	}
	return cfg, nil
}

func envOr(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envInt(key string, def int) (int, error) {
	v := os.Getenv(key)
	if v == "" {
		return def, nil
	}
	n, err := strconv.Atoi(v)
	if err != nil {
		return 0, fmt.Errorf("parse %s: %w", key, err)
	}
	return n, nil
}

// Run opens and migrates the database, then serves until ctx is cancelled.
//
// mount receives the API handler, which owns /api/ and /swagger/, and returns
// the main server's root handler. Pass nil to serve the API alone.
func Run(ctx context.Context, cfg Config, mount func(api http.Handler) http.Handler) error {
	telemetry.EnableContentionProfiling(cfg.MutexProfileFraction, cfg.BlockProfileRate)

	conn, err := db.Open(cfg.DBPath)
	if err != nil {
		return err
	}
	defer conn.Close()
	if err := db.Migrate(ctx, conn); err != nil {
		return err
	}

	tokens, err := auth.NewTokens(cfg.JWTSecret, cfg.TokenTTL)
	if err != nil {
		return err
	}

	reg := telemetry.NewRegistry(conn)
	handler := api.New(store.New(conn), tokens, api.NewMetrics(reg)).Handler()
	if mount != nil {
		handler = mount(handler)
	}

	servers := []*http.Server{{
		Addr:              cfg.Addr,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}}
	slog.Info("listening", "addr", cfg.Addr, "db", cfg.DBPath, "swagger", "/swagger/index.html")

	if cfg.OpsAddr != "off" {
		servers = append(servers, &http.Server{
			Addr:              cfg.OpsAddr,
			Handler:           telemetry.Handler(reg),
			ReadHeaderTimeout: 5 * time.Second,
			// No WriteTimeout: /debug/pprof/profile and /trace stream for ?seconds=N,
			// and pprof rejects durations longer than the server's write timeout.
		})
		slog.Info("ops listening", "addr", cfg.OpsAddr, "metrics", "/metrics", "pprof", "/debug/pprof/")
	}

	errCh := make(chan error, len(servers))
	for _, srv := range servers {
		go func() { errCh <- srv.ListenAndServe() }()
	}

	select {
	case err := <-errCh:
		if !errors.Is(err, http.ErrServerClosed) {
			_ = shutdown(servers)
			return err
		}
	case <-ctx.Done():
		slog.Info("shutting down")
	}
	return shutdown(servers)
}

// shutdown gracefully stops every server, returning the first error.
func shutdown(servers []*http.Server) error {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	var first error
	for _, srv := range servers {
		if err := srv.Shutdown(ctx); err != nil && first == nil {
			first = err
		}
	}
	return first
}
