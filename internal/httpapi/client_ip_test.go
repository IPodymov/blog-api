package httpapi

import (
	"net/http/httptest"
	"testing"
)

func TestClientIP(t *testing.T) {
	r := httptest.NewRequest("GET", "/health", nil)
	r.RemoteAddr = "127.0.0.1:1234"
	r.Header.Set("X-Forwarded-For", "203.0.113.5")
	if got := clientIP(r); got != "127.0.0.1" {
		t.Fatalf("trusted spoofed header: %s", got)
	}
}
