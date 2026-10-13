package repository

import (
	"blog-api/internal/domain"
	"context"
	"github.com/jackc/pgx/v5"
	"time"
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
func (s *Store) Rotate(ctx context.Context, old, access, refresh string, at time.Time) error {
	invalid := false
	e := s.Tx(ctx, func(tx pgx.Tx) error {
		var family string
		if e := tx.QueryRow(ctx, "SELECT family_id FROM refresh_sessions WHERE token_hash=$1", old).Scan(&family); e != nil {
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
func (s *Store) Authenticate(ctx context.Context, hash string) (domain.Actor, error) {
	var a domain.Actor
	e := s.DB.QueryRow(ctx, `SELECT u.id,u.role FROM access_sessions ac JOIN session_families f ON f.id=ac.family_id JOIN users u ON u.id=f.user_id WHERE ac.token_hash=$1 AND ac.expires_at>now() AND f.expires_at>now() AND NOT f.revoked`, hash).Scan(&a.ID, &a.Role)
	if e == pgx.ErrNoRows {
		return a, domain.Fail(401, "invalid_token", "Access-токен недействителен")
	}
	return a, e
}
func (s *Store) Logout(ctx context.Context, hash string) error {
	_, e := s.DB.Exec(ctx, "UPDATE session_families SET revoked=true WHERE id=(SELECT family_id FROM refresh_sessions WHERE token_hash=$1)", hash)
	return e
}

// Register commits the account and its first session atomically.
func (s *Store) Register(ctx context.Context, username, email, passwordHash, family, access, refresh string, at, rt time.Time) error {
	return s.Tx(ctx, func(tx pgx.Tx) error {
		var id int64
		if e := tx.QueryRow(ctx, "INSERT INTO users(username,email,password_hash) VALUES($1,$2,$3) RETURNING id", username, email, passwordHash).Scan(&id); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "INSERT INTO session_families(id,user_id,expires_at) VALUES($1,$2,$3)", family, id, rt); e != nil {
			return e
		}
		return insertTokens(ctx, tx, family, access, refresh, at)
	})
}
