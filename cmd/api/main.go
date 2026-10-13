package main

import (
	"blog-api/internal/config"
	"blog-api/internal/httpapi"
	"blog-api/internal/maintenance"
	"blog-api/internal/repository"
	"blog-api/internal/service"
	"context"
	"errors"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	log := slog.New(slog.NewJSONHandler(os.Stdout, nil))
	if e := run(log); e != nil {
		log.Error("startup failed", "error_type", e.Error())
		os.Exit(1)
	}
}
func run(log *slog.Logger) error {
	c, e := config.Load()
	if e != nil {
		return e
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	poolConfig, e := pgxpool.ParseConfig(c.DatabaseURL)
	if e != nil {
		return errors.New("database configuration failed")
	}
	// Keep idle connections low when Vercel scales instances horizontally.
	poolConfig.MaxConns = 5
	poolConfig.MinConns = 0
	poolConfig.MaxConnIdleTime = time.Minute
	pool, e := pgxpool.NewWithConfig(ctx, poolConfig)
	if e != nil {
		return errors.New("database configuration failed")
	}
	defer pool.Close()
	if e = pool.Ping(ctx); e != nil {
		return errors.New("database connection failed")
	}
	s := &service.Service{Repo: &repository.Store{DB: pool}, PublicURL: c.PublicURL, VisitorSecret: c.VisitorSecret, AccessTTL: c.AccessTTL, RefreshTTL: c.RefreshTTL, ViewInterval: c.ViewInterval, MaxPeriod: c.MaxPeriod}
	mux := http.NewServeMux()
	mux.Handle("/internal/maintenance", maintenance.Handler(os.Getenv("CRON_SECRET"), func(ctx context.Context) error {
		return maintenance.Run(ctx, pool, c.ViewInterval)
	}, log))
	mux.Handle("/", httpapi.New(s, c, log))
	server := &http.Server{Addr: c.Addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 15 * time.Second, WriteTimeout: 60 * time.Second, IdleTimeout: 60 * time.Second, MaxHeaderBytes: 16 << 10}
	defer server.Close()
	stop, done := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer done()
	errs := make(chan error, 1)
	go func() { log.Info("listening", "addr", c.Addr); errs <- server.ListenAndServe() }()
	select {
	case e := <-errs:
		if errors.Is(e, http.ErrServerClosed) {
			return nil
		}
		return e
	case <-stop.Done():
		log.Info("shutting down", "addr", c.Addr)
		shutdown, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdown); err != nil {
			return err
		}
		log.Info("stopped", "addr", c.Addr)
		return nil
	}
}
