package api

import (
	"net/http"
	"strconv"
	"time"

	"github.com/prometheus/client_golang/prometheus"
)

// Metrics holds the API's Prometheus instruments.
type Metrics struct {
	requests      *prometheus.CounterVec
	duration      *prometheus.HistogramVec
	inFlight      prometheus.Gauge
	logins        *prometheus.CounterVec
	registrations *prometheus.CounterVec
}

// NewMetrics creates the API metrics and registers them with reg.
func NewMetrics(reg prometheus.Registerer) *Metrics {
	m := &Metrics{
		requests: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gotodo_http_requests_total",
			Help: "HTTP requests handled, by method, route pattern and status code.",
		}, []string{"method", "route", "code"}),
		duration: prometheus.NewHistogramVec(prometheus.HistogramOpts{
			Name:    "gotodo_http_request_duration_seconds",
			Help:    "HTTP request latency, by method and route pattern.",
			Buckets: prometheus.DefBuckets,
		}, []string{"method", "route"}),
		inFlight: prometheus.NewGauge(prometheus.GaugeOpts{
			Name: "gotodo_http_requests_in_flight",
			Help: "HTTP requests currently being served.",
		}),
		logins: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gotodo_auth_logins_total",
			Help: "Login attempts, by result (success, invalid_credentials).",
		}, []string{"result"}),
		registrations: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name: "gotodo_auth_registrations_total",
			Help: "Registration attempts, by result (success, email_taken).",
		}, []string{"result"}),
	}
	reg.MustRegister(m.requests, m.duration, m.inFlight, m.logins, m.registrations)

	// Pre-create the known label values so the series exist at 0 before the first event.
	for _, r := range []string{"success", "invalid_credentials"} {
		m.logins.WithLabelValues(r)
	}
	for _, r := range []string{"success", "email_taken"} {
		m.registrations.WithLabelValues(r)
	}
	return m
}

// instrument records request count, latency and in-flight gauge. It must wrap
// the ServeMux directly: the mux sets r.Pattern on the request it is given,
// and the route label uses that pattern ("GET /api/todos/{id}") rather than
// the raw path, so ids do not create unbounded label values.
func (m *Metrics) instrument(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		m.inFlight.Inc()
		defer m.inFlight.Dec()

		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)

		route := r.Pattern
		if route == "" {
			route = "unmatched"
		}
		method := methodLabel(r.Method)
		m.requests.WithLabelValues(method, route, strconv.Itoa(rec.status)).Inc()
		m.duration.WithLabelValues(method, route).Observe(time.Since(start).Seconds())
	})
}

// methodLabel bounds the method label: clients can send any method string.
func methodLabel(method string) string {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodOptions:
		return method
	}
	return "other"
}
