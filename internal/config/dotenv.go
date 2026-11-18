package config

import (
	"bufio"
	"errors"
	"fmt"
	"os"
	"strings"
)

// LoadLocalEnv loads simple KEY=value settings from .env in the working
// directory. Existing environment variables take precedence. Values are never
// evaluated as shell commands, and a missing file is allowed in deployments.
func LoadLocalEnv() error {
	f, err := os.Open(".env")
	if errors.Is(err, os.ErrNotExist) {
		return nil
	}
	if err != nil {
		return errors.New("cannot open local .env")
	}
	defer f.Close()
	scanner := bufio.NewScanner(f)
	values := make(map[string]string)
	for line := 1; scanner.Scan(); line++ {
		s := strings.TrimSpace(scanner.Text())
		if s == "" || strings.HasPrefix(s, "#") {
			continue
		}
		s = strings.TrimPrefix(s, "export ")
		key, value, ok := strings.Cut(s, "=")
		key, value = strings.TrimSpace(key), strings.TrimSpace(value)
		if !ok || key == "" {
			return fmt.Errorf("invalid .env setting at line %d", line)
		}
		for i, r := range key {
			if !(r == '_' || r >= 'a' && r <= 'z' || r >= 'A' && r <= 'Z' || i > 0 && r >= '0' && r <= '9') {
				return fmt.Errorf("invalid .env key at line %d", line)
			}
		}
		if len(value) > 0 && (value[0] == '\'' || value[0] == '"') {
			if len(value) < 2 || value[len(value)-1] != value[0] {
				return fmt.Errorf("invalid .env quoted value at line %d", line)
			}
			value = value[1 : len(value)-1]
		}
		values[key] = value
	}
	if scanner.Err() != nil {
		return errors.New("cannot read local .env")
	}
	for key, value := range values {
		if _, exists := os.LookupEnv(key); !exists {
			if err := os.Setenv(key, value); err != nil {
				return errors.New("cannot apply local .env setting")
			}
		}
	}
	return nil
}
