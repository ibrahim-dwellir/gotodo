package api_test

import (
	"bytes"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"gotodo/internal/api"
	"gotodo/internal/auth"
	"gotodo/internal/db"
	"gotodo/internal/store"
	"gotodo/internal/telemetry"
)

func TestMain(m *testing.M) {
	slog.SetDefault(slog.New(slog.DiscardHandler))
	os.Exit(m.Run())
}

type testEnv struct {
	t       *testing.T
	baseURL string // public API server
	opsURL  string // /metrics and /debug/pprof server
	client  *http.Client
}

// newTestEnv starts the real handler against a freshly migrated SQLite file.
func newTestEnv(t *testing.T) *testEnv {
	t.Helper()
	conn, err := db.Open(filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { _ = conn.Close() })
	require.NoError(t, db.Migrate(t.Context(), conn))

	tokens, err := auth.NewTokens([]byte("test-secret"), time.Hour)
	require.NoError(t, err)

	reg := telemetry.NewRegistry(conn)
	srv := httptest.NewServer(api.New(store.New(conn), tokens, api.NewMetrics(reg)).Handler())
	t.Cleanup(srv.Close)
	ops := httptest.NewServer(telemetry.Handler(reg))
	t.Cleanup(ops.Close)
	return &testEnv{t: t, baseURL: srv.URL, opsURL: ops.URL, client: srv.Client()}
}

// do sends a JSON request and returns the status code and raw body.
func (e *testEnv) do(method, path, token string, body any) (int, []byte) {
	e.t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(e.t, err)
		rdr = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(e.t.Context(), method, e.baseURL+path, rdr)
	require.NoError(e.t, err)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	resp, err := e.client.Do(req)
	require.NoError(e.t, err)
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	require.NoError(e.t, err)
	return resp.StatusCode, data
}

func decode[T any](t *testing.T, data []byte) T {
	t.Helper()
	var v T
	require.NoError(t, json.Unmarshal(data, &v), string(data))
	return v
}

type userJSON struct {
	ID          int64      `json:"id"`
	FirstName   string     `json:"first_name"`
	LastName    string     `json:"last_name"`
	Email       string     `json:"email"`
	Password    *string    `json:"password"`
	LastLoginAt *time.Time `json:"last_login_at"`
	CreatedAt   time.Time  `json:"created_at"`
	UpdatedAt   time.Time  `json:"updated_at"`
}

