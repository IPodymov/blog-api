package repository

import (
	"blog-api/internal/domain"
	"context"
)

const userCols = "id,username,display_name,avatar_url,bio,created_at,email,role"

func (s *Store) User(ctx context.Context, id int64) (domain.OwnUser, error) {
	var u domain.OwnUser
	e := s.DB.QueryRow(ctx, "SELECT "+userCols+" FROM users WHERE id=$1", id).Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.Bio, &u.CreatedAt, &u.Email, &u.Role)
	return u, Translate(e)
}
func (s *Store) CreateUser(ctx context.Context, username, email, hash string) (domain.OwnUser, error) {
	var id int64
	e := s.DB.QueryRow(ctx, "INSERT INTO users(username,email,password_hash) VALUES($1,$2,$3) RETURNING id", username, email, hash).Scan(&id)
	if e != nil {
		return domain.OwnUser{}, Translate(e)
	}
	return s.User(ctx, id)
}
func (s *Store) Credentials(ctx context.Context, email string) (int64, string, error) {
	var id int64
	var hash string
	e := s.DB.QueryRow(ctx, "SELECT id,password_hash FROM users WHERE email=$1", email).Scan(&id, &hash)
	return id, hash, Translate(e)
}
func (s *Store) UpdateUser(ctx context.Context, id int64, p domain.ProfilePatch) (domain.OwnUser, error) {
	_, e := s.DB.Exec(ctx, "UPDATE users SET display_name=coalesce($2,display_name),avatar_url=coalesce($3,avatar_url),bio=coalesce($4,bio) WHERE id=$1", id, p.DisplayName, p.AvatarURL, p.Bio)
	if e != nil {
		return domain.OwnUser{}, Translate(e)
	}
	return s.User(ctx, id)
}
func (s *Store) Metrics(ctx context.Context, id int64, public bool) (domain.Metrics, error) {
	var m domain.Metrics
	e := s.DB.QueryRow(ctx, `SELECT count(*) FILTER(WHERE status='published'),count(*) FILTER(WHERE status='draft'),coalesce(sum(views),0),coalesce(sum(likes),0),coalesce(sum(comments),0) FROM article_metrics WHERE author_id=$1 AND (NOT $2 OR status='published')`, id, public).Scan(&m.Published, &m.Drafts, &m.Views, &m.Likes, &m.Comments)
	return m, e
}
