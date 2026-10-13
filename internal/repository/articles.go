package repository

import (
	"blog-api/internal/domain"
	"context"
	"github.com/jackc/pgx/v5"
	"sort"
)

const articleSelect = `SELECT a.id,a.title,a.slug,a.description,a.content,a.status,a.version,a.created_at,a.updated_at,a.published_at,a.views,a.likes,a.comments,a.popularity,
 u.id,u.username,u.display_name,u.avatar_url,u.bio,u.created_at,
 coalesce((SELECT array_agg(t.name ORDER BY t.name) FROM article_tags at JOIN tags t ON t.id=at.tag_id WHERE at.article_id=a.id),'{}'),
 EXISTS(SELECT 1 FROM likes WHERE article_id=a.id AND user_id=$1)
 FROM article_metrics a JOIN users u ON u.id=a.author_id `

func scanArticle(row pgx.Row) (domain.Article, error) {
	var a domain.Article
	e := row.Scan(&a.ID, &a.Title, &a.Slug, &a.Description, &a.Content, &a.Status, &a.Version, &a.CreatedAt, &a.UpdatedAt, &a.PublishedAt, &a.Views, &a.Likes, &a.Comments, &a.Popularity, &a.Author.ID, &a.Author.Username, &a.Author.DisplayName, &a.Author.AvatarURL, &a.Author.Bio, &a.Author.CreatedAt, &a.Tags, &a.Liked)
	return a, Translate(e)
}
func (s *Store) Article(ctx context.Context, id, viewer int64) (domain.Article, error) {
	return scanArticle(s.DB.QueryRow(ctx, articleSelect+" WHERE a.id=$2", viewer, id))
}
func (s *Store) BySlug(ctx context.Context, slug string, viewer int64) (domain.Article, error) {
	return scanArticle(s.DB.QueryRow(ctx, articleSelect+" WHERE a.slug=$2 AND a.status='published'", viewer, slug))
}

const articleWhere = ` WHERE ($2::bigint=0 OR a.author_id=$2) AND (($3 AND ($4='' OR a.status=$4)) OR (NOT $3 AND a.status='published')) AND ($5='' OR EXISTS(SELECT 1 FROM article_tags at JOIN tags t ON t.id=at.tag_id WHERE at.article_id=a.id AND t.name=$5)) AND ($6='' OR to_tsvector('simple',a.title||' '||a.description||' '||a.content) @@ plainto_tsquery('simple',$6)) `

func (s *Store) Articles(ctx context.Context, viewer int64, f domain.Filter) (domain.Page[domain.Article], error) {
	p := domain.Page[domain.Article]{Items: []domain.Article{}, Page: f.Page, PageSize: f.Size}
	args := []any{viewer, f.Author, f.Own, f.Status, f.Tag, f.Search}
	// Count and page share one MVCC snapshot, so total and items agree within a response.
	e := s.Tx(ctx, func(tx pgx.Tx) error {
		if _, e := tx.Exec(ctx, "SET TRANSACTION ISOLATION LEVEL REPEATABLE READ READ ONLY"); e != nil {
			return e
		}
		if e := tx.QueryRow(ctx, "SELECT count(*) FROM article_metrics a"+articleWhere+" AND $1::bigint>=0", args...).Scan(&p.Total); e != nil {
			return e
		}
		order := map[string]string{"published_at": "a.published_at DESC NULLS LAST,a.id DESC", "views": "a.views DESC,a.id DESC", "popularity": "a.popularity DESC,a.id DESC", "created_at": "a.created_at DESC,a.id DESC"}[f.Sort]
		args = append(args, f.Size, (f.Page-1)*f.Size)
		rows, e := tx.Query(ctx, articleSelect+articleWhere+" ORDER BY "+order+" LIMIT $7 OFFSET $8", args...)
		if e != nil {
			return e
		}
		defer rows.Close()
		for rows.Next() {
			a, e := scanArticle(rows)
			if e != nil {
				return e
			}
			p.Items = append(p.Items, a)
		}
		return rows.Err()
	})
	return p, e
}
func tags(ctx context.Context, tx pgx.Tx, id int64, names []string) error {
	names = append([]string(nil), names...)
	sort.Strings(names) // consistent lock order prevents deadlocks across article tag updates
	if _, e := tx.Exec(ctx, "DELETE FROM article_tags WHERE article_id=$1", id); e != nil {
		return e
	}
	for _, name := range names {
		var tid int64
		if e := tx.QueryRow(ctx, "INSERT INTO tags(name) VALUES($1) ON CONFLICT(name) DO UPDATE SET name=EXCLUDED.name RETURNING id", name).Scan(&tid); e != nil {
			return e
		}
		if _, e := tx.Exec(ctx, "INSERT INTO article_tags VALUES($1,$2) ON CONFLICT DO NOTHING", id, tid); e != nil {
			return e
		}
	}
	return nil
}
func (s *Store) CreateArticle(ctx context.Context, author int64, in domain.ArticleInput) (domain.Article, error) {
	var id int64
	e := s.Tx(ctx, func(tx pgx.Tx) error {
		if e := tx.QueryRow(ctx, `INSERT INTO articles(author_id,title,slug,description,content,status,published_at) VALUES($1,$2,$3,$4,$5,$6,CASE WHEN $6='published' THEN now() END) RETURNING id`, author, in.Title, in.Slug, in.Description, in.Content, in.Status).Scan(&id); e != nil {
			return e
		}
		return tags(ctx, tx, id, in.Tags)
	})
	if e != nil {
		return domain.Article{}, e
	}
	return s.Article(ctx, id, author)
}
func (s *Store) UpdateArticle(ctx context.Context, id, version, viewer int64, in domain.ArticleInput) (domain.Article, error) {
	e := s.Tx(ctx, func(tx pgx.Tx) error {
		r, e := tx.Exec(ctx, `UPDATE articles SET title=$3,slug=$4,description=$5,content=$6,status=$7,version=version+1,updated_at=now(),published_at=CASE WHEN $7='draft' THEN NULL WHEN status='draft' THEN now() ELSE published_at END WHERE id=$1 AND version=$2`, id, version, in.Title, in.Slug, in.Description, in.Content, in.Status)
		if e != nil {
			return e
		}
		if r.RowsAffected() == 0 {
			return domain.Fail(412, "version_conflict", "Версия статьи изменилась")
		}
		return tags(ctx, tx, id, in.Tags)
	})
	if e != nil {
		return domain.Article{}, e
	}
	return s.Article(ctx, id, viewer)
}
func (s *Store) DeleteArticle(ctx context.Context, id, version int64) error {
	r, e := s.DB.Exec(ctx, "DELETE FROM articles WHERE id=$1 AND version=$2", id, version)
	if e != nil {
		return Translate(e)
	}
	if r.RowsAffected() == 0 {
		return domain.Fail(412, "version_conflict", "Версия статьи изменилась")
	}
	return nil
}

