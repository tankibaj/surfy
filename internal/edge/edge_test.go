package edge

import (
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/tankibaj/server-them/internal/state"
)

func TestProxyRejectsNonLocalhost(t *testing.T) {
	t.Setenv("SERVE_THEM_STATE_DIR", t.TempDir())
	r := httptest.NewRequest(http.MethodGet, "https://example.test/", nil)
	w := httptest.NewRecorder()
	proxy(w, r)
	if w.Code != http.StatusMisdirectedRequest {
		t.Fatalf("got %d", w.Code)
	}
}

func TestProxyForwardsWebToLoopback(t *testing.T) {
	t.Setenv("SERVE_THEM_STATE_DIR", t.TempDir())
	useRecordingTransport(t)
	if e := state.Update(func(r *state.Registry) error {
		r.Projects["id"] = &state.Project{ID: "id", Name: "demo", Status: "running", Web: &state.Service{Port: 43121, Status: "running"}}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest(http.MethodGet, "https://demo.localhost/assets/app.js?v=1", nil)
	w := httptest.NewRecorder()
	proxy(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "http://127.0.0.1:43121/assets/app.js?v=1" {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

func TestProxyPreservesAPIPath(t *testing.T) {
	t.Setenv("SERVE_THEM_STATE_DIR", t.TempDir())
	useRecordingTransport(t)
	if e := state.Update(func(r *state.Registry) error {
		r.Projects["id"] = &state.Project{ID: "id", Name: "demo", Status: "running", API: &state.Service{Port: 43122, Status: "running"}}
		return nil
	}); e != nil {
		t.Fatal(e)
	}
	r := httptest.NewRequest(http.MethodGet, "https://demo.localhost/api/users?page=2", nil)
	w := httptest.NewRecorder()
	proxy(w, r)
	if w.Code != http.StatusOK || w.Body.String() != "http://127.0.0.1:43122/api/users?page=2" {
		t.Fatalf("status=%d body=%q", w.Code, w.Body.String())
	}
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }
func useRecordingTransport(t *testing.T) {
	old := proxyTransport
	t.Cleanup(func() { proxyTransport = old })
	proxyTransport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(strings.NewReader(r.URL.String()))}, nil
	})
}
