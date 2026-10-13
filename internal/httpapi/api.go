package httpapi

import (
	"blog-api/internal/config"
	"blog-api/internal/domain"
	"blog-api/internal/service"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"mime"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/go-chi/chi/v5"
)

type ctxKey int

const (
	actorKey ctxKey = iota
	requestKey
)

type API struct {
	Service   *service.Service
	Config    config.Config
	Log       *slog.Logger
	mu        sync.Mutex
	buckets   map[string]bucket
	lastSweep time.Time
}
type bucket struct {
	start time.Time
	n     int
}

func New(s *service.Service, c config.Config, l *slog.Logger) http.Handler {
	a := &API{Service: s, Config: c, Log: l, buckets: map[string]bucket{}}
	r := chi.NewRouter()
	r.Use(a.middleware)
	r.NotFound(func(w http.ResponseWriter, r *http.Request) {
		a.respond(w, r, nil, domain.Fail(404, "not_found", "Маршрут не найден"), 0)
	})
	r.MethodNotAllowed(func(w http.ResponseWriter, r *http.Request) {
		a.respond(w, r, nil, domain.Fail(405, "method_not_allowed", "Метод не поддерживается"), 0)
	})
	r.Get("/health", func(w http.ResponseWriter, r *http.Request) {
		a.respond(w, r, map[string]string{"status": "ok"}, nil, 200)
	})
	r.Get("/ready", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(r.Context(), 2*time.Second)
		defer cancel()
		if e := s.Repo.DB.Ping(ctx); e != nil {
			a.respond(w, r, nil, domain.Fail(503, "not_ready", "База данных недоступна"), 0)
			return
		}
		a.respond(w, r, map[string]string{"status": "ready"}, nil, 200)
	})
	r.Get("/openapi.yaml", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/yaml")
		http.ServeFile(w, r, "openapi.yaml")
	})
	r.Get("/docs", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/html; charset=utf-8")
		io.WriteString(w, `<!doctype html><html lang="ru"><head><meta charset="utf-8"><title>Blog API</title><link rel="stylesheet" href="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.29.5/swagger-ui.css"></head><body><div id="swagger"></div><script src="https://cdn.jsdelivr.net/npm/swagger-ui-dist@5.29.5/swagger-ui-bundle.js"></script><script>SwaggerUIBundle({url:'/openapi.yaml',dom_id:'#swagger',persistAuthorization:false})</script></body></html>`)
	})
	r.Route("/api/v1", func(r chi.Router) {
		r.Post("/auth/register", a.register)
		r.Post("/auth/login", a.login)
		r.Post("/auth/refresh", a.refresh)
		r.Post("/auth/logout", a.logout)
		r.Get("/users/{user_id}", a.profile)
		r.Get("/users/{user_id}/articles", a.userArticles)
		r.Get("/me", a.me)
		r.Patch("/me", a.patchMe)
		r.Get("/me/articles", a.myArticles)
		r.Get("/me/statistics", a.statistics)
		r.Get("/me/statistics/articles", a.statisticsArticles)
		r.Get("/articles", a.articles)
		r.Post("/articles", a.createArticle)
		r.Get("/articles-by-slug/{slug}", a.slug)
		r.Get("/articles/{article_id}", a.article)
		r.Patch("/articles/{article_id}", a.patchArticle)
		r.Delete("/articles/{article_id}", a.deleteArticle)
		r.Post("/articles/{article_id}/views", a.view)
		r.Get("/articles/{article_id}/statistics", a.articleStatistics)
		r.Get("/articles/{article_id}/comments", a.comments)
		r.Post("/articles/{article_id}/comments", a.createComment)
		r.Patch("/comments/{comment_id}", a.patchComment)
		r.Delete("/comments/{comment_id}", a.deleteComment)
		r.Put("/articles/{article_id}/like", a.like)
		r.Delete("/articles/{article_id}/like", a.unlike)
		r.Get("/tags", a.tags)
	})
	return r
}
func actor(r *http.Request) domain.Actor {
	a, _ := r.Context().Value(actorKey).(domain.Actor)
	return a
}
func (a *API) respond(w http.ResponseWriter, r *http.Request, v any, e error, status int) {
	if e != nil {
		de := &domain.Error{Status: 500, Code: "internal_error", Message: "Внутренняя ошибка сервера"}
		if !errors.As(e, &de) {
			a.Log.Error("request failed", "request_id", r.Context().Value(requestKey), "error_type", fmt.Sprintf("%T", e))
		}
		status = de.Status
		v = map[string]any{"error": map[string]any{"code": de.Code, "message": de.Message, "details": de.Details, "request_id": r.Context().Value(requestKey)}}
	}
	if status == 0 {
		status = 200
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	if status != 204 {
		json.NewEncoder(w).Encode(v)
	}
}
func decode(r *http.Request, v any, patch bool) error {
	media, _, mediaErr := mime.ParseMediaType(r.Header.Get("Content-Type"))
	if mediaErr != nil || media != "application/json" {
		return domain.Fail(415, "unsupported_media_type", "Требуется application/json")
	}
	b, e := io.ReadAll(r.Body)
	if e != nil {
		var max *http.MaxBytesError
		if errors.As(e, &max) {
			return domain.Fail(413, "body_too_large", "Тело запроса слишком большое")
		}
		return domain.Fail(400, "invalid_json", "Не удалось прочитать JSON")
	}
	var obj map[string]json.RawMessage
	if json.Unmarshal(b, &obj) != nil || obj == nil {
		return domain.Fail(400, "invalid_json", "Ожидается JSON объект")
	}
	for k, value := range obj {
		if string(value) == "null" && k != "parent_id" {
			return domain.Invalid(k, "null не допускается")
		}
	}
	if patch {
		if len(obj) == 0 {
			return domain.Invalid("body", "Укажите хотя бы одно поле")
		}
		for k, v := range obj {
			if string(v) == "null" {
				return domain.Invalid(k, "null не допускается; для очистки используйте пустое значение")
			}
		}
	}
	d := json.NewDecoder(strings.NewReader(string(b)))
	d.DisallowUnknownFields()
	if e = d.Decode(v); e != nil {
		return domain.Fail(400, "invalid_json", "Некорректный JSON или неизвестное поле")
	}
	return nil
}
func id(r *http.Request, name string) (int64, error) {
	v, e := strconv.ParseInt(chi.URLParam(r, name), 10, 64)
	if e != nil || v <= 0 {
		return 0, domain.Invalid(name, "Ожидается положительный int64")
	}
	return v, nil
}
func version(r *http.Request) (int64, error) {
	v := r.Header.Get("If-Match")
	if v == "" {
		return 0, domain.Fail(428, "version_required", "Укажите If-Match с ETag статьи")
	}
	if len(v) < 3 || v[0] != '"' || v[len(v)-1] != '"' {
		return 0, domain.Invalid("If-Match", "Формат: \"1\"")
	}
	n, e := strconv.ParseInt(v[1:len(v)-1], 10, 64)
	if e != nil || n < 1 {
		return 0, domain.Invalid("If-Match", "Некорректная версия")
	}
	return n, nil
}
func filter(r *http.Request, stats bool) (domain.Filter, error) {
	q := r.URL.Query()
	f := domain.Filter{Page: 1, Size: 20, Tag: q.Get("tag"), Search: q.Get("q"), Status: q.Get("status"), Sort: q.Get("sort")}
	for _, x := range []struct {
		k string
		p *int
	}{{"page", &f.Page}, {"page_size", &f.Size}} {
		if q.Get(x.k) != "" {
			n, e := strconv.Atoi(q.Get(x.k))
			if e != nil || n < 1 {
				return f, domain.Invalid(x.k, "Ожидается положительное число")
			}
			*x.p = n
		}
	}
	if f.Size > 100 || f.Page > 100000 {
		return f, domain.Invalid("pagination", "page_size <=100, page <=100000")
	}
	if q.Get("author_id") != "" {
		n, e := strconv.ParseInt(q.Get("author_id"), 10, 64)
		if e != nil || n < 1 {
			return f, domain.Invalid("author_id", "Ожидается положительное число")
		}
		f.Author = n
	}
	if len(f.Search) > 200 {
		return f, domain.Invalid("q", "Максимум 200 байт")
	}
	if len(f.Tag) > 40 {
		return f, domain.Invalid("tag", "Максимум 40 байт")
	}
	if f.Status != "" && f.Status != "draft" && f.Status != "published" {
		return f, domain.Invalid("status", "draft или published")
	}
	allowed := map[string]bool{"published_at": true, "views": true, "popularity": true, "created_at": true}
	if stats {
		allowed = map[string]bool{"published_at": true, "views": true, "views_period": true, "likes": true, "comments": true}
	}
	if f.Sort == "" {
		if stats {
			f.Sort = "views_period"
		} else {
			f.Sort = "published_at"
		}
	}
	if !allowed[f.Sort] {
		return f, domain.Invalid("sort", "Неизвестная сортировка")
	}
	return f, nil
}
func (a *API) period(r *http.Request) (domain.Period, error) {
	return a.Service.Period(r.URL.Query().Get("date_from"), r.URL.Query().Get("date_to"))
}
func (a *API) allow(key string, limit int) bool {
	a.mu.Lock()
	defer a.mu.Unlock()
	now := time.Now()
	if now.Sub(a.lastSweep) >= time.Minute {
		for k, b := range a.buckets {
			if now.Sub(b.start) >= time.Minute {
				delete(a.buckets, k)
			}
		}
		a.lastSweep = now
	}
	if len(a.buckets) >= 10000 {
		if _, present := a.buckets[key]; !present {
			return false
		}
	}
	b := a.buckets[key]
	if now.Sub(b.start) >= time.Minute {
		b = bucket{start: now}
	}
	b.n++
	a.buckets[key] = b
	return b.n <= limit
}
func (a *API) middleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		rid := service.Random()
		ctx := context.WithValue(r.Context(), requestKey, rid)
		r = r.WithContext(ctx)
		w.Header().Set("X-Request-ID", rid)
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("Cache-Control", "no-store")
		defer func() {
			if recover() != nil {
				a.Log.Error("panic recovered", "request_id", rid)
				a.respond(w, r, nil, domain.Fail(500, "internal_error", "Внутренняя ошибка сервера"), 0)
			}
		}()
		origin := r.Header.Get("Origin")
		if origin != "" {
			allowed := false
			for _, v := range a.Config.Origins {
				if v == origin {
					allowed = true
				}
			}
			w.Header().Add("Vary", "Origin")
			if !allowed {
				a.respond(w, r, nil, domain.Fail(403, "origin_forbidden", "Origin не разрешён"), 0)
				return
			}
			w.Header().Set("Access-Control-Allow-Origin", origin)
			w.Header().Set("Access-Control-Allow-Credentials", "true")
			w.Header().Set("Access-Control-Expose-Headers", "ETag,X-Request-ID,Retry-After")
			if r.Method == "OPTIONS" {
				w.Header().Set("Access-Control-Allow-Methods", "GET,POST,PATCH,PUT,DELETE,OPTIONS")
				w.Header().Set("Access-Control-Allow-Headers", "Authorization,Content-Type,If-Match")
				w.WriteHeader(204)
				return
			}
		}
		host, _, e := net.SplitHostPort(r.RemoteAddr)
		if e != nil {
			host = r.RemoteAddr
		}
		key := service.Hash(host)
		limit := 300
		group := "general"
		if strings.Contains(r.URL.Path, "/auth/") {
			limit = 10
			group = "auth"
		} else if strings.HasSuffix(r.URL.Path, "/views") {
			limit = 60
			group = "views"
		}
		if !a.allow(group+key, limit) {
			w.Header().Set("Retry-After", "60")
			a.respond(w, r, nil, domain.Fail(429, "rate_limited", "Слишком много запросов"), 0)
			return
		}
		if r.ContentLength > 512<<10 {
			a.respond(w, r, nil, domain.Fail(413, "body_too_large", "Тело запроса слишком большое"), 0)
			return
		}
		r.Body = http.MaxBytesReader(w, r.Body, 512<<10)
		ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		var ac domain.Actor
		if h := r.Header.Get("Authorization"); h != "" {
			if !strings.HasPrefix(h, "Bearer ") {
				a.respond(w, r, nil, domain.Fail(401, "invalid_token", "Ожидается Bearer token"), 0)
				return
			}
			var e error
			ac, e = a.Service.Authenticate(ctx, strings.TrimPrefix(h, "Bearer "))
			if e != nil {
				a.respond(w, r, nil, e, 0)
				return
			}
		}
		ctx = context.WithValue(ctx, actorKey, ac)
		start := time.Now()
		next.ServeHTTP(w, r.WithContext(ctx))
		a.Log.Info("http request", "request_id", rid, "method", r.Method, "duration_ms", time.Since(start).Milliseconds())
	})
}
