package service

import (
	"blog-api/internal/domain"
	"strings"
	"testing"
)

func TestArticleValidation(t *testing.T) {
	valid := domain.ArticleInput{Title: "Заголовок", Slug: "hello-world", Content: "# Content", Status: "draft", Tags: []string{"go"}}
	if e := ValidateArticle(valid); e != nil {
		t.Fatal(e)
	}
	for _, mutate := range []func(*domain.ArticleInput){func(v *domain.ArticleInput) { v.Title = " " }, func(v *domain.ArticleInput) { v.Content = "" }, func(v *domain.ArticleInput) { v.Slug = "../escape" }, func(v *domain.ArticleInput) { v.Status = "private" }, func(v *domain.ArticleInput) { v.Tags = []string{"go", "go"} }, func(v *domain.ArticleInput) { v.Title = strings.Repeat("я", 201) }, func(v *domain.ArticleInput) { v.Tags = []string{""} }} {
		v := valid
		mutate(&v)
		if ValidateArticle(v) == nil {
			t.Errorf("invalid accepted: %+v", v)
		}
	}
}
func TestVisitorSignature(t *testing.T) {
	s := &Service{VisitorSecret: strings.Repeat("s", 32)}
	id := Random()
	cookie := s.SignVisitor(id)
	got, ok := s.VerifyVisitor(cookie)
	if !ok || got != id {
		t.Fatal("signature rejected")
	}
	for _, v := range []string{cookie + "x", id, "arbitrary-id", s.SignVisitor("short")} {
		if _, ok := s.VerifyVisitor(v); ok {
			t.Fatalf("forged cookie accepted: %s", v)
		}
	}
	if s.VisitorHash("user:1") == s.VisitorHash("guest:1") {
		t.Fatal("identity domains collide")
	}
}
func TestCanonicalDraft(t *testing.T) {
	s := &Service{PublicURL: "https://blog.example/"}
	draft := s.Canonical(domain.Article{Status: "draft", Slug: "secret"})
	if draft.CanonicalURL != nil {
		t.Fatal("draft link leaked")
	}
	published := s.Canonical(domain.Article{Status: "published", Slug: "post"})
	if *published.CanonicalURL != "https://blog.example/articles/post" {
		t.Fatal("incorrect URL")
	}
}
