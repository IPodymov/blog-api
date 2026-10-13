package main

import (
	"blog-api/internal/config"
	"blog-api/migrations"
	"context"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if err := config.LoadLocalEnv(); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	p, e := pgxpool.New(ctx, os.Getenv("DATABASE_URL"))
	if e != nil {
		log.Fatal("database configuration failed")
	}
	defer p.Close()
	if e = migrations.Apply(ctx, p); e != nil {
		log.Fatal(e)
	}
	log.Print("migrations applied")
}
