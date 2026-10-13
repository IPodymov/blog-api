package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/health", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Vercel-Forwarded-For", "203.0.113.5")
	if got := clientIP(r, false); got != "127.0.0.1" {
		t.Fatalf("trusted spoofed header outside Vercel: %s", got)
	}
	if got := clientIP(r, true); got != "203.0.113.5" {
		t.Fatalf("platform client IP: %s", got)
	}
	r.Header.Set("X-Vercel-Forwarded-For", "invalid, 203.0.113.5")
	if got := clientIP(r, true); got != "127.0.0.1" {
		t.Fatalf("trusted invalid platform IP: %s", got)
	}
}
