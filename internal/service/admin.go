package service

import (
	"blog-api/internal/domain"
	"context"
	"net/url"
	"time"

	"golang.org/x/crypto/bcrypt"
)

// AdminTTL is the lifetime of a platform admin token; there is no refresh, the CRM logs in again.
const AdminTTL = 12 * time.Hour

var roles = map[string]bool{"user": true, "author": true, "admin": true}

func (s *Service) CreateAdmin(ctx context.Context, email, password string) (domain.Admin, error) {
	email, e := Email(email)
	if e != nil {
		return domain.Admin{}, e
	}
	hash, e := HashPassword(password)
	if e != nil {
		return domain.Admin{}, e
	}
	return s.Repo.CreateAdmin(ctx, email, hash)
}
func (s *Service) SetAdminPassword(ctx context.Context, email, password string) error {
	email, e := Email(email)
	if e != nil {
		return e
	}
	hash, e := HashPassword(password)
	if e != nil {
		return e
	}
	return s.Repo.SetAdminPassword(ctx, email, hash)
}
func (s *Service) AdminLogin(ctx context.Context, email, password string) (domain.AdminToken, error) {
	email, e := Email(email)
	if e != nil {
		return domain.AdminToken{}, e
	}
	if len(password) < 1 || len(password) > 72 {
		return domain.AdminToken{}, domain.Invalid("password", "1..72 байта")
	}
	id, hash, e := s.Repo.AdminCredentials(ctx, email)
	if e != nil {
		if de, ok := e.(*domain.Error); !ok || de.Status != 404 {
			return domain.AdminToken{}, e
		}
		hash = string(dummyHash)
	}
	if bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil || id == 0 {
		return domain.AdminToken{}, domain.Fail(401, "invalid_credentials", "Неверный email или пароль")
	}
	t := domain.AdminToken{Access: Random(), TokenType: "Bearer", ExpiresIn: int64(AdminTTL.Seconds())}
	return t, s.Repo.NewAdminSession(ctx, id, Hash(t.Access), time.Now().Add(AdminTTL))
}
func (s *Service) AuthenticateAdmin(ctx context.Context, token string) (domain.Admin, error) {
	if len(token) != 43 {
		return domain.Admin{}, domain.Fail(401, "invalid_token", "Токен администратора недействителен")
	}
	return s.Repo.AuthenticateAdmin(ctx, Hash(token))
}
func (s *Service) AdminLogout(ctx context.Context, token string) error {
	return s.Repo.AdminLogout(ctx, Hash(token))
}

// adminSite resolves a site from the admin URL; unlike X-Site-Key, an unknown key is a 404 resource.
func (s *Service) adminSite(ctx context.Context, key string) (domain.Site, error) {
	if len(key) > 64 || !slugRE.MatchString(key) {
		return domain.Site{}, domain.Fail(404, "not_found", "Сайт не найден")
	}
	v, e := s.Repo.Site(ctx, key)
	if e != nil {
		return v, e
	}
	if v.PublicURL == "" {
		v.PublicURL = s.PublicURL
	}
	return v, nil
}
func validateSite(name, publicURL *string) error {
	if name != nil {
		if e := text("name", *name, 0, 200); e != nil {
			return e
		}
	}
	if publicURL != nil && *publicURL != "" {
		u, e := url.Parse(*publicURL)
		if e != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" || u.User != nil || u.RawQuery != "" || u.Fragment != "" || len(*publicURL) > 2048 {
			return domain.Invalid("public_url", "Ожидается http(s) URL без query и fragment либо пустая строка")
		}
	}
	return nil
}
func (s *Service) Sites(ctx context.Context, f domain.Filter) (domain.Page[domain.SiteInfo], error) {
	return s.Repo.Sites(ctx, f)
}
func (s *Service) SiteInfo(ctx context.Context, key string) (domain.SiteInfo, error) {
	if _, e := s.adminSite(ctx, key); e != nil {
		return domain.SiteInfo{}, e
	}
	return s.Repo.SiteInfo(ctx, key)
}
func (s *Service) CreateSite(ctx context.Context, in domain.SiteInput) (domain.SiteInfo, error) {
	if len(in.Key) > 64 || !slugRE.MatchString(in.Key) {
		return domain.SiteInfo{}, domain.Invalid("key", "Строчные латинские буквы, цифры и дефис, до 64 символов")
	}
	if e := validateSite(&in.Name, &in.PublicURL); e != nil {
		return domain.SiteInfo{}, e
	}
	return s.Repo.CreateSite(ctx, in)
}
func (s *Service) UpdateSite(ctx context.Context, key string, p domain.SitePatch) (domain.SiteInfo, error) {
	if _, e := s.adminSite(ctx, key); e != nil {
		return domain.SiteInfo{}, e
	}
	if e := validateSite(p.Name, p.PublicURL); e != nil {
		return domain.SiteInfo{}, e
	}
	return s.Repo.UpdateSite(ctx, key, p)
}

