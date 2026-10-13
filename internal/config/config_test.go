package config

import "testing"

func TestHostingPort(t *testing.T) {
	t.Chdir(t.TempDir())
	t.Setenv("DATABASE_URL", "postgres://unused")
	t.Setenv("VISITOR_SECRET", "01234567890123456789012345678901")
	t.Setenv("PUBLIC_SITE_URL", "https://example.org")
	t.Setenv("CORS_ORIGINS", "https://example.org")
	t.Setenv("HTTP_ADDR", "")
	t.Setenv("PORT", "4567")
	c, err := Load()
	if err != nil || c.Addr != ":4567" {
		t.Fatalf("addr=%q err=%v", c.Addr, err)
	}
	t.Setenv("HTTP_ADDR", "127.0.0.1:9000")
	c, err = Load()
	if err != nil || c.Addr != "127.0.0.1:9000" {
		t.Fatalf("explicit addr=%q err=%v", c.Addr, err)
	}
}
