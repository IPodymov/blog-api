package repository

import (
	"blog-api/internal/domain"
	"context"
	"time"

	"github.com/jackc/pgx/v5"
)

func (s *Store) NewSession(ctx context.Context, user int64, family, access, refresh string, at, rt time.Time) error {
	return s.Tx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "INSERT INTO session_families(id,user_id,expires_at) VALUES($1,$2,$3)", family, user, rt); e != nil {
			return e
		}
		return insertTokens(ctx, tx, family, access, refresh, at)
	})
}
func insertTokens(ctx context.Context, tx pgx.Tx, family, access, refresh string, at time.Time) error {
	if _, e := tx.Exec(ctx, "INSERT INTO refresh_sessions(token_hash,family_id) VALUES($1,$2)", refresh, family); e != nil {
		return e
	}
	_, e := tx.Exec(ctx, "INSERT INTO access_sessions(token_hash,family_id,expires_at) VALUES($1,$2,$3)", access, family, at)
	return e
}
func (s *Store) Rotate(ctx context.Context, site int64, old, access, refresh string, at time.Time) error {
	invalid := false
	e := s.Tx(ctx, func(tx pgx.Tx) error {
		var family string
		// A token of another site is treated as unknown and does not revoke that site's session.
		if e := tx.QueryRow(ctx, "SELECT r.family_id FROM refresh_sessions r JOIN session_families f ON f.id=r.family_id JOIN users u ON u.id=f.user_id WHERE r.token_hash=$1 AND u.site_id=$2", old, site).Scan(&family); e != nil {
			if e == pgx.ErrNoRows {
				invalid = true
				return nil
			}
			return e
		}
		var revoked, expired, used bool
		if e := tx.QueryRow(ctx, "SELECT revoked,expires_at<=now() FROM session_families WHERE id=$1 FOR UPDATE", family).Scan(&revoked, &expired); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, "SELECT used FROM refresh_sessions WHERE token_hash=$1", old).Scan(&used); e != nil {
			return e
		}
		if revoked || expired || used {
			invalid = true
			_, e := tx.Exec(ctx, "UPDATE session_families SET revoked=true WHERE id=$1", family)
			return e
		}
		if _, e := tx.Exec(ctx, "UPDATE refresh_sessions SET used=true WHERE token_hash=$1", old); e != nil {
			return e
		}
		return insertTokens(ctx, tx, family, access, refresh, at)
	})
	if e != nil {
		return e
	}
	if invalid {
		return domain.Fail(401, "invalid_token", "Refresh-токен недействителен")
	}
	return nil
}
func (s *Store) Authenticate(ctx context.Context, site domain.Site, hash string) (domain.Actor, error) {
	a := domain.Actor{Site: site}
	e := s.DB.QueryRow(ctx, `SELECT u.id,u.role FROM access_sessions ac JOIN session_families f ON f.id=ac.family_id JOIN users u ON u.id=f.user_id WHERE ac.token_hash=$1 AND u.site_id=$2 AND ac.expires_at>now() AND f.expires_at>now() AND NOT f.revoked`, hash, site.ID).Scan(&a.ID, &a.Role)
	if e == pgx.ErrNoRows {
		return a, domain.Fail(401, "invalid_token", "Access-токен недействителен")
	}
	return a, e
}
func (s *Store) Logout(ctx context.Context, site int64, hash string) error {
	_, e := s.DB.Exec(ctx, "UPDATE session_families f SET revoked=true FROM users u WHERE u.id=f.user_id AND u.site_id=$2 AND f.id=(SELECT family_id FROM refresh_sessions WHERE token_hash=$1)", hash, site)
	return e
}

// Register commits the account and its first session atomically.
// A non-empty invite hash grants the invite's role and consumes the invite in the same transaction.
func (s *Store) Register(ctx context.Context, site int64, username, email, passwordHash, invite, family, access, refresh string, at, rt time.Time) error {
	return s.Tx(ctx, func(tx pgx.Tx) error {
		role := "user"
		var inviteID int64
		if invite != "" {
			var only *string
			e := tx.QueryRow(ctx, "SELECT id,role,email FROM invites WHERE token_hash=$1 AND site_id=$2 AND used_at IS NULL AND expires_at>now() FOR UPDATE", invite, site).Scan(&inviteID, &role, &only)
			if e == pgx.ErrNoRows {
				return domain.Invalid("invite_token", "Приглашение недействительно, использовано или истекло")
			}
			if e != nil {
				return e
			}
			if only != nil && *only != email {
				return domain.Invalid("email", "Приглашение выдано на другой email")
			}
		}
		var id int64
		if e := tx.QueryRow(ctx, "INSERT INTO users(site_id,username,email,password_hash,role) VALUES($1,$2,$3,$4,$5) RETURNING id", site, username, email, passwordHash, role).Scan(&id); e != nil {
			return e
		}
		if inviteID != 0 {
			if _, e := tx.Exec(ctx, "UPDATE invites SET used_by=$2,used_at=now() WHERE id=$1", inviteID, id); e != nil {
				return e
			}
		}
		if _, e := tx.Exec(ctx, "INSERT INTO session_families(id,user_id,expires_at) VALUES($1,$2,$3)", family, id, rt); e != nil {
			return e
		}
		return insertTokens(ctx, tx, family, access, refresh, at)
	})
}
