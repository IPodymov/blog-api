// maintenance is an idempotent operator command, suitable for a daily cron job.
package main

import (
	"blog-api/internal/config"
	"blog-api/internal/maintenance"
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
	if e = maintenance.Run(ctx, p, c.ViewInterval); e != nil {
		log.Fatal("maintenance failed")
	}
	log.New(os.Stdout, "", 0).Print("maintenance completed")
}
