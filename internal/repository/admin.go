package repository

import (
	"blog-api/internal/domain"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) CreateAdmin(ctx context.Context, email, hash string) (domain.Admin, error) {
	a := domain.Admin{Email: email}
	e := s.DB.QueryRow(ctx, "INSERT INTO platform_admins(email,password_hash) VALUES($1,$2) RETURNING id,created_at", email, hash).Scan(&a.ID, &a.CreatedAt)
	return a, Translate(e)
}

// SetAdminPassword also ends every session of that admin.
func (s *Store) SetAdminPassword(ctx context.Context, email, hash string) error {
	return s.Tx(ctx, func(tx pgx.Tx) error {
		var id int64
		if e := tx.QueryRow(ctx, "UPDATE platform_admins SET password_hash=$2 WHERE email=$1 RETURNING id", email, hash).Scan(&id); e != nil {
			return e
		}
		_, e := tx.Exec(ctx, "DELETE FROM admin_sessions WHERE admin_id=$1", id)
		return e
	})
}
func (s *Store) AdminCredentials(ctx context.Context, email string) (int64, string, error) {
	var id int64
	var hash string
	e := s.DB.QueryRow(ctx, "SELECT id,password_hash FROM platform_admins WHERE email=$1", email).Scan(&id, &hash)
	return id, hash, Translate(e)
}
func (s *Store) NewAdminSession(ctx context.Context, admin int64, hash string, expires time.Time) error {
	_, e := s.DB.Exec(ctx, "INSERT INTO admin_sessions(token_hash,admin_id,expires_at) VALUES($1,$2,$3)", hash, admin, expires)
	return Translate(e)
}
func (s *Store) AuthenticateAdmin(ctx context.Context, hash string) (domain.Admin, error) {
	var a domain.Admin
	e := s.DB.QueryRow(ctx, "SELECT p.id,p.email,p.created_at FROM admin_sessions s JOIN platform_admins p ON p.id=s.admin_id WHERE s.token_hash=$1 AND s.expires_at>now()", hash).Scan(&a.ID, &a.Email, &a.CreatedAt)
	if e == pgx.ErrNoRows {
		return a, domain.Fail(401, "invalid_token", "Токен администратора недействителен")
	}
	return a, e
}
func (s *Store) AdminLogout(ctx context.Context, hash string) error {
	_, e := s.DB.Exec(ctx, "DELETE FROM admin_sessions WHERE token_hash=$1", hash)
	return e
}

const siteCols = "id,key,name,public_url,created_at"

func scanSite(row pgx.Row) (domain.SiteInfo, error) {
	var v domain.SiteInfo
	e := row.Scan(&v.ID, &v.Key, &v.Name, &v.PublicURL, &v.CreatedAt)
	return v, Translate(e)
}
func (s *Store) SiteInfo(ctx context.Context, key string) (domain.SiteInfo, error) {
	return scanSite(s.DB.QueryRow(ctx, "SELECT "+siteCols+" FROM sites WHERE key=$1", key))
}
func (s *Store) Sites(ctx context.Context, f domain.Filter) (domain.Page[domain.SiteInfo], error) {
	p := domain.Page[domain.SiteInfo]{Items: []domain.SiteInfo{}, Page: f.Page, PageSize: f.Size}
	if e := s.DB.QueryRow(ctx, "SELECT count(*) FROM sites").Scan(&p.Total); e != nil {
		return p, e
	}
	rows, e := s.DB.Query(ctx, "SELECT "+siteCols+" FROM sites ORDER BY id LIMIT $1 OFFSET $2", f.Size, (f.Page-1)*f.Size)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanSite(rows)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, v)
	}
	return p, rows.Err()
}
func (s *Store) CreateSite(ctx context.Context, in domain.SiteInput) (domain.SiteInfo, error) {
	return scanSite(s.DB.QueryRow(ctx, "INSERT INTO sites(key,name,public_url) VALUES($1,$2,$3) RETURNING "+siteCols, in.Key, in.Name, in.PublicURL))
}
func (s *Store) UpdateSite(ctx context.Context, key string, p domain.SitePatch) (domain.SiteInfo, error) {
	return scanSite(s.DB.QueryRow(ctx, "UPDATE sites SET name=coalesce($2,name),public_url=coalesce($3,public_url) WHERE key=$1 RETURNING "+siteCols, key, p.Name, p.PublicURL))
}

func scanOwnUser(row pgx.Row) (domain.OwnUser, error) {
	var u domain.OwnUser
	e := row.Scan(&u.ID, &u.Username, &u.DisplayName, &u.AvatarURL, &u.Bio, &u.CreatedAt, &u.Email, &u.Role)
	return u, Translate(e)
}

