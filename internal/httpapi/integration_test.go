//go:build integration

package httpapi

import (
	"blog-api/internal/config"
	"blog-api/internal/domain"
	"blog-api/internal/repository"
	"blog-api/internal/service"
	"blog-api/migrations"
	"context"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5/pgxpool"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"
)

func TestEndToEnd(t *testing.T) {
	dsn := os.Getenv("TEST_DATABASE_URL")
	if dsn == "" {
		t.Fatal("TEST_DATABASE_URL is required for integration tests")
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
	pc.ConnConfig.RuntimeParams["timezone"] = "UTC"
	pc.MaxConns = 20
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if e = migrations.Apply(ctx, pool); e != nil {
		t.Fatal(e)
	}
	if e = migrations.Apply(ctx, pool); e != nil {
		t.Fatal("migration not repeatable", e)
	}
	repo := &repository.Store{DB: pool}
	s := &service.Service{Repo: repo, PublicURL: "https://blog.example", VisitorSecret: strings.Repeat("s", 32), AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour, ViewInterval: 30 * time.Minute, MaxPeriod: 366}
	c := config.Config{Origins: []string{"https://blog.example"}, SecureCookie: true}
	h := New(s, c, slog.New(slog.NewTextHandler(io.Discard, nil)))
	call := func(method, path, body, token, etag string) *httptest.ResponseRecorder {
		r := httptest.NewRequest(method, "/api/v1"+path, strings.NewReader(body))
		r.RemoteAddr = "127.0.0.1:1234"
		if body != "" {
			r.Header.Set("Content-Type", "application/json")
		}
		if token != "" {
			r.Header.Set("Authorization", "Bearer "+token)
		}
		if etag != "" {
			r.Header.Set("If-Match", etag)
		}
		w := httptest.NewRecorder()
		h.ServeHTTP(w, r)
		return w
	}
	expect := func(w *httptest.ResponseRecorder, status int) {
		t.Helper()
		if w.Code != status {
			t.Fatalf("HTTP %d expected %d: %s", w.Code, status, w.Body.String())
		}
	}
	register := func(name string) domain.Tokens {
		w := call("POST", "/auth/register", fmt.Sprintf(`{"username":%q,"email":%q,"password":"test-password-2026"}`, name, name+"@example.org"), "", "")
		expect(w, 201)
		var v domain.Tokens
		if e := json.Unmarshal(w.Body.Bytes(), &v); e != nil {
			t.Fatal(e)
		}
		return v
	}
	owner := register("owner")
	other := register("other")
	oa, e := s.Authenticate(ctx, owner.Access)
	if e != nil {
		t.Fatal(e)
	}
	ba, e := s.Authenticate(ctx, other.Access)
	if e != nil {
		t.Fatal(e)
	}
	w := call("GET", "/me", "", owner.Access, "")
	expect(w, 200)
	if !strings.Contains(w.Body.String(), "owner@example.org") {
		t.Fatal("email missing")
	}
	expect(call("GET", "/me", "", "", ""), 401)
	expect(call("PATCH", "/me", `{"bio":"Автор"}`, owner.Access, ""), 200)
	input := `{"title":"Test","slug":"test-post","content":"# Hello","tags":["go"]}`
	w = call("POST", "/articles", input, owner.Access, "")
	expect(w, 201)
	var article domain.Article
	json.Unmarshal(w.Body.Bytes(), &article)
	path := fmt.Sprintf("/articles/%d", article.ID)
	etag := w.Header().Get("ETag")
	expect(call("GET", path, "", other.Access, ""), 404)
	expect(call("GET", path, "", "", ""), 404)
	expect(call("PATCH", path, `{"title":"stolen"}`, other.Access, etag), 403)
	expect(call("DELETE", path, "", other.Access, etag), 403)
	expect(call("GET", path+"/statistics", "", other.Access, ""), 403)
	w = call("GET", fmt.Sprintf("/users/%d", oa.ID), "", other.Access, "")
	expect(w, 200)
	for _, private := range []string{"email", "password", "role", "draft_articles"} {
		if strings.Contains(w.Body.String(), `"`+private+`"`) {
			t.Fatal("private profile leaked", private)
		}
	}
	w = call("GET", fmt.Sprintf("/users/%d/articles", oa.ID), "", "", "")
	expect(w, 200)
	var page domain.Page[domain.Article]
	json.Unmarshal(w.Body.Bytes(), &page)
	if page.Total != 0 {
		t.Fatal("draft exposed")
	}
	w = call("GET", "/me/articles?status=draft", "", owner.Access, "")
	expect(w, 200)
	json.Unmarshal(w.Body.Bytes(), &page)
	if page.Total != 1 || page.Items[0].Status != "draft" {
		t.Fatal("draft missing")
	}
	expect(call("POST", path+"/views", "", other.Access, ""), 404)
	w = call("PATCH", path, `{"status":"published"}`, owner.Access, etag)
	expect(w, 200)
	etag = w.Header().Get("ETag")
	json.Unmarshal(w.Body.Bytes(), &article)
	if article.CanonicalURL == nil {
		t.Fatal("public link missing")
	}
	expect(call("PATCH", path, `{"title":"stale"}`, owner.Access, `"1"`), 412)
	expect(call("PATCH", path, `{"title":"new"}`, owner.Access, ""), 428)
	w = call("GET", fmt.Sprintf("/users/%d/articles", oa.ID), "", "", "")
	expect(w, 200)
	json.Unmarshal(w.Body.Bytes(), &page)
	if page.Total != 1 || page.Items[0].Status != "published" {
		t.Fatal("published list wrong")
	}
	expect(call("GET", "/articles-by-slug/test-post", "", "", ""), 200)
	for i := 0; i < 3; i++ {
		expect(call("GET", path, "", "", ""), 200)
	}
	v, e := s.Article(ctx, ba, article.ID)
	if e != nil || v.Views != 0 {
		t.Fatal("GET counted a view", e)
	}
	w = call("POST", path+"/views", "", owner.Access, "")
	expect(w, 200)
	if !strings.Contains(w.Body.String(), "false") {
		t.Fatal("author counted")
	}
	// Repository concurrency tests bypass HTTP rate limits while exercising real SQL transactions.
	parallel := func(fn func() error) {
		t.Helper()
		var wg sync.WaitGroup
		errs := make(chan error, 16)
		for i := 0; i < 16; i++ {
			wg.Add(1)
			go func() { defer wg.Done(); errs <- fn() }()
		}
		wg.Wait()
		close(errs)
		for e := range errs {
			if e != nil {
				t.Fatal(e)
			}
		}
	}
	parallel(func() error { _, _, e := s.Like(ctx, ba, article.ID, true); return e })
	v, e = s.Article(ctx, ba, article.ID)
	if e != nil || v.Likes != 1 || !v.Liked {
		t.Fatal("concurrent likes wrong", v.Likes, e)
	}
	parallel(func() error { _, _, e := s.Like(ctx, ba, article.ID, false); return e })
	v, _ = s.Article(ctx, ba, article.ID)
	if v.Likes != 0 || v.Liked {
		t.Fatal("unlike wrong")
	}
	parallel(func() error { _, e := s.View(ctx, ba, article.ID, "user:"+fmt.Sprint(ba.ID)); return e })
	v, _ = s.Article(ctx, ba, article.ID)
	if v.Views != 1 {
		t.Fatal("concurrent views overcounted", v.Views)
	}
	counted, e := s.View(ctx, ba, article.ID, "user:"+fmt.Sprint(ba.ID))
	if e != nil || counted {
		t.Fatal("duplicate counted", e)
	}
	if _, e = pool.Exec(ctx, "UPDATE view_dedup SET last_seen=clock_timestamp()-interval '31 minutes' WHERE article_id=$1", article.ID); e != nil {
		t.Fatal(e)
	}
	counted, e = s.View(ctx, ba, article.ID, "user:"+fmt.Sprint(ba.ID))
	if e != nil || !counted {
		t.Fatal("next interval not counted", e)
	}
	// A guest receives a signed secure cookie and subsequent calls deduplicate.
	wr := call("POST", path+"/views", "", "", "")
	expect(wr, 200)
	cookies := wr.Result().Cookies()
	if len(cookies) != 1 || !cookies[0].HttpOnly || !cookies[0].Secure || cookies[0].SameSite != http.SameSiteLaxMode {
		t.Fatal("guest cookie unsafe")
	}
	r := httptest.NewRequest("POST", "/api/v1"+path+"/views", nil)
	r.AddCookie(cookies[0])
	r.RemoteAddr = "127.0.0.1:1234"
	wr = httptest.NewRecorder()
	h.ServeHTTP(wr, r)
	expect(wr, 200)
	if !strings.Contains(wr.Body.String(), "false") {
		t.Fatal("guest duplicate counted")
	}
	w = call("POST", path+"/comments", `{"content":"Root"}`, other.Access, "")
	expect(w, 201)
	var comment domain.Comment
	json.Unmarshal(w.Body.Bytes(), &comment)
	w = call("POST", path+"/comments", fmt.Sprintf(`{"content":"Reply","parent_id":%d}`, comment.ID), owner.Access, "")
	expect(w, 201)
	var reply domain.Comment
	json.Unmarshal(w.Body.Bytes(), &reply)
	expect(call("POST", path+"/comments", fmt.Sprintf(`{"content":"Too deep","parent_id":%d}`, reply.ID), owner.Access, ""), 422)
	expect(call("PATCH", fmt.Sprintf("/comments/%d", comment.ID), `{"content":"stolen"}`, owner.Access, ""), 403)
	expect(call("DELETE", fmt.Sprintf("/comments/%d", comment.ID), "", other.Access, ""), 204)
	w = call("GET", path+"/comments", "", "", "")
	expect(w, 200)
	var cp domain.Page[domain.Comment]
	json.Unmarshal(w.Body.Bytes(), &cp)
	if cp.Total != 2 || !cp.Items[0].Deleted || cp.Items[0].Content != "" || cp.Items[1].Deleted {
		t.Fatal("tombstone/replies wrong")
	}
	// Seed historical dates; inclusive period boundaries and zero filling are tested.
	today := time.Now().UTC().Format(time.DateOnly)
	from := time.Now().UTC().AddDate(0, 0, -2).Format(time.DateOnly)
	if _, e = pool.Exec(ctx, "INSERT INTO article_daily_views(article_id,day,views) VALUES($1,$2,4)", article.ID, from); e != nil {
		t.Fatal(e)
	}
	p, e := s.Period(from, today)
	if e != nil {
		t.Fatal(e)
	}
	st, e := s.ArticleStatistics(ctx, oa, article.ID, p)
	if e != nil || len(st.Daily) != 3 || st.Daily[0].Views != 4 || st.Daily[1].Views != 0 || st.ViewsPeriod != 7 || st.Comments != 1 {
		t.Fatalf("stats wrong: %+v %v", st, e)
	}
	w = call("GET", "/me/statistics?date_from="+from+"&date_to="+today, "", owner.Access, "")
	expect(w, 200)
	var summary domain.Statistics
	json.Unmarshal(w.Body.Bytes(), &summary)
	if summary.ViewsPeriod != 7 || len(summary.Top) != 1 {
		t.Fatal("summary wrong")
	}
	empty, _ := s.Period(from, from)
	if _, e = pool.Exec(ctx, "DELETE FROM article_daily_views WHERE article_id=$1 AND day=$2", article.ID, from); e != nil {
		t.Fatal(e)
	}
	st, e = s.ArticleStatistics(ctx, oa, article.ID, empty)
	if e != nil || st.ViewsPeriod != 0 || len(st.Daily) != 1 || st.Daily[0].Views != 0 {
		t.Fatal("empty period wrong", e)
	}
	expect(call("GET", "/me/statistics?date_from=2026-02-30", "", owner.Access, ""), 422)
	w = call("PATCH", path, `{"status":"draft"}`, owner.Access, etag)
	expect(w, 200)
	etag = w.Header().Get("ETag")
	prof, e := s.PublicProfile(ctx, oa.ID)
	if e != nil || prof.Statistics.Published != 0 || prof.Statistics.Views != 0 {
		t.Fatal("unpublished public statistics wrong")
	}
	st, e = s.ArticleStatistics(ctx, oa, article.ID, p)
	if e != nil || st.ViewsAll != 3 {
		t.Fatal("unpublication lost private views")
	}
	expect(call("PUT", path+"/like", "", other.Access, ""), 404)
	expect(call("DELETE", path, "", owner.Access, etag), 204)
	summary, e = s.Statistics(ctx, oa, p)
	if e != nil || summary.Views != 0 || summary.ViewsPeriod != 0 || summary.Drafts != 0 {
		t.Fatal("deletion stats wrong", e)
	}
	var remnants int64
	if e = pool.QueryRow(ctx, "SELECT (SELECT count(*) FROM view_dedup)+(SELECT count(*) FROM comments)+(SELECT count(*) FROM article_daily_views)").Scan(&remnants); e != nil || remnants != 0 {
		t.Fatal("cascade deletion wrong", e)
	}

	// Two editors race with the same ETag: exactly one wins.
	fresh, e := s.CreateArticle(ctx, oa, domain.ArticleInput{Title: "Race", Slug: "race", Content: "Content", Status: "published"})
	if e != nil {
		t.Fatal(e)
	}
	title := "Edited"
	var wg sync.WaitGroup
	outcomes := make(chan error, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, e := s.UpdateArticle(ctx, oa, fresh.ID, fresh.Version, domain.ArticlePatch{Title: &title})
			outcomes <- e
		}()
	}
	wg.Wait()
	close(outcomes)
	wins, conflicts := 0, 0
	for e := range outcomes {
		if e == nil {
			wins++
		} else if de, ok := e.(*domain.Error); ok && de.Status == 412 {
			conflicts++
		} else {
			t.Fatal(e)
		}
	}
	if wins != 1 || conflicts != 1 {
		t.Fatalf("concurrent updates: %d wins, %d conflicts", wins, conflicts)
	}
	// Administrator moderation and preview exclusion.
	if _, e = pool.Exec(ctx, "UPDATE users SET role='admin' WHERE id=$1", ba.ID); e != nil {
		t.Fatal(e)
	}
	admin, e := s.Authenticate(ctx, other.Access)
	if e != nil {
		t.Fatal(e)
	}
	counted, e = s.View(ctx, admin, fresh.ID, "user:"+fmt.Sprint(admin.ID))
	if e != nil || counted {
		t.Fatal("admin preview counted", e)
	}
	if _, e = s.ArticleStatistics(ctx, admin, fresh.ID, p); e != nil {
		t.Fatal("admin statistics forbidden", e)
	}
	draft := "draft"
	updated, e := s.UpdateArticle(ctx, admin, fresh.ID, 2, domain.ArticlePatch{Status: &draft})
	if e != nil {
		t.Fatal("admin edit forbidden", e)
	}
	if _, e = s.Article(ctx, admin, fresh.ID); e != nil {
		t.Fatal("admin draft forbidden", e)
	}
	if e = s.DeleteArticle(ctx, admin, fresh.ID, updated.Version); e != nil {
		t.Fatal("admin deletion forbidden", e)
	}
	// Rotation, replay detection and logout invalidate access credentials immediately.
	w = call("POST", "/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, other.Refresh), "", "")
	expect(w, 200)
	var rotated domain.Tokens
	json.Unmarshal(w.Body.Bytes(), &rotated)
	expect(call("GET", "/me", "", rotated.Access, ""), 200)
	expect(call("POST", "/auth/refresh", fmt.Sprintf(`{"refresh_token":%q}`, other.Refresh), "", ""), 401)
	expect(call("GET", "/me", "", rotated.Access, ""), 401)
	expect(call("POST", "/auth/logout", fmt.Sprintf(`{"refresh_token":%q}`, owner.Refresh), "", ""), 204)
	expect(call("GET", "/me", "", owner.Access, ""), 401)
}
