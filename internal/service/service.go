package service

import (
	"blog-api/internal/domain"
	"blog-api/internal/repository"
	"context"
	"crypto/hmac"
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"encoding/hex"
	"golang.org/x/crypto/bcrypt"
	"net/mail"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode/utf8"
)

type Service struct {
	Repo                                *repository.Store
	PublicURL, VisitorSecret            string
	AccessTTL, RefreshTTL, ViewInterval time.Duration
	MaxPeriod                           int
}

func Random() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return base64.RawURLEncoding.EncodeToString(b)
}
func Hash(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (s *Service) VisitorHash(id string) string {
	h := hmac.New(sha256.New, []byte(s.VisitorSecret))
	h.Write([]byte(id))
	return hex.EncodeToString(h.Sum(nil))
}
func (s *Service) SignVisitor(id string) string { return id + "." + s.VisitorHash("cookie:"+id) }
func (s *Service) VerifyVisitor(cookie string) (string, bool) {
	p := strings.Split(cookie, ".")
	if len(p) != 2 || len(p[0]) != 43 {
		return "", false
	}
	want := s.VisitorHash("cookie:" + p[0])
	return p[0], hmac.Equal([]byte(want), []byte(p[1]))
}
func require(a domain.Actor) error {
	if a.ID == 0 {
		return domain.Fail(401, "authentication_required", "Требуется авторизация")
	}
	return nil
}
func manage(a domain.Actor, owner int64) error {
	if e := require(a); e != nil {
		return e
	}
	if !a.CanManage(owner) {
		return domain.Fail(403, "forbidden", "Нет доступа к ресурсу")
	}
	return nil
}
func text(field, value string, min, max int) error {
	n := utf8.RuneCountInString(value)
	if n < min || n > max || (min > 0 && strings.TrimSpace(value) == "") {
		return domain.Invalid(field, "Недопустимая длина")
	}
	return nil
}

var usernameRE = regexp.MustCompile(`^[a-z0-9_]{3,32}$`)
var slugRE = regexp.MustCompile(`^[a-z0-9]+(-[a-z0-9]+)*$`)

func ValidateArticle(in domain.ArticleInput) error {
	for _, x := range []struct {
		f, v     string
		min, max int
	}{{"title", in.Title, 1, 200}, {"slug", in.Slug, 1, 200}, {"description", in.Description, 0, 500}, {"content", in.Content, 1, 100000}} {
		if e := text(x.f, x.v, x.min, x.max); e != nil {
			return e
		}
	}
	if !slugRE.MatchString(in.Slug) {
		return domain.Invalid("slug", "Используйте строчные латинские буквы, цифры и дефис")
	}
	if in.Status != "draft" && in.Status != "published" {
		return domain.Invalid("status", "draft или published")
	}
	if len(in.Tags) > 10 {
		return domain.Invalid("tags", "Максимум 10 тегов")
	}
	seen := map[string]bool{}
	for _, t := range in.Tags {
		if len(t) > 40 || !slugRE.MatchString(t) || seen[t] {
			return domain.Invalid("tags", "Теги уникальны, длина 1..40, формат slug")
		}
		seen[t] = true
	}
	return nil
}
func (s *Service) Canonical(a domain.Article) domain.Article {
	if a.Status == "published" {
		u := strings.TrimRight(s.PublicURL, "/") + "/articles/" + a.Slug
		a.CanonicalURL = &u
	}
	return a
}
func (s *Service) Register(ctx context.Context, username, email, password string) (domain.Tokens, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	if !usernameRE.MatchString(username) {
		return domain.Tokens{}, domain.Invalid("username", "Формат [a-z0-9_], 3..32")
	}
	m, e := mail.ParseAddress(email)
	if e != nil || m.Address != email || len(email) > 254 {
		return domain.Tokens{}, domain.Invalid("email", "Некорректный email")
	}
	if len(password) < 12 || len(password) > 72 {
		return domain.Tokens{}, domain.Invalid("password", "12..72 байта")
	}
	hash, e := bcrypt.GenerateFromPassword([]byte(password), 12)
	if e != nil {
		return domain.Tokens{}, e
	}
	tokens := s.tokenPair()
	now := time.Now()
	e = s.Repo.Register(ctx, username, email, string(hash), Random(), Hash(tokens.Access), Hash(tokens.Refresh), now.Add(s.AccessTTL), now.Add(s.RefreshTTL))
	return tokens, e
}

var dummyHash, _ = bcrypt.GenerateFromPassword([]byte("constant-dummy-password"), 12)

func (s *Service) Login(ctx context.Context, email, password string) (domain.Tokens, error) {
	email = strings.ToLower(strings.TrimSpace(email))
	address, parseErr := mail.ParseAddress(email)
	if parseErr != nil || address.Address != email || len(email) > 254 {
		return domain.Tokens{}, domain.Invalid("email", "Некорректный email")
	}
	if len(password) < 1 || len(password) > 72 {
		return domain.Tokens{}, domain.Invalid("password", "1..72 байта")
	}

	id, hash, e := s.Repo.Credentials(ctx, strings.ToLower(strings.TrimSpace(email)))
	if e != nil {
		if de, ok := e.(*domain.Error); !ok || de.Status != 404 {
			return domain.Tokens{}, e
		}
		hash = string(dummyHash)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || id == 0 {
		return domain.Tokens{}, domain.Fail(401, "invalid_credentials", "Неверный email или пароль")
	}
	return s.newTokens(ctx, id)
}
func (s *Service) tokenPair() domain.Tokens {
	return domain.Tokens{Access: Random(), Refresh: Random(), TokenType: "Bearer", ExpiresIn: int64(s.AccessTTL.Seconds())}
}
func (s *Service) newTokens(ctx context.Context, id int64) (domain.Tokens, error) {
	t := s.tokenPair()
	now := time.Now()
	e := s.Repo.NewSession(ctx, id, Random(), Hash(t.Access), Hash(t.Refresh), now.Add(s.AccessTTL), now.Add(s.RefreshTTL))
	return t, e
}
func (s *Service) Refresh(ctx context.Context, old string) (domain.Tokens, error) {
	if len(old) != 43 {
		return domain.Tokens{}, domain.Fail(401, "invalid_token", "Refresh-токен недействителен")
	}
	t := s.tokenPair()
	e := s.Repo.Rotate(ctx, Hash(old), Hash(t.Access), Hash(t.Refresh), time.Now().Add(s.AccessTTL))
	return t, e
}
func (s *Service) Logout(ctx context.Context, old string) error { return s.Repo.Logout(ctx, Hash(old)) }
func (s *Service) Authenticate(ctx context.Context, token string) (domain.Actor, error) {
	if len(token) != 43 {
		return domain.Actor{}, domain.Fail(401, "invalid_token", "Access-токен недействителен")
	}
	return s.Repo.Authenticate(ctx, Hash(token))
}
func (s *Service) PublicProfile(ctx context.Context, id int64) (domain.Profile, error) {
	u, e := s.Repo.User(ctx, id)
	if e != nil {
		return domain.Profile{}, e
	}
	m, e := s.Repo.Metrics(ctx, id, true)
	return domain.Profile{User: u.User, Statistics: domain.PublicMetrics{Published: m.Published, Views: m.Views, Likes: m.Likes, Comments: m.Comments}}, e
}
func (s *Service) Me(ctx context.Context, a domain.Actor) (domain.OwnUser, error) {
	if e := require(a); e != nil {
		return domain.OwnUser{}, e
	}
	return s.Repo.User(ctx, a.ID)
}
func (s *Service) UpdateProfile(ctx context.Context, a domain.Actor, p domain.ProfilePatch) (domain.OwnUser, error) {
	if e := require(a); e != nil {
		return domain.OwnUser{}, e
	}
	if p.DisplayName != nil {
		if e := text("display_name", *p.DisplayName, 0, 100); e != nil {
			return domain.OwnUser{}, e
		}
	}
	if p.Bio != nil {
		if e := text("bio", *p.Bio, 0, 2000); e != nil {
			return domain.OwnUser{}, e
		}
	}
	if p.AvatarURL != nil && *p.AvatarURL != "" {
		u, e := url.Parse(*p.AvatarURL)
		if e != nil || u.Scheme != "https" || u.Host == "" || u.User != nil || len(*p.AvatarURL) > 2048 {
			return domain.OwnUser{}, domain.Invalid("avatar_url", "Ожидается HTTPS URL либо пустая строка")
		}
	}
	return s.Repo.UpdateUser(ctx, a.ID, p)
}
func (s *Service) Article(ctx context.Context, a domain.Actor, id int64) (domain.Article, error) {
	v, e := s.Repo.Article(ctx, id, a.ID)
	if e != nil {
		return v, e
	}
	if !a.CanRead(v.Author.ID, v.Status) {
		return domain.Article{}, domain.Fail(404, "not_found", "Статья не найдена")
	}
	return s.Canonical(v), nil
}
func (s *Service) Slug(ctx context.Context, a domain.Actor, slug string) (domain.Article, error) {
	v, e := s.Repo.BySlug(ctx, slug, a.ID)
	return s.Canonical(v), e
}
func (s *Service) Articles(ctx context.Context, a domain.Actor, f domain.Filter) (domain.Page[domain.Article], error) {
	if f.Own {
		if e := require(a); e != nil {
			return domain.Page[domain.Article]{}, e
		}
		f.Author = a.ID
	}
	p, e := s.Repo.Articles(ctx, a.ID, f)
	for i := range p.Items {
		p.Items[i] = s.Canonical(p.Items[i])
	}
	return p, e
}
func (s *Service) CreateArticle(ctx context.Context, a domain.Actor, in domain.ArticleInput) (domain.Article, error) {
	if e := require(a); e != nil {
		return domain.Article{}, e
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	if e := ValidateArticle(in); e != nil {
		return domain.Article{}, e
	}
	v, e := s.Repo.CreateArticle(ctx, a.ID, in)
	return s.Canonical(v), e
}
func (s *Service) UpdateArticle(ctx context.Context, a domain.Actor, id, version int64, p domain.ArticlePatch) (domain.Article, error) {
	v, e := s.Repo.Article(ctx, id, a.ID)
	if e != nil {
		return v, e
	}
	if e := manage(a, v.Author.ID); e != nil {
		return domain.Article{}, e
	}
	if v.Version != version {
		return domain.Article{}, domain.Fail(412, "version_conflict", "Версия статьи изменилась")
	}
	in := domain.ArticleInput{Title: v.Title, Slug: v.Slug, Description: v.Description, Content: v.Content, Tags: v.Tags, Status: v.Status}
	if p.Title != nil {
		in.Title = *p.Title
	}
	if p.Slug != nil {
		in.Slug = *p.Slug
	}
	if p.Description != nil {
		in.Description = *p.Description
	}
	if p.Content != nil {
		in.Content = *p.Content
	}
	if p.Tags != nil {
		in.Tags = *p.Tags
	}
	if p.Status != nil {
		in.Status = *p.Status
	}
	if e := ValidateArticle(in); e != nil {
		return domain.Article{}, e
	}
	v, e = s.Repo.UpdateArticle(ctx, id, version, a.ID, in)
	return s.Canonical(v), e
}
func (s *Service) DeleteArticle(ctx context.Context, a domain.Actor, id, version int64) error {
	v, e := s.Repo.Article(ctx, id, a.ID)
	if e != nil {
		return e
	}
	if e := manage(a, v.Author.ID); e != nil {
		return e
	}
	return s.Repo.DeleteArticle(ctx, id, version)
}
func (s *Service) View(ctx context.Context, a domain.Actor, id int64, visitor string) (bool, error) {
	return s.Repo.View(ctx, id, a.ID, a.Role == "admin", s.VisitorHash(visitor), s.ViewInterval)
}
func (s *Service) Like(ctx context.Context, a domain.Actor, id int64, on bool) (int64, bool, error) {
	if e := require(a); e != nil {
		return 0, false, e
	}
	return s.Repo.Like(ctx, id, a.ID, on)
}
func (s *Service) Comments(ctx context.Context, a domain.Actor, id int64, f domain.Filter) (domain.Page[domain.Comment], error) {
	if _, e := s.Article(ctx, a, id); e != nil {
		return domain.Page[domain.Comment]{}, e
	}
	return s.Repo.Comments(ctx, id, f)
}
func (s *Service) CreateComment(ctx context.Context, a domain.Actor, id int64, in domain.CommentInput) (domain.Comment, error) {
	if e := require(a); e != nil {
		return domain.Comment{}, e
	}
	if e := text("content", in.Content, 1, 5000); e != nil {
		return domain.Comment{}, e
	}
	return s.Repo.CreateComment(ctx, id, a.ID, in)
}
func (s *Service) UpdateComment(ctx context.Context, a domain.Actor, id int64, content string) (domain.Comment, error) {
	c, e := s.Repo.Comment(ctx, id)
	if e != nil {
		return c, e
	}
	if e := manage(a, c.Author.ID); e != nil {
		return domain.Comment{}, e
	}
	if e := text("content", content, 1, 5000); e != nil {
		return domain.Comment{}, e
	}
	return s.Repo.UpdateComment(ctx, id, content)
}
func (s *Service) DeleteComment(ctx context.Context, a domain.Actor, id int64) error {
	c, e := s.Repo.Comment(ctx, id)
	if e != nil {
		return e
	}
	if e := manage(a, c.Author.ID); e != nil {
		return e
	}
	return s.Repo.DeleteComment(ctx, id)
}
func (s *Service) Period(from, to string) (domain.Period, error) {
	return domain.ParsePeriod(from, to, time.Now(), s.MaxPeriod)
}
func (s *Service) Statistics(ctx context.Context, a domain.Actor, p domain.Period) (domain.Statistics, error) {
	if e := require(a); e != nil {
		return domain.Statistics{}, e
	}
	m, e := s.Repo.Metrics(ctx, a.ID, false)
	if e != nil {
		return domain.Statistics{}, e
	}
	daily, total, e := s.Repo.Daily(ctx, a.ID, 0, p)
	if e != nil {
		return domain.Statistics{}, e
	}
	top, e := s.StatisticsArticles(ctx, a, domain.Filter{Page: 1, Size: 5, Sort: "views_period"}, p)
	return domain.Statistics{Metrics: m, DateFrom: p.FromDate(), DateTo: p.ToDate(), Timezone: "UTC", ViewsPeriod: total, Daily: daily, Top: top.Items}, e
}
func (s *Service) StatisticsArticles(ctx context.Context, a domain.Actor, f domain.Filter, p domain.Period) (domain.Page[domain.ArticleStat], error) {
	if e := require(a); e != nil {
		return domain.Page[domain.ArticleStat]{}, e
	}
	v, e := s.Repo.ArticleStats(ctx, a.ID, a.ID, f, p)
	for i := range v.Items {
		v.Items[i].Article = s.Canonical(v.Items[i].Article)
	}
	return v, e
}
func (s *Service) ArticleStatistics(ctx context.Context, a domain.Actor, id int64, p domain.Period) (domain.ArticleStatistics, error) {
	v, e := s.Repo.Article(ctx, id, a.ID)
	if e != nil {
		return domain.ArticleStatistics{}, e
	}
	if e := manage(a, v.Author.ID); e != nil {
		return domain.ArticleStatistics{}, e
	}
	daily, total, e := s.Repo.Daily(ctx, 0, id, p)
	return domain.ArticleStatistics{ArticleID: id, ViewsAll: v.Views, ViewsPeriod: total, Likes: v.Likes, Comments: v.Comments, DateFrom: p.FromDate(), DateTo: p.ToDate(), Timezone: "UTC", Daily: daily}, e
}