// Users lists accounts of a site; Search matches username, email or display name.
func (s *Store) Users(ctx context.Context, site int64, f domain.Filter) (domain.Page[domain.OwnUser], error) {
	p := domain.Page[domain.OwnUser]{Items: []domain.OwnUser{}, Page: f.Page, PageSize: f.Size}
	const where = " WHERE site_id=$1 AND ($2='' OR role=$2) AND ($3='' OR strpos(username,$3)>0 OR strpos(email,lower($3))>0 OR strpos(lower(display_name),lower($3))>0)"
	if e := s.DB.QueryRow(ctx, "SELECT count(*) FROM users"+where, site, f.Role, f.Search).Scan(&p.Total); e != nil {
		return p, e
	}
	rows, e := s.DB.Query(ctx, "SELECT "+userCols+" FROM users"+where+" ORDER BY id LIMIT $4 OFFSET $5", site, f.Role, f.Search, f.Size, (f.Page-1)*f.Size)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		u, e := scanOwnUser(rows)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, u)
	}
	return p, rows.Err()
}
func (s *Store) AdminCreateUser(ctx context.Context, site int64, in domain.UserInput, hash string) (domain.OwnUser, error) {
	return scanOwnUser(s.DB.QueryRow(ctx, "INSERT INTO users(site_id,username,email,password_hash,role,display_name) VALUES($1,$2,$3,$4,$5,$6) RETURNING "+userCols, site, in.Username, in.Email, hash, in.Role, in.DisplayName))
}

// AdminUpdateUser changes profile, role and password; a new password ends all sessions of the user.
func (s *Store) AdminUpdateUser(ctx context.Context, site, id int64, p domain.UserPatch, hash *string) (domain.OwnUser, error) {
	var u domain.OwnUser
	e := s.Tx(ctx, func(tx pgx.Tx) error {
		var e error
		u, e = scanOwnUser(tx.QueryRow(ctx, `UPDATE users SET display_name=coalesce($3,display_name),avatar_url=coalesce($4,avatar_url),bio=coalesce($5,bio),role=coalesce($6,role),password_hash=coalesce($7,password_hash) WHERE id=$1 AND site_id=$2 RETURNING `+userCols, id, site, p.DisplayName, p.AvatarURL, p.Bio, p.Role, hash))
		if e != nil || hash == nil {
			return e
		}
		_, e = tx.Exec(ctx, "UPDATE session_families SET revoked=true WHERE user_id=$1", id)
		return e
	})
	return u, e
}

// DeleteUser removes the account with its articles, comments, likes and sessions.
func (s *Store) DeleteUser(ctx context.Context, site, id int64) error {
	r, e := s.DB.Exec(ctx, "DELETE FROM users WHERE id=$1 AND site_id=$2", id, site)
	if e != nil {
		return Translate(e)
	}
	if r.RowsAffected() == 0 {
		return domain.Fail(404, "not_found", "Пользователь не найден")
	}
	return nil
}

const inviteCols = "id,role,email,created_at,expires_at,used_by,used_at"

func scanInvite(row pgx.Row) (domain.Invite, error) {
	var v domain.Invite
	e := row.Scan(&v.ID, &v.Role, &v.Email, &v.CreatedAt, &v.ExpiresAt, &v.UsedBy, &v.UsedAt)
	return v, Translate(e)
}
func (s *Store) CreateInvite(ctx context.Context, site, admin int64, hash, role string, email *string, expires time.Time) (domain.Invite, error) {
	return scanInvite(s.DB.QueryRow(ctx, "INSERT INTO invites(token_hash,site_id,role,email,created_by,expires_at) VALUES($1,$2,$3,$4,$5,$6) RETURNING "+inviteCols, hash, site, role, email, admin, expires))
}
func (s *Store) Invites(ctx context.Context, site int64, f domain.Filter) (domain.Page[domain.Invite], error) {
	p := domain.Page[domain.Invite]{Items: []domain.Invite{}, Page: f.Page, PageSize: f.Size}
	if e := s.DB.QueryRow(ctx, "SELECT count(*) FROM invites WHERE site_id=$1", site).Scan(&p.Total); e != nil {
		return p, e
	}
	rows, e := s.DB.Query(ctx, "SELECT "+inviteCols+" FROM invites WHERE site_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3", site, f.Size, (f.Page-1)*f.Size)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		v, e := scanInvite(rows)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, v)
	}
	return p, rows.Err()
}
func (s *Store) DeleteInvite(ctx context.Context, site, id int64) error {
	r, e := s.DB.Exec(ctx, "DELETE FROM invites WHERE id=$1 AND site_id=$2", id, site)
	if e != nil {
		return e
	}
	if r.RowsAffected() == 0 {
		return domain.Fail(404, "not_found", "Приглашение не найдено")
	}
	return nil
}

// ActiveInvite returns an unused, unexpired invite of the site.
func (s *Store) ActiveInvite(ctx context.Context, site int64, hash string) (domain.Invite, error) {
	return scanInvite(s.DB.QueryRow(ctx, "SELECT "+inviteCols+" FROM invites WHERE token_hash=$1 AND site_id=$2 AND used_at IS NULL AND expires_at>now()", hash, site))
}