type todoJSON struct {
	ID          int64     `json:"id"`
	UserID      int64     `json:"user_id"`
	Title       string    `json:"title"`
	Description string    `json:"description"`
	Status      string    `json:"status"`
	DueDate     *string   `json:"due_date"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type loginJSON struct {
	Token     string    `json:"token"`
	ExpiresAt time.Time `json:"expires_at"`
	User      userJSON  `json:"user"`
}

func (e *testEnv) registerAndLogin(email string) (string, userJSON) {
	e.t.Helper()
	code, body := e.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"first_name": "Test", "last_name": "User", "email": email, "password": "password123",
	})
	require.Equal(e.t, http.StatusCreated, code, string(body))
	code, body = e.do(http.MethodPost, "/api/auth/login", "", map[string]string{
		"email": email, "password": "password123",
	})
	require.Equal(e.t, http.StatusOK, code, string(body))
	l := decode[loginJSON](e.t, body)
	return l.Token, l.User
}

func TestRegister_CreatesUserWithTimestamps(t *testing.T) {
	env := newTestEnv(t)

	code, body := env.do(http.MethodPost, "/api/auth/register", "", map[string]string{
		"first_name": " Ada ", "last_name": "Lovelace", "email": " Ada@Example.com ", "password": "password123",
	})
	require.Equal(t, http.StatusCreated, code, string(body))

	u := decode[userJSON](t, body)
	assert.Positive(t, u.ID)
	assert.Equal(t, "Ada", u.FirstName)
	assert.Equal(t, "Lovelace", u.LastName)
	assert.Equal(t, "ada@example.com", u.Email)
	assert.Nil(t, u.Password, "password must never be serialised")
	assert.Nil(t, u.LastLoginAt, "a fresh user has never logged in")
	assert.WithinDuration(t, time.Now(), u.CreatedAt, 5*time.Second)
	assert.Equal(t, u.CreatedAt, u.UpdatedAt)
	assert.NotContains(t, string(body), "password")
}

func TestRegister_Validation(t *testing.T) {
	env := newTestEnv(t)
	valid := func() map[string]any {
		return map[string]any{"first_name": "A", "last_name": "B", "email": "a@b.co", "password": "password123"}
	}

	tests := []struct {
		name    string
		mutate  func(map[string]any)
		wantMsg string
	}{
		{name: "missing first name", mutate: func(m map[string]any) { m["first_name"] = "  " }, wantMsg: "first_name is required"},
		{name: "missing last name", mutate: func(m map[string]any) { delete(m, "last_name") }, wantMsg: "last_name is required"},
		{name: "bad email", mutate: func(m map[string]any) { m["email"] = "nope" }, wantMsg: "email is not a valid address"},
		{name: "email with display name", mutate: func(m map[string]any) { m["email"] = "Bob <bob@x.io>" }, wantMsg: "email is not a valid address"},
		{name: "short password", mutate: func(m map[string]any) { m["password"] = "short" }, wantMsg: "password must be at least 8 characters"},
		{name: "unknown field", mutate: func(m map[string]any) { m["admin"] = true }, wantMsg: `invalid JSON body: json: unknown field "admin"`},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			req := valid()
			tc.mutate(req)
			code, body := env.do(http.MethodPost, "/api/auth/register", "", req)
			assert.Equal(t, http.StatusBadRequest, code)
			assert.JSONEq(t, `{"error":`+strconv.Quote(tc.wantMsg)+`}`, string(body))
		})
	}
}

func TestRegister_DuplicateEmailIsCaseInsensitive(t *testing.T) {
	env := newTestEnv(t)
	req := map[string]string{"first_name": "A", "last_name": "B", "email": "dup@example.com", "password": "password123"}

	code, _ := env.do(http.MethodPost, "/api/auth/register", "", req)
	require.Equal(t, http.StatusCreated, code)

	req["email"] = "DUP@example.com"
	code, body := env.do(http.MethodPost, "/api/auth/register", "", req)
	assert.Equal(t, http.StatusConflict, code)
	assert.JSONEq(t, `{"error":"email is already registered"}`, string(body))
}

func TestLogin_RecordsLastLoginButNotUpdatedAt(t *testing.T) {
	env := newTestEnv(t)
	token, user := env.registerAndLogin("login@example.com")

	require.NotEmpty(t, token)
	require.NotNil(t, user.LastLoginAt)
	assert.WithinDuration(t, time.Now(), *user.LastLoginAt, 5*time.Second)
	assert.True(t, user.LastLoginAt.After(user.CreatedAt))
	assert.Equal(t, user.CreatedAt, user.UpdatedAt, "logging in is not a profile modification")

	code, body := env.do(http.MethodGet, "/api/users/me", token, nil)
	require.Equal(t, http.StatusOK, code)
	me := decode[userJSON](t, body)
	assert.Equal(t, user.ID, me.ID)
	assert.Equal(t, user.LastLoginAt.UTC(), me.LastLoginAt.UTC())
}

func TestLogin_RejectsBadCredentials(t *testing.T) {
	env := newTestEnv(t)
	env.registerAndLogin("real@example.com")

	tests := []struct {
		name     string
		email    string
		password string
	}{
		{name: "wrong password", email: "real@example.com", password: "wrong-password"},
		{name: "unknown email", email: "ghost@example.com", password: "password123"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := env.do(http.MethodPost, "/api/auth/login", "", map[string]string{
				"email": tc.email, "password": tc.password,
			})
			assert.Equal(t, http.StatusUnauthorized, code)
			assert.JSONEq(t, `{"error":"invalid email or password"}`, string(body))
		})
	}
}

func TestUpdateMe_BumpsUpdatedAt(t *testing.T) {
	env := newTestEnv(t)
	token, user := env.registerAndLogin("me@example.com")

	code, body := env.do(http.MethodPut, "/api/users/me", token, map[string]string{
		"first_name": "Grace", "last_name": "Hopper",
	})
	require.Equal(t, http.StatusOK, code, string(body))
	u := decode[userJSON](t, body)
	assert.Equal(t, "Grace", u.FirstName)
	assert.Equal(t, "Hopper", u.LastName)
	assert.True(t, u.UpdatedAt.After(user.UpdatedAt))
	assert.Equal(t, user.CreatedAt.UTC(), u.CreatedAt.UTC())
}

func TestProtectedRoutes_RequireValidToken(t *testing.T) {
	env := newTestEnv(t)

	otherIssuer, err := auth.NewTokens([]byte("some-other-secret"), time.Hour)
	require.NoError(t, err)
	forged, _, err := otherIssuer.Issue(1)
	require.NoError(t, err)

	routes := []struct{ method, path string }{
		{http.MethodGet, "/api/users/me"},
		{http.MethodPut, "/api/users/me"},
		{http.MethodGet, "/api/todos"},
		{http.MethodPost, "/api/todos"},
		{http.MethodGet, "/api/todos/1"},
		{http.MethodPut, "/api/todos/1"},
		{http.MethodDelete, "/api/todos/1"},
	}
	for _, rt := range routes {
		for _, tok := range []struct{ name, value string }{{"no token", ""}, {"garbage token", "abc.def.ghi"}, {"wrong secret", forged}} {
			t.Run(rt.method+" "+rt.path+" "+tok.name, func(t *testing.T) {
				code, _ := env.do(rt.method, rt.path, tok.value, nil)
				assert.Equal(t, http.StatusUnauthorized, code)
			})
		}
	}
}

func TestTodos_CRUD(t *testing.T) {
	env := newTestEnv(t)
	token, user := env.registerAndLogin("crud@example.com")

	// Create with defaults.
	code, body := env.do(http.MethodPost, "/api/todos", token, map[string]any{"title": "  Buy milk  "})
	require.Equal(t, http.StatusCreated, code, string(body))
	created := decode[todoJSON](t, body)
	assert.Equal(t, user.ID, created.UserID)
	assert.Equal(t, "Buy milk", created.Title)
	assert.Equal(t, "", created.Description)
	assert.Equal(t, "not_started", created.Status)
	assert.Nil(t, created.DueDate)
	assert.Equal(t, created.CreatedAt, created.UpdatedAt)

	// Create with every field.
	code, body = env.do(http.MethodPost, "/api/todos", token, map[string]any{
		"title": "Ship release", "description": "v1.0", "status": "in_progress", "due_date": "2026-12-31",
	})
	require.Equal(t, http.StatusCreated, code, string(body))
	second := decode[todoJSON](t, body)
	require.NotNil(t, second.DueDate)
	assert.Equal(t, "2026-12-31", *second.DueDate)

	// List: newest first.
	code, body = env.do(http.MethodGet, "/api/todos", token, nil)
	require.Equal(t, http.StatusOK, code)
	list := decode[[]todoJSON](t, body)
	require.Len(t, list, 2)
	assert.Equal(t, second.ID, list[0].ID)
	assert.Equal(t, created.ID, list[1].ID)

	// List filtered by status.
	code, body = env.do(http.MethodGet, "/api/todos?status=in_progress", token, nil)
	require.Equal(t, http.StatusOK, code)
	filtered := decode[[]todoJSON](t, body)
	require.Len(t, filtered, 1)
	assert.Equal(t, second.ID, filtered[0].ID)

	// Get.
	path := "/api/todos/" + strconv.FormatInt(created.ID, 10)
	code, body = env.do(http.MethodGet, path, token, nil)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, created, decode[todoJSON](t, body))

	// Put replaces fields and bumps updated_at only.
	code, body = env.do(http.MethodPut, path, token, map[string]any{
		"title": "Buy oat milk", "description": "2 litres", "status": "in_review", "due_date": "2026-11-30",
	})
	require.Equal(t, http.StatusOK, code, string(body))
	updated := decode[todoJSON](t, body)
	assert.Equal(t, "Buy oat milk", updated.Title)
	assert.Equal(t, "2 litres", updated.Description)
	assert.Equal(t, "in_review", updated.Status)
	require.NotNil(t, updated.DueDate)
	assert.Equal(t, "2026-11-30", *updated.DueDate)
	assert.Equal(t, created.CreatedAt.UTC(), updated.CreatedAt.UTC())
	assert.True(t, updated.UpdatedAt.After(created.UpdatedAt))

	// Put without due_date clears it.
	code, body = env.do(http.MethodPut, path, token, map[string]any{"title": "Buy oat milk", "status": "completed"})
	require.Equal(t, http.StatusOK, code, string(body))
	assert.Nil(t, decode[todoJSON](t, body).DueDate)

	// Delete, then it is gone.
	code, _ = env.do(http.MethodDelete, path, token, nil)
	assert.Equal(t, http.StatusNoContent, code)
	code, _ = env.do(http.MethodGet, path, token, nil)
	assert.Equal(t, http.StatusNotFound, code)
	code, _ = env.do(http.MethodDelete, path, token, nil)
	assert.Equal(t, http.StatusNotFound, code)
}

func TestTodos_Validation(t *testing.T) {
	env := newTestEnv(t)
	token, _ := env.registerAndLogin("val@example.com")
	code, body := env.do(http.MethodPost, "/api/todos", token, map[string]any{"title": "x"})
	require.Equal(t, http.StatusCreated, code)
	path := "/api/todos/" + strconv.FormatInt(decode[todoJSON](t, body).ID, 10)

	tests := []struct {
		name    string
		method  string
		path    string
		body    any
		want    int
		wantMsg string
	}{
		{name: "post missing title", method: http.MethodPost, path: "/api/todos", body: map[string]any{"title": " "}, want: 400, wantMsg: "title is required"},
		{name: "post bad status", method: http.MethodPost, path: "/api/todos", body: map[string]any{"title": "a", "status": "done"}, want: 400, wantMsg: "status must be one of: not_started, in_progress, in_review, completed"},
		{name: "post bad due date", method: http.MethodPost, path: "/api/todos", body: map[string]any{"title": "a", "due_date": "31/12/2026"}, want: 400, wantMsg: "due_date must be a date in YYYY-MM-DD format"},
		{name: "post impossible date", method: http.MethodPost, path: "/api/todos", body: map[string]any{"title": "a", "due_date": "2026-02-30"}, want: 400, wantMsg: "due_date must be a date in YYYY-MM-DD format"},
		{name: "put missing status", method: http.MethodPut, path: path, body: map[string]any{"title": "a"}, want: 400, wantMsg: "status is required"},
		{name: "empty body", method: http.MethodPost, path: "/api/todos", body: nil, want: 400, wantMsg: "request body must not be empty"},
		{name: "list bad status filter", method: http.MethodGet, path: "/api/todos?status=nope", want: 400, wantMsg: "status must be one of: not_started, in_progress, in_review, completed"},
		{name: "non-numeric id", method: http.MethodGet, path: "/api/todos/abc", want: 404, wantMsg: "todo not found"},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			code, body := env.do(tc.method, tc.path, token, tc.body)
			assert.Equal(t, tc.want, code)
			assert.JSONEq(t, `{"error":`+strconv.Quote(tc.wantMsg)+`}`, string(body))
		})
	}
}

func TestTodos_UsersOnlySeeTheirOwn(t *testing.T) {
	env := newTestEnv(t)
	alice, _ := env.registerAndLogin("alice@example.com")
	bob, _ := env.registerAndLogin("bob@example.com")

	code, body := env.do(http.MethodPost, "/api/todos", alice, map[string]any{"title": "Alice's secret"})
	require.Equal(t, http.StatusCreated, code)
	aliceTodo := decode[todoJSON](t, body)
	path := "/api/todos/" + strconv.FormatInt(aliceTodo.ID, 10)

	code, body = env.do(http.MethodGet, "/api/todos", bob, nil)
	require.Equal(t, http.StatusOK, code)
	assert.JSONEq(t, `[]`, string(body), "bob's list must not include alice's todo")

	code, _ = env.do(http.MethodGet, path, bob, nil)
	assert.Equal(t, http.StatusNotFound, code)

	code, _ = env.do(http.MethodPut, path, bob, map[string]any{"title": "pwned", "status": "completed"})
	assert.Equal(t, http.StatusNotFound, code)

	code, _ = env.do(http.MethodDelete, path, bob, nil)
	assert.Equal(t, http.StatusNotFound, code)

	// Alice's todo is untouched.
	code, body = env.do(http.MethodGet, path, alice, nil)
	require.Equal(t, http.StatusOK, code)
	assert.Equal(t, aliceTodo, decode[todoJSON](t, body))
}

func TestSwagger_ServesUIAndSpec(t *testing.T) {
	env := newTestEnv(t)

	code, body := env.do(http.MethodGet, "/swagger/index.html", "", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), "swagger-ui")
	assert.Contains(t, string(body), `openapi.yaml`)

	code, _ = env.do(http.MethodGet, "/swagger/swagger-ui-bundle.js", "", nil)
	assert.Equal(t, http.StatusOK, code, "swagger assets are bundled, not fetched from a CDN")

	code, body = env.do(http.MethodGet, "/swagger/openapi.yaml", "", nil)
	assert.Equal(t, http.StatusOK, code)
	assert.Contains(t, string(body), "openapi: 3.0.3")
}

func TestMigrate_IsIdempotent(t *testing.T) {
	path := filepath.Join(t.TempDir(), "twice.db")
	for range 2 {
		conn, err := db.Open(path)
		require.NoError(t, err)
		require.NoError(t, db.Migrate(t.Context(), conn))
		require.NoError(t, conn.Close())
	}
}
