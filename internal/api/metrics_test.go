package api_test

import (
	"io"
	"net/http"
	"strconv"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// scrape returns the /metrics body from the ops server.
func (e *testEnv) scrape() string {
	e.t.Helper()
	resp, err := e.client.Get(e.opsURL + "/metrics")
	require.NoError(e.t, err)
	defer resp.Body.Close()
	require.Equal(e.t, http.StatusOK, resp.StatusCode)
	b, err := io.ReadAll(resp.Body)
	require.NoError(e.t, err)
	return string(b)
}

func TestMetrics_RecordRoutesAndAuthEvents(t *testing.T) {
	env := newTestEnv(t)
	token, _ := env.registerAndLogin("metrics@example.com")

	// One duplicate registration and one bad login.
	code, _ := env.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"first_name": "A", "last_name": "B", "email": "metrics@example.com", "password": "password123",
	})
	require.Equal(t, http.StatusConflict, code)
	code, _ = env.do(http.MethodPost, "/api/auth/login", "", map[string]string{
		"email": "metrics@example.com", "password": "wrong-password",
	})
	require.Equal(t, http.StatusUnauthorized, code)

	// Two different ids must collapse into one route series.
	code, body := env.do(http.MethodPost, "/api/todos", token, map[string]any{"title": "a"})
	require.Equal(t, http.StatusCreated, code)
	id := decode[todoJSON](t, body).ID
	env.do(http.MethodGet, "/api/todos/"+strconv.FormatInt(id, 10), token, nil)
	env.do(http.MethodGet, "/api/todos/424242", token, nil)
	env.do(http.MethodGet, "/no/such/path", "", nil)
	env.do("BREW", "/api/todos", "", nil)

	m := env.scrape()

	for _, want := range []string{
		`gotodo_auth_registrations_total{result="success"} 1`,
		`gotodo_auth_registrations_total{result="email_taken"} 1`,
		`gotodo_auth_logins_total{result="success"} 1`,
		`gotodo_auth_logins_total{result="invalid_credentials"} 1`,
		`gotodo_http_requests_total{code="201",method="POST",route="POST /api/todos"} 1`,
		`gotodo_http_requests_total{code="200",method="GET",route="GET /api/todos/{id}"} 1`,
		`gotodo_http_requests_total{code="404",method="GET",route="GET /api/todos/{id}"} 1`,
		`gotodo_http_requests_total{code="404",method="GET",route="unmatched"} 1`,
		`gotodo_http_requests_total{code="405",method="other",route="unmatched"} 1`,
		`gotodo_http_request_duration_seconds_count{method="POST",route="POST /api/auth/login"} 2`,
		`gotodo_http_requests_in_flight 0`,
		`go_goroutines `,
		`go_sql_open_connections{db_name="sqlite"} `,
	} {
		assert.Contains(t, m, want)
	}
	assert.NotContains(t, m, "/api/todos/424242", "raw ids must not leak into labels")
	assert.NotContains(t, m, "/no/such/path", "raw paths must not become labels")
}