func (s *Service) Users(ctx context.Context, key string, f domain.Filter) (domain.Page[domain.OwnUser], error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.Page[domain.OwnUser]{}, e
	}
	if f.Role != "" && !roles[f.Role] {
		return domain.Page[domain.OwnUser]{}, domain.Invalid("role", "user, author или admin")
	}
	return s.Repo.Users(ctx, site.ID, f)
}
func (s *Service) User(ctx context.Context, key string, id int64) (domain.OwnUser, error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.OwnUser{}, e
	}
	return s.Repo.User(ctx, site.ID, id)
}
func (s *Service) CreateUser(ctx context.Context, key string, in domain.UserInput) (domain.OwnUser, error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.OwnUser{}, e
	}
	if !usernameRE.MatchString(in.Username) {
		return domain.OwnUser{}, domain.Invalid("username", "Формат [a-z0-9_], 3..32")
	}
	if in.Email, e = Email(in.Email); e != nil {
		return domain.OwnUser{}, e
	}
	if in.Role == "" {
		in.Role = "user"
	}
	if !roles[in.Role] {
		return domain.OwnUser{}, domain.Invalid("role", "user, author или admin")
	}
	if e := text("display_name", in.DisplayName, 0, 100); e != nil {
		return domain.OwnUser{}, e
	}
	hash, e := HashPassword(in.Password)
	if e != nil {
		return domain.OwnUser{}, e
	}
	return s.Repo.AdminCreateUser(ctx, site.ID, in, hash)
}
func (s *Service) UpdateUser(ctx context.Context, key string, id int64, p domain.UserPatch) (domain.OwnUser, error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.OwnUser{}, e
	}
	if e := validateProfile(p.ProfilePatch); e != nil {
		return domain.OwnUser{}, e
	}
	if p.Role != nil && !roles[*p.Role] {
		return domain.OwnUser{}, domain.Invalid("role", "user, author или admin")
	}
	var hash *string
	if p.Password != nil {
		h, e := HashPassword(*p.Password)
		if e != nil {
			return domain.OwnUser{}, e
		}
		hash = &h
	}
	return s.Repo.AdminUpdateUser(ctx, site.ID, id, p, hash)
}
func (s *Service) DeleteUser(ctx context.Context, key string, id int64) error {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return e
	}
	return s.Repo.DeleteUser(ctx, site.ID, id)
}

// AdminArticles lists articles of every author and status; Author and Status filter it.
func (s *Service) AdminArticles(ctx context.Context, key string, f domain.Filter) (domain.Page[domain.Article], error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.Page[domain.Article]{}, e
	}
	f.Own = true
	p, e := s.Repo.Articles(ctx, site.ID, 0, f)
	for i := range p.Items {
		p.Items[i] = Canonical(site, p.Items[i])
	}
	return p, e
}
func (s *Service) AdminArticle(ctx context.Context, key string, id int64) (domain.Article, error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.Article{}, e
	}
	v, e := s.Repo.Article(ctx, site.ID, id, 0)
	return Canonical(site, v), e
}

