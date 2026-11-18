// Command seed fills a site with demo content through the public HTTP API and doubles as a smoke test.
//
//	API_URL=https://blog-api-zeta-green.vercel.app ADMIN_EMAIL=... ADMIN_PASSWORD=... go run ./cmd/seed -site default
//
// It is idempotent: demo accounts get fresh passwords, existing articles are kept.
// All demo articles carry the "demo" tag.
package main

import (
	"blog-api/internal/service"
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

type client struct {
	base, site string
	http       *http.Client
}

// call sends a JSON request and decodes the response into out; it fails unless the status is one of ok.
func (c client) call(method, path, token, etag string, body, out any, ok ...int) (int, http.Header) {
	var r io.Reader
	if body != nil {
		b, _ := json.Marshal(body)
		r = bytes.NewReader(b)
	}
	req, e := http.NewRequest(method, c.base+path, r)
	if e != nil {
		log.Fatal(e)
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if strings.HasPrefix(path, "/api/v1/") {
		req.Header.Set("X-Site-Key", c.site)
	}
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	res, e := c.http.Do(req)
	if e != nil {
		log.Fatal(method, " ", path, ": ", e)
	}
	defer res.Body.Close()
	raw, _ := io.ReadAll(res.Body)
	for _, s := range ok {
		if res.StatusCode == s {
			if out != nil && len(raw) > 0 {
				if e := json.Unmarshal(raw, out); e != nil {
					log.Fatal(method, " ", path, ": ", e)
				}
			}
			return res.StatusCode, res.Header
		}
	}
	log.Fatalf("%s %s: HTTP %d %s", method, path, res.StatusCode, raw)
	return 0, nil
}

type token struct {
	Access string `json:"access_token"`
}
type user struct {
	ID       int64  `json:"id"`
	Username string `json:"username"`
}
type page[T any] struct {
	Items []T `json:"items"`
	Total int `json:"total"`
}
type article struct {
	ID     int64  `json:"id"`
	Slug   string `json:"slug"`
	Status string `json:"status"`
}

var posts = []struct{ title, slug, description, content, status string }{
	{"Добро пожаловать в блог", "welcome", "Первый пост демонстрационного наполнения.", "# Добро пожаловать\n\nЭто демонстрационная статья. Она показывает, что API принимает, хранит и отдаёт публикации.", "published"},
	{"Как устроен этот API", "how-the-api-works", "Коротко об архитектуре.", "## Архитектура\n\nGo-монолит, PostgreSQL, изолированные сайты по заголовку `X-Site-Key`, Bearer-токены.", "published"},
	{"Пять советов по написанию статей", "five-writing-tips", "Пишите понятно и коротко.", "1. Начинайте с главного.\n2. Один абзац — одна мысль.\n3. Используйте заголовки.\n4. Добавляйте примеры.\n5. Перечитывайте перед публикацией.", "published"},
	{"Markdown: шпаргалка", "markdown-cheatsheet", "Основные элементы разметки.", "**Жирный**, *курсив*, `код`.\n\n- список\n- ещё пункт\n\n> Цитата", "published"},
	{"Здоровый сон: базовые правила", "healthy-sleep-basics", "Демонстрационный текст на медицинскую тему.", "Ложитесь и вставайте в одно время, проветривайте спальню, ограничьте экраны за час до сна. Текст носит демонстрационный характер и не является медицинской рекомендацией.", "published"},
	{"Черновик: план следующих статей", "next-posts-draft", "Черновик не виден публично.", "- Статистика просмотров\n- Комментарии и модерация", "draft"},
}

func main() {
	site := flag.String("site", "default", "site key to fill")
	flag.Parse()
	base := strings.TrimRight(os.Getenv("API_URL"), "/")
	if base == "" {
		base = "http://localhost:8080"
	}
	email, password := os.Getenv("ADMIN_EMAIL"), os.Getenv("ADMIN_PASSWORD")
	if email == "" || password == "" {
		log.Fatal("ADMIN_EMAIL and ADMIN_PASSWORD are required")
	}
	c := client{base: base, site: *site, http: &http.Client{Timeout: 30 * time.Second}}
	admin := "/api/admin/v1/sites/" + url.PathEscape(*site)

	var at token
	c.call("POST", "/api/admin/v1/auth/login", "", "", map[string]string{"email": email, "password": password}, &at, 200)
	c.call("GET", admin, at.Access, "", nil, nil, 200)
	fmt.Println("admin login: ok, site:", *site)

	// ensure creates the demo account or resets its password, then logs in on the site.
	ensure := func(username, role string) (user, string) {
		pw := service.Random()[:24]
		var u user
		st, _ := c.call("POST", admin+"/users", at.Access, "", map[string]string{"username": username, "email": username + "@demo.example", "password": pw, "role": role, "display_name": "Демо " + role}, &u, 201, 409)
		if st == 409 {
			var found page[user]
			c.call("GET", admin+"/users?q="+username, at.Access, "", nil, &found, 200)
			for _, x := range found.Items {
				if x.Username == username {
					u = x
				}
			}
			if u.ID == 0 {
				log.Fatalf("%s: email taken by another account", username)
			}
			c.call("PATCH", fmt.Sprintf("%s/users/%d", admin, u.ID), at.Access, "", map[string]string{"password": pw, "role": role}, &u, 200)
		}
		var t token
		c.call("POST", "/api/v1/auth/login", "", "", map[string]string{"email": username + "@demo.example", "password": pw}, &t, 200)
		fmt.Printf("user %s (id %d, %s): ok\n", username, u.ID, role)
		return u, t.Access
	}
	author, authorToken := ensure("demo_author", "author")
	_, readerToken := ensure("demo_reader", "user")

	// Only articles created by this run get a comment, so repeated runs do not pile comments up.
	created := map[string]bool{}
	for _, p := range posts {
		st, _ := c.call("POST", "/api/v1/articles", authorToken, "", map[string]any{"title": p.title, "slug": p.slug, "description": p.description, "content": p.content, "status": p.status, "tags": []string{"demo"}}, nil, 201, 409)
		if st == 201 {
			created[p.slug] = true
		}
	}
	fmt.Printf("articles: %d created, %d already existed\n", len(created), len(posts)-len(created))

	var mine page[article]
	c.call("GET", "/api/v1/me/articles?tag=demo&page_size=100", authorToken, "", nil, &mine, 200)
	for _, a := range mine.Items {
		path := fmt.Sprintf("/api/v1/articles/%d", a.ID)
		if a.Status != "published" {
			c.call("GET", path, "", "", nil, nil, 404)
			continue
		}
		c.call("POST", path+"/views", readerToken, "", nil, nil, 200)
		c.call("PUT", path+"/like", readerToken, "", nil, nil, 200)
		if created[a.Slug] {
			c.call("POST", path+"/comments", readerToken, "", map[string]string{"content": "Спасибо, полезно!"}, nil, 201)
		}
		c.call("GET", "/api/v1/articles-by-slug/"+a.Slug, "", "", nil, nil, 200)
	}

	var public page[article]
	c.call("GET", fmt.Sprintf("/api/v1/users/%d/articles", author.ID), "", "", nil, &public, 200)
	var all page[article]
	c.call("GET", admin+"/articles?tag=demo", at.Access, "", nil, &all, 200)
	var tags page[string]
	c.call("GET", "/api/v1/tags", "", "", nil, &tags, 200)
	c.call("GET", "/api/v1/me/statistics", authorToken, "", nil, nil, 200)
	c.call("POST", "/api/admin/v1/auth/logout", at.Access, "", nil, nil, 204)
	fmt.Printf("public demo articles: %d, all incl. drafts (admin): %d, site tags: %d\n", public.Total, all.Total, tags.Total)
	fmt.Println("smoke test passed")
}
