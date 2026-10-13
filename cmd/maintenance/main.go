// maintenance is an idempotent operator command, suitable for a daily cron job.
package main

import (
	"blog-api/internal/config"
	"context"
	"github.com/jackc/pgx/v5/pgxpool"
	"log"
	"os"
	"time"
)

func main() {
	c, e := config.Load()
	if e != nil {
		log.Fatal(e)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	defer cancel()
	p, e := pgxpool.New(ctx, c.DatabaseURL)
	if e != nil {
		log.Fatal("invalid database configuration")
	}
	defer p.Close()
	tx, e := p.Begin(ctx)
	if e != nil {
		log.Fatal("database connection failed")
	}
	defer tx.Rollback(ctx)
	for _, q := range []struct {
		sql  string
		args []any
	}{{"DELETE FROM view_dedup WHERE last_seen < clock_timestamp() - ($1::double precision * interval '1 second')", []any{c.ViewInterval.Seconds()}}, {"DELETE FROM access_sessions WHERE expires_at <= now()", nil}, {"DELETE FROM session_families WHERE expires_at <= now()", nil}} {
		if _, e = tx.Exec(ctx, q.sql, q.args...); e != nil {
			log.Fatal("maintenance query failed")
		}
	}
	if e = tx.Commit(ctx); e != nil {
		log.Fatal("maintenance commit failed")
	}
	log.New(os.Stdout, "", 0).Print("maintenance completed")
}
