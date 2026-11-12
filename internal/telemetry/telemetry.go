// Package telemetry exposes Prometheus metrics and pprof profiles on a
// dedicated "ops" HTTP handler, kept off the public API port.
//
// Endpoints:
//
//	GET /metrics                     Prometheus exposition format
//	GET /debug/pprof/                index of runtime profiles
//	GET /debug/pprof/profile         CPU profile (?seconds=N, default 30)
//	GET /debug/pprof/{heap,allocs,goroutine,mutex,block,threadcreate}
//	GET /debug/pprof/{cmdline,symbol,trace}
//	GET /debug/pprof/delta_{heap,mutex,block}   godeltaprof delta profiles
//
// The pprof paths are what Grafana Alloy's pyroscope.scrape and the
// OpenTelemetry Collector's pprof receiver pull from. The delta_* variants
// report only what changed since the previous scrape, which is what
// Pyroscope's Go pull mode prefers for memory, mutex and block profiles.
package telemetry

import (
	"database/sql"
	"net/http"
	"net/http/pprof"
	"runtime"

	deltapprof "github.com/grafana/pyroscope-go/godeltaprof/http/pprof"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

// EnableContentionProfiling turns on sampling for the mutex and block
// profiles, which are empty by default. mutexFraction samples 1/n mutex
// contention events; blockRate samples one blocking event per n nanoseconds
// spent blocked. Values <= 0 leave that profile off.
func EnableContentionProfiling(mutexFraction, blockRate int) {
	if mutexFraction > 0 {
		runtime.SetMutexProfileFraction(mutexFraction)
	}
	if blockRate > 0 {
		runtime.SetBlockProfileRate(blockRate)
	}
}

// NewRegistry returns a registry preloaded with Go runtime, process, build
// info and (when db is non-nil) database/sql connection pool collectors.
func NewRegistry(db *sql.DB) *prometheus.Registry {
	reg := prometheus.NewRegistry()
	reg.MustRegister(
		collectors.NewGoCollector(),
		collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}),
		collectors.NewBuildInfoCollector(),
	)
	if db != nil {
		reg.MustRegister(collectors.NewDBStatsCollector(db, "sqlite"))
	}
	return reg
}

// Handler serves /metrics from reg and the pprof endpoints.
func Handler(reg *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()

	mux.Handle("GET /metrics", promhttp.HandlerFor(reg, promhttp.HandlerOpts{
		Registry:          reg, // also exports promhttp_* metrics about the scrape handler itself
		EnableOpenMetrics: true,
	}))

	// pprof.Index serves the index page and every named runtime profile
	// (heap, allocs, goroutine, mutex, block, threadcreate) under /debug/pprof/.
	mux.HandleFunc("GET /debug/pprof/", pprof.Index)
	mux.HandleFunc("GET /debug/pprof/cmdline", pprof.Cmdline)
	mux.HandleFunc("GET /debug/pprof/profile", pprof.Profile)
	mux.HandleFunc("GET /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("POST /debug/pprof/symbol", pprof.Symbol)
	mux.HandleFunc("GET /debug/pprof/trace", pprof.Trace)

	mux.HandleFunc("GET /debug/pprof/delta_heap", deltapprof.Heap)
	mux.HandleFunc("GET /debug/pprof/delta_mutex", deltapprof.Mutex)
	mux.HandleFunc("GET /debug/pprof/delta_block", deltapprof.Block)

	return mux
}
