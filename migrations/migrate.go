package migrations

import (
	"context"
	"crypto/sha256"
	"embed"
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"sort"
	"strings"
)

//go:embed *.sql
var Files embed.FS

func Apply(ctx context.Context, p *pgxpool.Pool) error {
	tx, e := p.Begin(ctx)
	if e != nil {
		return e
	}
	defer tx.Rollback(ctx)
	if _, e = tx.Exec(ctx, "SELECT pg_advisory_xact_lock(781224)"); e != nil {
		return e
	}
	if _, e = tx.Exec(ctx, "CREATE TABLE IF NOT EXISTS schema_migrations(name text PRIMARY KEY,checksum text NOT NULL,applied_at timestamptz NOT NULL DEFAULT now())"); e != nil {
		return e
	}
	entries, e := Files.ReadDir(".")
	if e != nil {
		return e
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Name() < entries[j].Name() })
	for _, f := range entries {
		if !strings.HasSuffix(f.Name(), ".sql") {
			continue
		}
		b, e := Files.ReadFile(f.Name())
		if e != nil {
			return e
		}
		h := sha256.Sum256(b)
		hash := hex.EncodeToString(h[:])
		var old string
		e = tx.QueryRow(ctx, "SELECT checksum FROM schema_migrations WHERE name=$1", f.Name()).Scan(&old)
		if e == nil {
			if old != hash {
				return fmt.Errorf("migration checksum mismatch: %s", f.Name())
			}
			continue
		}
		if !errors.Is(e, pgx.ErrNoRows) {
			return e
		}
		if _, e = tx.Exec(ctx, string(b)); e != nil {
			return e
		}
		if _, e = tx.Exec(ctx, "INSERT INTO schema_migrations(name,checksum) VALUES($1,$2)", f.Name(), hash); e != nil {
			return e
		}
	}
	return tx.Commit(ctx)
}
