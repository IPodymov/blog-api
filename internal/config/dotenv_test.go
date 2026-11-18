package config

import (
	"os"
	"strings"
	"testing"
)

func TestLoadLocalEnv(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := LoadLocalEnv(); err != nil {
		t.Fatal(err)
	}
	t.Setenv("BLOG_ENV_EXISTING", "external")
	const key = "BLOG_ENV_LOCAL_TEST"
	previous, existed := os.LookupEnv(key)
	os.Unsetenv(key)
	t.Cleanup(func() {
		if existed {
			os.Setenv(key, previous)
		} else {
			os.Unsetenv(key)
		}
	})
	if err := os.WriteFile(".env", []byte("# local settings\nBLOG_ENV_EXISTING=file\nexport "+key+"='literal $(command) # value'\n"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := LoadLocalEnv(); err != nil {
		t.Fatal(err)
	}
	if os.Getenv("BLOG_ENV_EXISTING") != "external" || os.Getenv(key) != "literal $(command) # value" {
		t.Fatal("environment precedence or literal value handling failed")
	}
}

func TestLoadLocalEnvMalformed(t *testing.T) {
	t.Chdir(t.TempDir())
	if err := os.WriteFile(".env", []byte("SECRET='private-value\n"), 0600); err != nil {
		t.Fatal(err)
	}
	err := LoadLocalEnv()
	if err == nil || strings.Contains(err.Error(), "private-value") {
		t.Fatal("expected a validation error without secret contents")
	}
}
