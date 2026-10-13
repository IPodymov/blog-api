package maintenance

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

// Run deletes expired operational data atomically and is safe to repeat.
func Run(ctx context.Context, pool *pgxpool.Pool, viewInterval time.Duration) error {
	tx, err := pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	for _, query := range []struct {
		sql  string
		args []any
	}{
		{"DELETE FROM view_dedup WHERE last_seen < clock_timestamp() - ($1::double precision * interval '1 second')", []any{viewInterval.Seconds()}},
		{"DELETE FROM access_sessions WHERE expires_at <= now()", nil},
		{"DELETE FROM session_families WHERE expires_at <= now()", nil},
	} {
		if _, err := tx.Exec(ctx, query.sql, query.args...); err != nil {
			return err
		}
	}
	return tx.Commit(ctx)
}
