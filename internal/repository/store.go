package repository

import (
	"blog-api/internal/domain"
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Store struct{ DB *pgxpool.Pool }

func Translate(e error) error {
	if e == nil {
		return nil
	}
	if errors.Is(e, pgx.ErrNoRows) {
		return domain.Fail(404, "not_found", "Ресурс не найден")
	}
	var pe *pgconn.PgError
	if errors.As(e, &pe) {
		switch pe.Code {
		case "23505":
			return domain.Fail(409, "already_exists", "Значение уже используется")
		case "23503", "23514":
			return domain.Invalid("resource", "Нарушено ограничение данных")
		}
	}
	return e
}
func (s *Store) Tx(ctx context.Context, fn func(pgx.Tx) error) error {
	tx, e := s.DB.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if e = fn(tx); e != nil {
		return Translate(e)
	}
	return Translate(tx.Commit(ctx))
}
