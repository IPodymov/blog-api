package config

import (
	"fmt"
	"net/url"
	"os"
	"strconv"
	"strings"
	"time"
)

type Config struct {
	DatabaseURL, Addr, PublicURL, VisitorSecret string
	Origins                                     []string
	SecureCookie                                bool
	AccessTTL, RefreshTTL, ViewInterval         time.Duration
	MaxPeriod                                   int
}

func env(k, d string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return d
}
func Load() (Config, error) {
	if err := LoadLocalEnv(); err != nil {
		return Config{}, err
	}
	c := Config{DatabaseURL: os.Getenv("DATABASE_URL"), Addr: env("HTTP_ADDR", ":8080"), PublicURL: env("PUBLIC_SITE_URL", "http://localhost:3000"), VisitorSecret: os.Getenv("VISITOR_SECRET"), Origins: strings.Split(env("CORS_ORIGINS", "http://localhost:3000"), ","), MaxPeriod: 366}
	if c.DatabaseURL == "" || len(c.VisitorSecret) < 32 {
		return c, fmt.Errorf("DATABASE_URL and VISITOR_SECRET (>=32 bytes) required")
	}
	u, e := url.Parse(c.PublicURL)
	if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.RawQuery != "" || u.Fragment != "" || u.User != nil {
		return c, fmt.Errorf("invalid PUBLIC_SITE_URL")
	}
	c.SecureCookie, e = strconv.ParseBool(env("COOKIE_SECURE", "true"))
	if e != nil {
		return c, e
	}
	for _, x := range []struct {
		k, d string
		p    *time.Duration
	}{{"ACCESS_TTL", "15m", &c.AccessTTL}, {"REFRESH_TTL", "720h", &c.RefreshTTL}, {"VIEW_INTERVAL", "30m", &c.ViewInterval}} {
		v, e := time.ParseDuration(env(x.k, x.d))
		if e != nil || v <= 0 {
			return c, fmt.Errorf("invalid %s", x.k)
		}
		*x.p = v
	}
	if c.AccessTTL > time.Hour || c.RefreshTTL <= c.AccessTTL {
		return c, fmt.Errorf("ACCESS_TTL must be <=1h and shorter than REFRESH_TTL")
	}
	c.MaxPeriod, e = strconv.Atoi(env("MAX_STATISTICS_DAYS", "366"))
	if e != nil || c.MaxPeriod < 30 || c.MaxPeriod > 3660 {
		return c, fmt.Errorf("invalid MAX_STATISTICS_DAYS")
	}
	for i, o := range c.Origins {
		c.Origins[i] = strings.TrimSpace(o)
		u, e := url.Parse(c.Origins[i])
		if e != nil || u.Host == "" || (u.Scheme != "http" && u.Scheme != "https") || u.Path != "" || u.RawQuery != "" || u.Fragment != "" {
			return c, fmt.Errorf("invalid CORS origin")
		}
	}
	return c, nil
}
