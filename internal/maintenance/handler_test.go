package maintenance

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestHandlerAuthorization(t *testing.T) {
	secret := strings.Repeat("s", 32)
	for _, test := range []struct {
		name, method, secret, authorization string
		want                                int
		called                              bool
	}{
		{"missing", "GET", secret, "", 401, false},
		{"wrong", "GET", secret, "Bearer wrong", 401, false},
		{"disabled", "GET", "", "Bearer ", 401, false},
		{"weak", "GET", "weak", "Bearer weak", 401, false},
		{"method", "POST", secret, "Bearer " + secret, 405, false},
		{"valid", "GET", secret, "Bearer " + secret, 204, true},
	} {
		t.Run(test.name, func(t *testing.T) {
			called := false
			h := Handler(test.secret, func(ctx context.Context) error {
				called = true
				if _, ok := ctx.Deadline(); !ok {
					t.Error("missing maintenance deadline")
				}
				return nil
			}, slog.New(slog.NewTextHandler(io.Discard, nil)))
			r := httptest.NewRequest(test.method, "/internal/maintenance", nil)
			r.Header.Set("Authorization", test.authorization)
			w := httptest.NewRecorder()
			h.ServeHTTP(w, r)
			if w.Code != test.want || called != test.called {
				t.Fatalf("status=%d called=%v", w.Code, called)
			}
			if w.Header().Get("Cache-Control") != "no-store" {
				t.Fatal("maintenance response must not be cached")
			}
		})
	}
}

func TestHandlerHidesDatabaseErrors(t *testing.T) {
	secret := strings.Repeat("s", 32)
	h := Handler(secret, func(context.Context) error { return errors.New("private database credentials") }, slog.New(slog.NewTextHandler(io.Discard, nil)))
	r := httptest.NewRequest("GET", "/internal/maintenance", nil)
	r.Header.Set("Authorization", "Bearer "+secret)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	if w.Code != 500 || strings.Contains(w.Body.String(), "private") {
		t.Fatalf("unexpected error response: %d %s", w.Code, w.Body.String())
	}
}
