package domain

import (
	"fmt"
	"time"
)

type Error struct {
	Status        int
	Code, Message string
	Details       map[string]string
}

func (e *Error) Error() string                { return e.Message }
func Fail(status int, code, msg string) error { return &Error{status, code, msg, nil} }
func Invalid(field, msg string) error {
	return &Error{422, "validation_failed", "Некорректные данные", map[string]string{field: msg}}
}

type Actor struct {
	ID   int64
	Role string
}

func (a Actor) CanManage(owner int64) bool { return a.ID != 0 && (a.ID == owner || a.Role == "admin") }
func (a Actor) CanRead(owner int64, status string) bool {
	return status == "published" || a.CanManage(owner)
}

type User struct {
	ID          int64     `json:"id"`
	Username    string    `json:"username"`
	DisplayName string    `json:"display_name"`
	AvatarURL   string    `json:"avatar_url"`
	Bio         string    `json:"bio"`
	CreatedAt   time.Time `json:"created_at"`
}
type OwnUser struct {
	User
	Email string `json:"email"`
	Role  string `json:"role"`
}
type Metrics struct {
	Published int64 `json:"published_articles"`
	Drafts    int64 `json:"draft_articles"`
	Views     int64 `json:"views_all_time"`
	Likes     int64 `json:"likes_current"`
	Comments  int64 `json:"comments_current"`
}
type PublicMetrics struct {
	Published int64 `json:"published_articles"`
	Views     int64 `json:"views_all_time"`
	Likes     int64 `json:"likes_current"`
	Comments  int64 `json:"comments_current"`
}
type Profile struct {
	User
	Statistics PublicMetrics `json:"statistics"`
}
type Article struct {
	ID           int64      `json:"id"`
	Author       User       `json:"author"`
	Title        string     `json:"title"`
	Slug         string     `json:"slug"`
	Description  string     `json:"description"`
	Content      string     `json:"content"`
	Tags         []string   `json:"tags"`
	Status       string     `json:"status"`
	Version      int64      `json:"version"`
	CreatedAt    time.Time  `json:"created_at"`
	UpdatedAt    time.Time  `json:"updated_at"`
	PublishedAt  *time.Time `json:"published_at"`
	Views        int64      `json:"views"`
	Likes        int64      `json:"likes"`
	Comments     int64      `json:"comments"`
	Popularity   int64      `json:"popularity"`
	Liked        bool       `json:"liked_by_me"`
	CanonicalURL *string    `json:"canonical_url"`
}
type ArticleInput struct {
	Title       string   `json:"title"`
	Slug        string   `json:"slug"`
	Description string   `json:"description"`
	Content     string   `json:"content"`
	Tags        []string `json:"tags"`
	Status      string   `json:"status"`
}
type ArticlePatch struct {
	Title       *string   `json:"title"`
	Slug        *string   `json:"slug"`
	Description *string   `json:"description"`
	Content     *string   `json:"content"`
	Tags        *[]string `json:"tags"`
	Status      *string   `json:"status"`
}
type ProfilePatch struct {
	DisplayName *string `json:"display_name"`
	AvatarURL   *string `json:"avatar_url"`
	Bio         *string `json:"bio"`
}
type Comment struct {
	ID        int64     `json:"id"`
	ArticleID int64     `json:"article_id"`
	Author    User      `json:"author"`
	ParentID  *int64    `json:"parent_id"`
	Depth     int       `json:"depth"`
	Content   string    `json:"content"`
	Deleted   bool      `json:"deleted"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}
type CommentInput struct {
	Content  string `json:"content"`
	ParentID *int64 `json:"parent_id"`
}
type Tokens struct {
	Access    string `json:"access_token"`
	Refresh   string `json:"refresh_token"`
	TokenType string `json:"token_type"`
	ExpiresIn int64  `json:"expires_in"`
}
type Page[T any] struct {
	Items    []T   `json:"items"`
	Page     int   `json:"page"`
	PageSize int   `json:"page_size"`
	Total    int64 `json:"total"`
}
type Filter struct {
	Page, Size                int
	Author                    int64
	Status, Tag, Search, Sort string
	Own                       bool
	From, To                  time.Time
}
type Day struct {
	Date  string `json:"date"`
	Views int64  `json:"views"`
}
type Period struct{ From, To time.Time }

func (p Period) FromDate() string { return p.From.Format(time.DateOnly) }
func (p Period) ToDate() string   { return p.To.Format(time.DateOnly) }
func ParsePeriod(from, to string, now time.Time, max int) (Period, error) {
	today := time.Date(now.UTC().Year(), now.UTC().Month(), now.UTC().Day(), 0, 0, 0, 0, time.UTC)
	p := Period{today.AddDate(0, 0, -29), today}
	var e error
	if from != "" {
		p.From, e = time.Parse(time.DateOnly, from)
		if e != nil {
			return p, Invalid("date_from", "ожидается YYYY-MM-DD")
		}
	}
	if to != "" {
		p.To, e = time.Parse(time.DateOnly, to)
		if e != nil {
			return p, Invalid("date_to", "ожидается YYYY-MM-DD")
		}
	}
	if p.From.After(p.To) || p.To.After(today) || p.To.Sub(p.From)/(24*time.Hour) >= time.Duration(max) {
		return p, Invalid("period", fmt.Sprintf("диапазон 1..%d дней, без будущих дат", max))
	}
	return p, nil
}

type ArticleStat struct {
	Article     Article `json:"article"`
	ViewsPeriod int64   `json:"views_period"`
}
type Statistics struct {
	Metrics
	DateFrom    string        `json:"date_from"`
	DateTo      string        `json:"date_to"`
	Timezone    string        `json:"timezone"`
	ViewsPeriod int64         `json:"views_period"`
	Daily       []Day         `json:"daily"`
	Top         []ArticleStat `json:"top_articles"`
}
type ArticleStatistics struct {
	ArticleID   int64  `json:"article_id"`
	ViewsAll    int64  `json:"views_all_time"`
	ViewsPeriod int64  `json:"views_period"`
	Likes       int64  `json:"likes_current"`
	Comments    int64  `json:"comments_current"`
	DateFrom    string `json:"date_from"`
	DateTo      string `json:"date_to"`
	Timezone    string `json:"timezone"`
	Daily       []Day  `json:"daily"`
}
