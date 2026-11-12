package main

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestMount_RoutesUIAndAPI(t *testing.T) {
	api := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.WriteString(w, "api:"+r.URL.Path)
	})
	srv := httptest.NewServer(mount(api))
	t.Cleanup(srv.Close)

	get := func(path string) (int, string) {
		t.Helper()
		resp, err := srv.Client().Get(srv.URL + path)
		require.NoError(t, err)
		defer resp.Body.Close()
		b, err := io.ReadAll(resp.Body)
		require.NoError(t, err)
		return resp.StatusCode, string(b)
	}

	tests := []struct {
		name     string
		path     string
		wantCode int
		contains string
	}{
		{name: "ui index", path: "/", wantCode: http.StatusOK, contains: `x-data="app"`},
		{name: "ui script", path: "/app.js", wantCode: http.StatusOK, contains: "Alpine.data('app'"},
		{name: "api", path: "/api/todos", wantCode: http.StatusOK, contains: "api:/api/todos"},
		{name: "swagger", path: "/swagger/index.html", wantCode: http.StatusOK, contains: "api:/swagger/index.html"},
		{name: "unknown static", path: "/nope.css", wantCode: http.StatusNotFound},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := get(tc.path)
			assert.Equal(t, tc.wantCode, code)
			assert.Contains(t, body, tc.contains)
		})
	}
}
