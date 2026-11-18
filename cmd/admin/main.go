// Command admin manages platform admins directly in the database; the public API cannot create them.
//
//	go run ./cmd/admin create -email admin@example.org
//	go run ./cmd/admin reset-password -email admin@example.org
//
// The password is read from ADMIN_PASSWORD or generated and printed once.
package main

import (
	"blog-api/internal/config"
	"blog-api/internal/repository"
	"blog-api/internal/service"
	"blog-api/migrations"
	"context"
	"flag"
	"fmt"
	"log"
	"os"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func main() {
	if len(os.Args) < 2 || (os.Args[1] != "create" && os.Args[1] != "reset-password") {
		log.Fatal("usage: admin create|reset-password -email EMAIL")
	}
	fs := flag.NewFlagSet(os.Args[1], flag.ExitOnError)
	email := fs.String("email", "", "admin email")
	fs.Parse(os.Args[2:])
	if *email == "" {
		log.Fatal("-email is required")
	}
	if err := config.LoadLocalEnv(); err != nil {
		log.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), time.Minute)
	defer cancel()
	databaseURL := os.Getenv("DATABASE_URL_UNPOOLED")
	if databaseURL == "" {
		databaseURL = os.Getenv("DATABASE_URL")
	}
	p, e := pgxpool.New(ctx, databaseURL)
	if e != nil {
		log.Fatal("database configuration failed")
	}
	defer p.Close()
	// Migrations are idempotent; applying them here lets the command run before the first deploy.
	if e = migrations.Apply(ctx, p); e != nil {
		log.Fatal(e)
	}
	password, generated := os.Getenv("ADMIN_PASSWORD"), false
	if password == "" {
		password, generated = service.Random()[:24], true
	}
	s := &service.Service{Repo: &repository.Store{DB: p}}
	if os.Args[1] == "create" {
		a, e := s.CreateAdmin(ctx, *email, password)
		if e != nil {
			log.Fatal("create admin: ", e)
		}
		fmt.Printf("admin created: id=%d email=%s\n", a.ID, a.Email)
	} else if e = s.SetAdminPassword(ctx, *email, password); e != nil {
		log.Fatal("reset password: ", e)
	} else {
		fmt.Println("password changed, sessions revoked")
	}
	if generated {
		fmt.Println("password:", password)
	}
}
