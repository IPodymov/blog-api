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
	"io"
	"log/slog"
	"net/http/httptest"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPlatformAdmin(t *testing.T) {
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
	pool, e := pgxpool.NewWithConfig(ctx, pc)
	if e != nil {
		t.Fatal(e)
	}
	defer pool.Close()
	if e = migrations.Apply(ctx, pool); e != nil {
		t.Fatal(e)
	}
	s := &service.Service{Repo: &repository.Store{DB: pool}, PublicURL: "https://blog.example", VisitorSecret: strings.Repeat("s", 32), AccessTTL: 15 * time.Minute, RefreshTTL: 24 * time.Hour, ViewInterval: 30 * time.Minute, MaxPeriod: 366}
	h := New(s, config.Config{}, slog.New(slog.NewTextHandler(io.Discard, nil)))
	// Each request gets its own address so the per-address auth rate limit does not interfere.
	requests := 0
	call := func(siteKey, method, path, body, token, etag string, status int) *httptest.ResponseRecorder {
		t.Helper()
		r := httptest.NewRequest(method, path, strings.NewReader(body))
		requests++
		r.RemoteAddr = fmt.Sprintf("10.0.%d.%d:1234", requests/250, requests%250+1)
		if siteKey != "" {
			r.Header.Set("X-Site-Key", siteKey)
		}
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
		if w.Code != status {
			t.Fatalf("%s %s: HTTP %d expected %d: %s", method, path, w.Code, status, w.Body.String())
		}
		return w
	}
	read := func(w *httptest.ResponseRecorder, v any) {
		t.Helper()
		if e := json.Unmarshal(w.Body.Bytes(), v); e != nil {
			t.Fatal(e)
		}
	}
	const adm = "/api/admin/v1"

	if _, e = s.CreateAdmin(ctx, "root@example.org", "admin-password-2026"); e != nil {
		t.Fatal(e)
	}
	if _, e = s.CreateAdmin(ctx, "ROOT@example.org", "admin-password-2026"); e == nil {
		t.Fatal("duplicate admin email accepted")
	}
	call("", "GET", adm+"/sites", "", "", "", 401)
	call("", "POST", adm+"/auth/login", `{"email":"root@example.org","password":"wrong-password"}`, "", "", 401)
	var at domain.AdminToken
	read(call("", "POST", adm+"/auth/login", `{"email":"root@example.org","password":"admin-password-2026"}`, "", "", 200), &at)

	// A site user, even a site admin, has no access to the platform API; the admin token is not a site token.
	var su domain.Tokens
	read(call("default", "POST", "/api/v1/auth/register", `{"username":"siteuser","email":"siteuser@example.org","password":"test-password-2026"}`, "", "", 201), &su)
	if _, e = pool.Exec(ctx, "UPDATE users SET role='admin' WHERE username='siteuser'"); e != nil {
		t.Fatal(e)
	}
	call("", "GET", adm+"/sites", "", su.Access, "", 401)
	call("default", "GET", "/api/v1/me", "", at.Access, "", 401)

	call("", "POST", adm+"/sites", `{"key":"Bad Key"}`, at.Access, "", 422)
	call("", "POST", adm+"/sites", `{"key":"med","name":"Мед","public_url":"https://med.example/"}`, at.Access, "", 201)
	call("", "POST", adm+"/sites", `{"key":"med"}`, at.Access, "", 409)
	call("", "PATCH", adm+"/sites/med", `{"public_url":"ftp://x"}`, at.Access, "", 422)
	call("", "PATCH", adm+"/sites/med", `{"name":"Медицинский помощник"}`, at.Access, "", 200)
	call("", "GET", adm+"/sites/missing", "", at.Access, "", 404)
	var sites domain.Page[domain.SiteInfo]
	read(call("", "GET", adm+"/sites", "", at.Access, "", 200), &sites)
	if sites.Total != 2 {
		t.Fatal("sites", sites.Total)
	}

	// Users created by the admin get the requested role and can work on their site right away.
	call("", "POST", adm+"/sites/med/users", `{"username":"editor","email":"editor@example.org","password":"short","role":"author"}`, at.Access, "", 422)
	call("", "POST", adm+"/sites/med/users", `{"username":"editor","email":"editor@example.org","password":"editor-password-2026","role":"boss"}`, at.Access, "", 422)
	var editor domain.OwnUser
	read(call("", "POST", adm+"/sites/med/users", `{"username":"editor","email":"editor@example.org","password":"editor-password-2026","role":"author","display_name":"Редактор"}`, at.Access, "", 201), &editor)
	if editor.Role != "author" {
		t.Fatal("role", editor.Role)
	}
	var et domain.Tokens
	read(call("med", "POST", "/api/v1/auth/login", `{"email":"editor@example.org","password":"editor-password-2026"}`, "", "", 200), &et)
	call("default", "POST", "/api/v1/auth/login", `{"email":"editor@example.org","password":"editor-password-2026"}`, "", "", 401)
	call("med", "POST", "/api/v1/articles", `{"title":"Own","slug":"own","content":"x"}`, et.Access, "", 201)

	var users domain.Page[domain.OwnUser]
	read(call("", "GET", adm+"/sites/med/users?role=author&q=edit", "", at.Access, "", 200), &users)
	if users.Total != 1 || users.Items[0].Email != "editor@example.org" {
		t.Fatal("user search", users)
	}
	call("", "GET", adm+"/sites/default/users/"+fmt.Sprint(editor.ID), "", at.Access, "", 404)

	// Articles on behalf of a site user, with drafts visible to the admin only.
	call("", "POST", adm+"/sites/default/articles", `{"author_id":`+fmt.Sprint(editor.ID)+`,"title":"X","slug":"x","content":"x"}`, at.Access, "", 422)
	w := call("", "POST", adm+"/sites/med/articles", `{"author_id":`+fmt.Sprint(editor.ID)+`,"title":"От админа","slug":"from-admin","content":"Текст","tags":["med"]}`, at.Access, "", 201)
	var art domain.Article
	read(w, &art)
	path := fmt.Sprintf("%s/sites/med/articles/%d", adm, art.ID)
	call("med", "GET", fmt.Sprintf("/api/v1/articles/%d", art.ID), "", "", "", 404)
	var list domain.Page[domain.Article]
	read(call("", "GET", adm+"/sites/med/articles?status=draft", "", at.Access, "", 200), &list)
	if list.Total != 2 {
		t.Fatal("admin drafts", list.Total)
	}
	call("", "PATCH", path, `{"status":"published"}`, at.Access, "", 428)
	w = call("", "PATCH", path, `{"status":"published"}`, at.Access, w.Header().Get("ETag"), 200)
	read(w, &art)
	if art.CanonicalURL == nil || *art.CanonicalURL != "https://med.example/articles/from-admin" {
		t.Fatal("canonical", art.CanonicalURL)
	}
	call("med", "GET", "/api/v1/articles-by-slug/from-admin", "", "", "", 200)
	call("", "DELETE", path, "", at.Access, `"1"`, 412)
	call("", "DELETE", path, "", at.Access, w.Header().Get("ETag"), 204)

	// A new password revokes the user's sessions; a role change applies immediately.
	call("", "PATCH", fmt.Sprintf("%s/sites/med/users/%d", adm, editor.ID), `{"role":"user"}`, at.Access, "", 200)
	call("med", "POST", "/api/v1/articles", `{"title":"Nope","slug":"nope","content":"x"}`, et.Access, "", 403)
	call("", "PATCH", fmt.Sprintf("%s/sites/med/users/%d", adm, editor.ID), `{"password":"another-password-2026","bio":"Врач"}`, at.Access, "", 200)
	call("med", "GET", "/api/v1/me", "", et.Access, "", 401)

	// Invites: preset role, optional email lock, single use, revocable.
	call("", "POST", adm+"/sites/med/invites", `{"role":"author","expires_in_hours":0.5}`, at.Access, "", 400)
	call("", "POST", adm+"/sites/med/invites", `{"role":"author","expires_in_hours":1000}`, at.Access, "", 422)
	var inv domain.Invite
	read(call("", "POST", adm+"/sites/med/invites", `{"role":"author"}`, at.Access, "", 201), &inv)
	if len(inv.Token) != 43 || inv.Role != "author" {
		t.Fatal("invite", inv)
	}
	call("default", "GET", "/api/v1/invites/"+inv.Token, "", "", "", 404)
	call("med", "GET", "/api/v1/invites/"+inv.Token, "", "", "", 200)
	call("default", "POST", "/api/v1/auth/register", `{"username":"writer","email":"writer@example.org","password":"writer-password-2026","invite_token":"`+inv.Token+`"}`, "", "", 422)
	var wt domain.Tokens
	read(call("med", "POST", "/api/v1/auth/register", `{"username":"writer","email":"writer@example.org","password":"writer-password-2026","invite_token":"`+inv.Token+`"}`, "", "", 201), &wt)
	call("med", "POST", "/api/v1/articles", `{"title":"Invited","slug":"invited","content":"x"}`, wt.Access, "", 201)
	call("med", "POST", "/api/v1/auth/register", `{"username":"writer2","email":"writer2@example.org","password":"writer-password-2026","invite_token":"`+inv.Token+`"}`, "", "", 422)
	call("med", "GET", "/api/v1/invites/"+inv.Token, "", "", "", 404)
	var locked domain.Invite
	read(call("", "POST", adm+"/sites/med/invites", `{"role":"admin","email":"Boss@Example.org"}`, at.Access, "", 201), &locked)
	call("med", "POST", "/api/v1/auth/register", `{"username":"intruder","email":"intruder@example.org","password":"writer-password-2026","invite_token":"`+locked.Token+`"}`, "", "", 422)
	var invites domain.Page[domain.Invite]
	read(call("", "GET", adm+"/sites/med/invites", "", at.Access, "", 200), &invites)
	if invites.Total != 2 || invites.Items[1].UsedAt == nil || invites.Items[0].Token != "" {
		t.Fatal("invites list", invites)
	}
	call("", "DELETE", fmt.Sprintf("%s/sites/med/invites/%d", adm, locked.ID), "", at.Access, "", 204)
	call("med", "POST", "/api/v1/auth/register", `{"username":"boss","email":"boss@example.org","password":"writer-password-2026","invite_token":"`+locked.Token+`"}`, "", "", 422)
	call("med", "POST", "/api/v1/auth/register", `{"username":"plain","email":"plain@example.org","password":"plain-password-2026"}`, "", "", 201)
	var plain domain.Page[domain.OwnUser]
	read(call("", "GET", adm+"/sites/med/users?q=plain", "", at.Access, "", 200), &plain)
	if plain.Items[0].Role != "user" {
		t.Fatal("self-registration must stay user")
	}

	// Deleting a user removes their content; the admin logs out.
	call("", "DELETE", fmt.Sprintf("%s/sites/med/users/%d", adm, editor.ID), "", at.Access, "", 204)
	call("", "DELETE", fmt.Sprintf("%s/sites/med/users/%d", adm, editor.ID), "", at.Access, "", 404)
	call("med", "GET", "/api/v1/articles-by-slug/own", "", "", "", 404)
	call("", "POST", adm+"/auth/logout", "", at.Access, "", 204)
	call("", "GET", adm+"/me", "", at.Access, "", 401)
	if e = s.SetAdminPassword(ctx, "root@example.org", "rotated-password-2026"); e != nil {
		t.Fatal(e)
	}
	call("", "POST", adm+"/auth/login", `{"email":"root@example.org","password":"rotated-password-2026"}`, "", "", 200)
}