// AdminCreateArticle publishes on behalf of any user of the site, whatever that user's role.
func (s *Service) AdminCreateArticle(ctx context.Context, key string, in domain.AdminArticleInput) (domain.Article, error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.Article{}, e
	}
	if in.AuthorID < 1 {
		return domain.Article{}, domain.Invalid("author_id", "Укажите пользователя этого сайта")
	}
	if _, e := s.Repo.User(ctx, site.ID, in.AuthorID); e != nil {
		if de, ok := e.(*domain.Error); ok && de.Status == 404 {
			return domain.Article{}, domain.Invalid("author_id", "Пользователь не найден на этом сайте")
		}
		return domain.Article{}, e
	}
	if in.Status == "" {
		in.Status = "draft"
	}
	if in.Tags == nil {
		in.Tags = []string{}
	}
	if e := ValidateArticle(in.ArticleInput); e != nil {
		return domain.Article{}, e
	}
	v, e := s.Repo.CreateArticle(ctx, site.ID, in.AuthorID, in.ArticleInput)
	return Canonical(site, v), e
}
func (s *Service) AdminUpdateArticle(ctx context.Context, key string, id, version int64, p domain.ArticlePatch) (domain.Article, error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.Article{}, e
	}
	v, e := s.Repo.Article(ctx, site.ID, id, 0)
	if e != nil {
		return v, e
	}
	if v.Version != version {
		return domain.Article{}, domain.Fail(412, "version_conflict", "Версия статьи изменилась")
	}
	in := mergeArticle(v, p)
	if e := ValidateArticle(in); e != nil {
		return domain.Article{}, e
	}
	v, e = s.Repo.UpdateArticle(ctx, site.ID, id, version, 0, in)
	return Canonical(site, v), e
}
func (s *Service) AdminDeleteArticle(ctx context.Context, key string, id, version int64) error {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return e
	}
	return s.Repo.DeleteArticle(ctx, site.ID, id, version)
}

// CreateInvite returns the raw token once; the CRM turns it into a registration link.
func (s *Service) CreateInvite(ctx context.Context, admin domain.Admin, key string, in domain.InviteInput) (domain.Invite, error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.Invite{}, e
	}
	if in.Role == "" {
		in.Role = "author"
	}
	if !roles[in.Role] {
		return domain.Invite{}, domain.Invalid("role", "user, author или admin")
	}
	if in.ExpiresInHours == 0 {
		in.ExpiresInHours = 72
	}
	if in.ExpiresInHours < 1 || in.ExpiresInHours > 720 {
		return domain.Invite{}, domain.Invalid("expires_in_hours", "1..720")
	}
	var email *string
	if in.Email != "" {
		v, e := Email(in.Email)
		if e != nil {
			return domain.Invite{}, e
		}
		email = &v
	}
	token := Random()
	v, e := s.Repo.CreateInvite(ctx, site.ID, admin.ID, Hash(token), in.Role, email, time.Now().Add(time.Duration(in.ExpiresInHours)*time.Hour))
	v.Token = token
	return v, e
}
func (s *Service) Invites(ctx context.Context, key string, f domain.Filter) (domain.Page[domain.Invite], error) {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return domain.Page[domain.Invite]{}, e
	}
	return s.Repo.Invites(ctx, site.ID, f)
}
func (s *Service) DeleteInvite(ctx context.Context, key string, id int64) error {
	site, e := s.adminSite(ctx, key)
	if e != nil {
		return e
	}
	return s.Repo.DeleteInvite(ctx, site.ID, id)
}

// InvitePreview lets the registration page show the role before the visitor signs up.
func (s *Service) InvitePreview(ctx context.Context, site domain.Site, token string) (domain.InvitePreview, error) {
	if len(token) != 43 {
		return domain.InvitePreview{}, domain.Fail(404, "not_found", "Приглашение не найдено или истекло")
	}
	v, e := s.Repo.ActiveInvite(ctx, site.ID, Hash(token))
	if e != nil {
		if de, ok := e.(*domain.Error); ok && de.Status == 404 {
			return domain.InvitePreview{}, domain.Fail(404, "not_found", "Приглашение не найдено или истекло")
		}
		return domain.InvitePreview{}, e
	}
	return domain.InvitePreview{Role: v.Role, Email: v.Email, ExpiresAt: v.ExpiresAt}, nil
}
