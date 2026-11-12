package web_test

import (
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"gotodo/ui/web"
)

func get(t *testing.T, srv *httptest.Server, path string) (int, http.Header, string) {
	t.Helper()
	resp, err := srv.Client().Get(srv.URL + path)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, resp.Header, string(b)
}

func TestHandler_ServesStaticAndProxiesAPI(t *testing.T) {
	var gotPath, gotAuth string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath, gotAuth = r.URL.RequestURI(), r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`[]`))
	}))
	t.Cleanup(upstream.Close)
	apiURL, _ := url.Parse(upstream.URL)

	srv := httptest.NewServer(web.NewHandler(apiURL))
	t.Cleanup(srv.Close)

	code, _, body := get(t, srv, "/")
	if code != http.StatusOK || !strings.Contains(body, `x-data="app"`) {
		t.Fatalf("index: got %d %q", code, body[:min(len(body), 80)])
	}
	code, hdr, _ := get(t, srv, "/app.js")
	if code != http.StatusOK || !strings.Contains(hdr.Get("Content-Type"), "javascript") {
		t.Fatalf("app.js: got %d %q", code, hdr.Get("Content-Type"))
	}

	req, _ := http.NewRequest(http.MethodGet, srv.URL+"/api/todos?status=completed", nil)
	req.Header.Set("Authorization", "Bearer abc")
	resp, err := srv.Client().Do(req)
	if err != nil {
		t.Fatal(err)
	}
	resp.Body.Close()
	if resp.StatusCode != http.StatusOK || gotPath != "/api/todos?status=completed" || gotAuth != "Bearer abc" {
		t.Fatalf("proxy: status=%d path=%q auth=%q", resp.StatusCode, gotPath, gotAuth)
	}
}

func TestHandler_APIDownReturnsJSON502(t *testing.T) {
	apiURL, _ := url.Parse("http://127.0.0.1:1") // nothing listens on port 1
	srv := httptest.NewServer(web.NewHandler(apiURL))
	t.Cleanup(srv.Close)

	code, _, body := get(t, srv, "/api/todos")
	if code != http.StatusBadGateway || body != `{"error":"the API server is unreachable"}` {
		t.Fatalf("got %d %q", code, body)
	}
}
