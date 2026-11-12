package telemetry_test

import (
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/pprof/profile"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gotodo/internal/telemetry"
)

func get(t *testing.T, srv *httptest.Server, path string) (int, []byte) {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	require.NoError(t, err)
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	return resp.StatusCode, b
}

func newServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(telemetry.Handler(telemetry.NewRegistry(nil)))
	t.Cleanup(srv.Close)
	return srv
}

// TestHandler_ServesParseableProfiles checks every path a Pyroscope/OTel
// scraper pulls returns a valid pprof profile with samples of the expected type.
func TestHandler_ServesParseableProfiles(t *testing.T) {
	telemetry.EnableContentionProfiling(5, 5)
	srv := newServer(t)

	tests := []struct {
		path       string
		sampleType string
	}{
		{path: "/debug/pprof/profile?seconds=1", sampleType: "cpu"},
		{path: "/debug/pprof/heap", sampleType: "inuse_space"},
		{path: "/debug/pprof/allocs", sampleType: "alloc_space"},
		{path: "/debug/pprof/goroutine", sampleType: "goroutine"},
		{path: "/debug/pprof/mutex", sampleType: "delay"},
		{path: "/debug/pprof/block", sampleType: "delay"},
		{path: "/debug/pprof/delta_heap", sampleType: "inuse_space"},
		{path: "/debug/pprof/delta_mutex", sampleType: "delay"},
		{path: "/debug/pprof/delta_block", sampleType: "delay"},
	}
	for _, tc := range tests {
		t.Run(tc.path, func(t *testing.T) {
			code, body := get(t, srv, tc.path)
			require.Equal(t, http.StatusOK, code, string(body))

			p, err := profile.Parse(bytes.NewReader(body))
			require.NoError(t, err)
			var types []string
			for _, st := range p.SampleType {
				types = append(types, st.Type)
			}
			assert.Contains(t, types, tc.sampleType)
		})
	}
}

func TestHandler_ServesMetricsAndIndex(t *testing.T) {
	srv := newServer(t)

	code, body := get(t, srv, "/metrics")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), "go_goroutines ")
	assert.Contains(t, string(body), "process_cpu_seconds_total ")
	assert.Contains(t, string(body), "go_build_info{")

	code, body = get(t, srv, "/debug/pprof/")
	require.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), "goroutine")
}
