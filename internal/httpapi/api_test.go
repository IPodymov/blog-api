package httpapi

import (
	"blog-api/internal/config"
	"blog-api/internal/domain"
	"blog-api/internal/service"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestPatchSemantics(t *testing.T) {
	for _, tc := range []struct {
		body string
		ok   bool
	}{{`{"description":""}`, true}, {`{"tags":[]}`, true}, {`{"title":null}`, false}, {`{}`, false}, {`{"unknown":1}`, false}, {`null`, false}, {`{"title":"new"} {}`, false}} {
		r := httptest.NewRequest("PATCH", "/", strings.NewReader(tc.body))
		r.Header.Set("Content-Type", "application/json")
		var p domain.ArticlePatch
		e := decode(r, &p, true)
		if (e == nil) != tc.ok {
			t.Errorf("%s: %v", tc.body, e)
		}
	}
}
func TestETag(t *testing.T) {
	for _, tc := range []struct {
		header string
		status int
	}{{"", 428}, {"1", 422}, {`W/"1"`, 422}, {`"0"`, 422}, {`"1"`, 0}} {
		r := httptest.NewRequest("PATCH", "/", nil)
		r.Header.Set("If-Match", tc.header)
		_, e := version(r)
		var de *domain.Error
		status := 0
		if errors.As(e, &de) {
			status = de.Status
		}
		if status != tc.status {
			t.Errorf("%s: %v", tc.header, e)
		}
	}
}
func TestFilters(t *testing.T) {
	for _, q := range []string{"page_size=101", "page=0", "sort=sql-injection", "author_id=-1", "status=secret"} {
		r := httptest.NewRequest("GET", "/?"+q, nil)
		if _, e := filter(r, false); e == nil {
			t.Error("invalid filter accepted", q)
		}
	}
}

func TestMiddlewareBoundaries(t *testing.T) {
	h := New(&service.Service{}, config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, tc := range []struct {
		method, path, origin, authorization string
		length                              int64
		status                              int
	}{
		{"GET", "/health", "", "", 0, 200}, {"GET", "/health", "https://any.example", "", 0, 200},
		{"OPTIONS", "/api/v1/articles", "https://blog.example", "", 0, 204},
		{"GET", "/health", "", "Bearer invalid", 0, 200}, {"POST", "/api/v1/articles", "", "", 513 << 10, 413},
		{"GET", "/api/v1/articles", "", "", 0, 400}, {"GET", "/api/v1/me", "", "Bearer invalid", 0, 400},
		{"GET", "/not-found", "", "", 0, 404},
	} {
		r := httptest.NewRequest(tc.method, tc.path, nil)
		r.Header.Set("Origin", tc.origin)
		r.Header.Set("Authorization", tc.authorization)
		r.ContentLength = tc.length
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != tc.status {
			t.Errorf("%+v: %d %s", tc, w.Code, w.Body.String())
		}
		if w.Header().Get("X-Request-ID") == "" {
			t.Error("request ID missing")
		}
	}
}

func TestCORSAllowsAnyOrigin(t *testing.T) {
	h := New(&service.Service{}, config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	for _, origin := range []string{"https://blog.example", "http://localhost:3001", "null"} {
		r := httptest.NewRequest("OPTIONS", "/api/v1/articles", nil)
		r.Header.Set("Origin", origin)
		r.Header.Set("Access-Control-Request-Method", "POST")
		r.Header.Set("Access-Control-Request-Headers", "x-site-key")
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		if w.Code != 204 || w.Header().Get("Access-Control-Allow-Origin") != origin || w.Header().Get("Vary") != "Origin" {
			t.Errorf("%s: %d %v", origin, w.Code, w.Header())
		}
		if !strings.Contains(w.Header().Get("Access-Control-Allow-Headers"), "X-Site-Key") {
			t.Errorf("%s: X-Site-Key not allowed: %v", origin, w.Header())
		}
	}
	r := httptest.NewRequest("GET", "/health", nil)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Header().Get("Access-Control-Allow-Origin") != "" {
		t.Errorf("CORS headers without Origin: %v", w.Header())
	}
}

func TestRootRedirectsToDocs(t *testing.T) {
	h := New(&service.Service{}, config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	w := httptest.NewRecorder()
	h.ServeHTTP(w, httptest.NewRequest("GET", "/", nil))
	if w.Code != http.StatusFound || w.Header().Get("Location") != "/docs" {
		t.Fatalf("got %d, Location %q", w.Code, w.Header().Get("Location"))
	}
}
