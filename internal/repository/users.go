package repository

import (
	"blog-api/internal/domain"
	"context"
)

const userCols = "id,username,display_name,avatar_url,bio,created_at,email,role"

func (s *Store) Site(ctx context.Context, key string) (domain.Site, error) {
	v := domain.Site{Key: key}
	e := s.DB.QueryRow(ctx, "SELECT id,public_url FROM sites WHERE key=$1", key).Scan(&v.ID, &v.PublicURL)
	return v, Translate(e)
}
func (s *Store) User(ctx context.Context, site, id int64) (domain.OwnUser, error) {
	var u domain.OwnUser
	e := s.DB.QueryRow(ctx, "SELECT "+userCols+" FROM users WHERE id=$1 AND site_id=$2", id, site).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.Bio, &u.CreatedAt, &u.Email, &u.Role)
	return u, Translate(e)
}
func (s *Store) CreateUser(ctx context.Context, site int64, username, email, hash string) (domain.OwnUser, error) {
	var id int64
	e := s.DB.QueryRow(ctx, "INSERT INTO users(site_id,username,email,password_hash) VALUES($1,$2,$3,$4) RETURNING id", site, username, email, hash).Scan(&id)
	if e != nil {
		return domain.OwnUser{}, Translate(e)
	}
	return s.User(ctx, site, id)
}
func (s *Store) Credentials(ctx context.Context, site int64, email string) (int64, string, error) {
	var id int64
	var hash string
	e := s.DB.QueryRow(ctx, "SELECT id,password_hash FROM users WHERE email=$1 AND site_id=$2", email, site).Scan(&id, &hash)
	return id, hash, Translate(e)
}
func (s *Store) UpdateUser(ctx context.Context, site, id int64, p domain.ProfilePatch) (domain.OwnUser, error) {
	_, e := s.DB.Exec(ctx, "UPDATE users SET display_name=coalesce($2,display_name),avatar_url=coalesce($3,avatar_url),bio=coalesce($4,bio) WHERE id=$1 AND site_id=$5", id, p.DisplayName, p.AvatarURL, p.Bio, site)
	if e != nil {
		return domain.OwnUser{}, Translate(e)
	}
	return s.User(ctx, site, id)
}
func (s *Store) Metrics(ctx context.Context, id int64, public bool) (domain.Metrics, error) {
	var m domain.Metrics
	e := s.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='published'),count(*) FILTER(WHERE status='draft'),coalesce(sum(views),0),coalesce(sum(likes),0),coalesce(sum(comments),0) FROM article_metrics WHERE author_id=$1 AND (NOT $2 OR status='published')`, id, public).Scan(&m.Published, &m.Drafts, &m.Views, &m.Likes, &m.Comments)
	return m, e
}
