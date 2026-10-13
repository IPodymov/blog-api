//go:build integration

package repository_test

import (
	"blog-api/internal/domain"
	"blog-api/internal/repository"
	"blog-api/internal/service"
	"blog-api/migrations"
	"context"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"sync"
	"testing"
	"time"
)

func TestPostgresConstraintsAndSessions(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL required")
	}
	ctx := context.Background()
	root, e := pgxpool.New(ctx, dsn)
	if e != nil {
		t.Fatal(e)
	}
	defer root.Close()
	schema := "test_" + service.Hash(service.Random())[:16]
	if _, e = root.Exec(ctx, "CREATE SCHEMA "+schema); e != nil {
		t.Fatal(e)
	}
	defer root.Exec(ctx, "DROP SCHEMA "+schema+" CASCADE")
	pc, e := pgxpool.ParseConfig(dsn)
	if e != nil {
		t.Fatal(e)
	}
	pc.ConnConfig.RuntimeParams["search_path"] = schema
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if e = migrations.Apply(ctx, pool); e != nil {
		t.Fatal(e)
	}
	r := &repository.Store{DB: pool}
	u, e := r.CreateUser(ctx, "owner", "owner@example.org", "test-only-hash")
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.CreateUser(ctx, "owner", "other@example.org", "hash"); e == nil {
		t.Fatal("username uniqueness absent")
	}
	a, e := r.CreateArticle(ctx, u.ID, domain.ArticleInput{Title: "One", Slug: "one", Content: "body", Status: "published"})
	if e != nil {
		t.Fatal(e)
	}
	b, e := r.CreateArticle(ctx, u.ID, domain.ArticleInput{Title: "Two", Slug: "two", Content: "body", Status: "published"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, "INSERT INTO likes(article_id,user_id) VALUES($1,$2)", a.ID, u.ID); e != nil {
		t.Fatal(e)
	}
	if _, e = pool.Exec(ctx, "INSERT INTO likes(article_id,user_id) VALUES($1,$2)", a.ID, u.ID); e == nil {
		t.Fatal("like uniqueness absent")
	}
	parent, e := r.CreateComment(ctx, a.ID, u.ID, domain.CommentInput{Content: "Root"})
	if e != nil {
		t.Fatal(e)
	}
	if _, e = r.CreateComment(ctx, b.ID, u.ID, domain.CommentInput{Content: "Wrong article", ParentID: &parent.ID}); e == nil {
		t.Fatal("cross-article comment allowed")
	}
	// Exact UTC day boundaries: three values, only inclusive requested days count.
	if _, e = pool.Exec(ctx, "INSERT INTO article_daily_views VALUES($1,'2026-01-01',3),($1,'2026-01-02',4),($1,'2026-01-03',8)", a.ID); e != nil {
		t.Fatal(e)
	}
	p, _ := domain.ParsePeriod("2026-01-01", "2026-01-02", time.Date(2026, 2, 1, 0, 0, 0, 0, time.UTC), 366)
	days, total, e := r.Daily(ctx, u.ID, a.ID, p)
	if e != nil || total != 7 || len(days) != 2 || days[0].Date != "2026-01-01" {
		t.Fatal("UTC date boundaries wrong", days, total, e)
	}
	access, refresh, family := service.Random(), service.Random(), service.Random()
	if e = r.NewSession(ctx, u.ID, family, service.Hash(access), service.Hash(refresh), time.Now().Add(time.Hour), time.Now().Add(24*time.Hour)); e != nil {
		t.Fatal(e)
	}
	var wg sync.WaitGroup
	out := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			out <- r.Rotate(ctx, service.Hash(refresh), service.Hash(service.Random()), service.Hash(service.Random()), time.Now().Add(time.Hour))
		}()
	}
	wg.Wait()
	close(out)
	wins, failures := 0, 0
	for e := range out {
		if e == nil {
			wins++
		} else if de, ok := e.(*domain.Error); ok && de.Status == 401 {
			failures++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || failures != 1 {
		t.Fatal("refresh race", wins, failures)
	}
	if _, e = r.Authenticate(ctx, service.Hash(access)); e == nil {
		t.Fatal("replay did not revoke access")
	}
	if _, e = pool.Exec(ctx, "DELETE FROM users WHERE id=$1", u.ID); e != nil {
		t.Fatal(e)
	}
	for _, table := range []string{"articles", "likes", "comments", "session_families", "refresh_sessions", "access_sessions", "article_daily_views"} {
		var count int64
		if e = pool.QueryRow(ctx, fmt.Sprintf("SELECT count(*) FROM %s", table)).Scan(&count); e != nil || count != 0 {
			t.Fatal("cascade failed", table, count, e)
		}
	}
}