// All interactions lock the article, serializing against unpublication/deletion.
func (s *Store) PublishedTx(ctx context.Context, id int64, fn func(pgx.Tx, int64) error) error {
	return s.Tx(ctx, func(tx pgx.Tx) error {
		var author int64
		var status string
		if e := tx.QueryRow(ctx, "SELECT author_id,status FROM articles WHERE id=$1 FOR UPDATE", id).Scan(&author, &status); e != nil {
			return e
		}
		if status != "published" {
			return domain.Fail(404, "not_found", "Публикация не найдена")
		}
		return fn(tx, author)
	})
}
func (s *Store) Like(ctx context.Context, id, user int64, on bool) (int64, bool, error) {
	var count int64
	var liked bool
	e := s.PublishedTx(ctx, id, func(tx pgx.Tx, _ int64) error {
		var e error
		if on {
			_, e = tx.Exec(ctx, "INSERT INTO likes(article_id,user_id) VALUES($1,$2) ON CONFLICT DO NOTHING", id, user)
		} else {
			_, e = tx.Exec(ctx, "DELETE FROM likes WHERE article_id=$1 AND user_id=$2", id, user)
		}
		if e != nil {
			return e
		}
		return tx.QueryRow(ctx, "SELECT count(*),coalesce(bool_or(user_id=$2),false) FROM likes WHERE article_id=$1", id, user).Scan(&count, &liked)
	})
	return count, liked, e
}
func (s *Store) Tags(ctx context.Context, f domain.Filter) (domain.Page[string], error) {
	p := domain.Page[string]{Items: []string{}, Page: f.Page, PageSize: f.Size}
	e := s.DB.QueryRow(ctx, "SELECT count(*) FROM tags WHERE EXISTS(SELECT 1 FROM article_tags at JOIN articles a ON a.id=at.article_id WHERE at.tag_id=tags.id AND a.status='published')").Scan(&p.Total)
	if e != nil {
		return p, e
	}
	rows, e := s.DB.Query(ctx, "SELECT name FROM tags WHERE EXISTS(SELECT 1 FROM article_tags at JOIN articles a ON a.id=at.article_id WHERE at.tag_id=tags.id AND a.status='published') ORDER BY name LIMIT $1 OFFSET $2", f.Size, (f.Page-1)*f.Size)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		var name string
		if e := rows.Scan(&name); e != nil {
			return p, e
		}
		p.Items = append(p.Items, name)
	}
	return p, rows.Err()
}
