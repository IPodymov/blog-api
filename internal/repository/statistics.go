package repository

import (
	"blog-api/internal/domain"
	"context"
	"github.com/jackc/pgx/v5"
	"time"
)

func (s *Store) View(ctx context.Context, id, user int64, admin bool, visitor string, interval time.Duration) (bool, error) {
	counted := false
	e := s.PublishedTx(ctx, id, func(tx pgx.Tx, author int64) error {
		if user == author || admin {
			return nil
		}
		// clock_timestamp is evaluated after obtaining the article lock; now() could predate a queued request.
		var at time.Time
		if e := tx.QueryRow(ctx, "SELECT clock_timestamp()").Scan(&at); e != nil {
			return e
		}
		r, e := tx.Exec(ctx, `INSERT INTO view_dedup(article_id,visitor_hash,last_seen) VALUES($1,$2,$3) ON CONFLICT(article_id,visitor_hash) DO UPDATE SET last_seen=EXCLUDED.last_seen WHERE view_dedup.last_seen <= $3::timestamptz - ($4::double precision * interval '1 second')`, id, visitor, at, interval.Seconds())
		if e != nil {
			return e
		}
		if r.RowsAffected() == 0 {
			return nil
		}
		_, e = tx.Exec(ctx, `INSERT INTO article_daily_views(article_id,day,views) VALUES($1,($2::timestamptz AT TIME ZONE 'UTC')::date,1) ON CONFLICT(article_id,day) DO UPDATE SET views=article_daily_views.views+1`, id, at)
		counted = e == nil
		return e
	})
	return counted, e
}
func (s *Store) Daily(ctx context.Context, author, article int64, p domain.Period) ([]domain.Day, int64, error) {
	rows, e := s.DB.Query(ctx, `SELECT to_char(d.day,'YYYY-MM-DD'),coalesce(v.views,0)::bigint FROM generate_series($3::date,$4::date,interval '1 day') d(day) LEFT JOIN (SELECT v.day,sum(v.views) views FROM article_daily_views v JOIN articles a ON a.id=v.article_id WHERE ($1::bigint=0 OR a.author_id=$1) AND ($2::bigint=0 OR a.id=$2) AND v.day BETWEEN $3::date AND $4::date GROUP BY v.day) v ON v.day=d.day::date ORDER BY d.day`, author, article, p.FromDate(), p.ToDate())
	if e != nil {
		return nil, 0, e
	}
	defer rows.Close()
	days := []domain.Day{}
	var sum int64
	for rows.Next() {
		var d domain.Day
		if e := rows.Scan(&d.Date, &d.Views); e != nil {
			return nil, 0, e
		}
		days = append(days, d)
		sum += d.Views
	}
	return days, sum, rows.Err()
}
func (s *Store) ArticleStats(ctx context.Context, author, viewer int64, f domain.Filter, p domain.Period) (domain.Page[domain.ArticleStat], error) {
	out := domain.Page[domain.ArticleStat]{Items: []domain.ArticleStat{}, Page: f.Page, PageSize: f.Size}
	e := s.DB.QueryRow(ctx, "SELECT count(*) FROM articles WHERE author_id=$1", author).Scan(&out.Total)
	if e != nil {
		return out, e
	}
	order := map[string]string{"views_period": "coalesce(pv.n,0) DESC,a.id DESC", "views": "a.views DESC,a.id DESC", "likes": "a.likes DESC,a.id DESC", "comments": "a.comments DESC,a.id DESC", "published_at": "a.published_at DESC NULLS LAST,a.id DESC"}[f.Sort]
	rows, e := s.DB.Query(ctx, articleSelect+` LEFT JOIN (SELECT article_id,sum(views) n FROM article_daily_views WHERE day BETWEEN $3::date AND $4::date GROUP BY article_id) pv ON pv.article_id=a.id WHERE a.author_id=$2 ORDER BY `+order+` LIMIT $5 OFFSET $6`, viewer, author, p.FromDate(), p.ToDate(), f.Size, (f.Page-1)*f.Size)
	if e != nil {
		return out, e
	}
	articles := []domain.Article{}
	for rows.Next() {
		a, e := scanArticle(rows)
		if e != nil {
			rows.Close()
			return out, e
		}
		articles = append(articles, a)
	}
	e = rows.Err()
	rows.Close()
	if e != nil {
		return out, e
	}
	ids := []int64{}
	for _, a := range articles {
		ids = append(ids, a.ID)
	}
	// A single second query for period metrics, independent of page length; no N+1.
	counts := map[int64]int64{}
	rows, e = s.DB.Query(ctx, "SELECT article_id,sum(views)::bigint FROM article_daily_views WHERE article_id=ANY($1::bigint[]) AND day BETWEEN $2::date AND $3::date GROUP BY article_id", ids, p.FromDate(), p.ToDate())
	if e != nil {
		return out, e
	}
	defer rows.Close()
	for rows.Next() {
		var id, n int64
		if e := rows.Scan(&id, &n); e != nil {
			return out, e
		}
		counts[id] = n
	}
	for _, a := range articles {
		out.Items = append(out.Items, domain.ArticleStat{Article: a, ViewsPeriod: counts[a.ID]})
	}
	return out, rows.Err()
}
