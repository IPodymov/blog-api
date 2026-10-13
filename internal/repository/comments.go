package repository

import (
	"blog-api/internal/domain"
	"context"
	"github.com/jackc/pgx/v5"
)

const commentSelect = `SELECT c.id,c.article_id,c.parent_id,c.depth,c.content,c.deleted,c.created_at,c.updated_at,u.id,u.username,u.display_name,u.avatar_url,u.bio,u.created_at FROM comments c JOIN users u ON u.id=c.author_id `

func scanComment(row pgx.Row) (domain.Comment, error) {
	var c domain.Comment
	e := row.Scan(&c.ID, &c.ArticleID, &c.ParentID, &c.Depth, &c.Content, &c.Deleted, &c.CreatedAt, &c.UpdatedAt, &c.Author.ID, &c.Author.Username, &c.Author.DisplayName, &c.Author.AvatarURL, &c.Author.Bio, &c.Author.CreatedAt)
	return c, Translate(e)
}
func (s *Store) Comment(ctx context.Context, id int64) (domain.Comment, error) {
	return scanComment(s.DB.QueryRow(ctx, commentSelect+" WHERE c.id=$1", id))
}
func (s *Store) Comments(ctx context.Context, id int64, f domain.Filter) (domain.Page[domain.Comment], error) {
	p := domain.Page[domain.Comment]{Items: []domain.Comment{}, Page: f.Page, PageSize: f.Size}
	e := s.DB.QueryRow(ctx, "SELECT count(*) FROM comments WHERE article_id=$1", id).Scan(&p.Total)
	if e != nil {
		return p, e
	}
	rows, e := s.DB.Query(ctx, commentSelect+" WHERE c.article_id=$1 ORDER BY c.id LIMIT $2 OFFSET $3", id, f.Size, (f.Page-1)*f.Size)
	if e != nil {
		return p, e
	}
	defer rows.Close()
	for rows.Next() {
		c, e := scanComment(rows)
		if e != nil {
			return p, e
		}
		p.Items = append(p.Items, c)
	}
	return p, rows.Err()
}
func (s *Store) CreateComment(ctx context.Context, article, user int64, in domain.CommentInput) (domain.Comment, error) {
	var id int64
	e := s.PublishedTx(ctx, article, func(tx pgx.Tx, _ int64) error {
		depth := 0
		if in.ParentID != nil {
			depth = 1
		}
		return tx.QueryRow(ctx, "INSERT INTO comments(article_id,author_id,parent_id,depth,content) VALUES($1,$2,$3,$4,$5) RETURNING id", article, user, in.ParentID, depth, in.Content).Scan(&id)
	})
	if e != nil {
		return domain.Comment{}, e
	}
	return s.Comment(ctx, id)
}
func (s *Store) UpdateComment(ctx context.Context, id int64, content string) (domain.Comment, error) {
	r, e := s.DB.Exec(ctx, "UPDATE comments SET content=$2,updated_at=now() WHERE id=$1 AND NOT deleted", id, content)
	if e != nil {
		return domain.Comment{}, Translate(e)
	}
	if r.RowsAffected() == 0 {
		return domain.Comment{}, domain.Fail(404, "not_found", "Комментарий не найден")
	}
	return s.Comment(ctx, id)
}
func (s *Store) DeleteComment(ctx context.Context, id int64) error {
	_, e := s.DB.Exec(ctx, "UPDATE comments SET content='',deleted=true,updated_at=now() WHERE id=$1", id)
	return Translate(e)
}
