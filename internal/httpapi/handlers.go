package httpapi

import (
	"blog-api/internal/domain"
	"blog-api/internal/service"
	"github.com/go-chi/chi/v5"
	"net/http"
	"strconv"
)

type credentials struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type registration struct {
	Username string `json:"username"`
	Email    string `json:"email"`
	Password string `json:"password"`
}
type refreshRequest struct {
	Refresh string `json:"refresh_token"`
}

func (a *API) register(w http.ResponseWriter, r *http.Request) {
	var b registration
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Register(r.Context(), b.Username, b.Email, b.Password)
	a.respond(w, r, v, e, 201)
}
func (a *API) login(w http.ResponseWriter, r *http.Request) {
	var b credentials
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Login(r.Context(), b.Email, b.Password)
	a.respond(w, r, v, e, 200)
}
func (a *API) refresh(w http.ResponseWriter, r *http.Request) {
	var b refreshRequest
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Refresh(r.Context(), b.Refresh)
	a.respond(w, r, v, e, 200)
}
func (a *API) logout(w http.ResponseWriter, r *http.Request) {
	var b refreshRequest
	e := decode(r, &b, false)
	if e == nil {
		if len(b.Refresh) != 43 {
			e = domain.Invalid("refresh_token", "Ожидается токен длиной 43")
		} else {
			e = a.Service.Logout(r.Context(), b.Refresh)
		}
	}
	a.respond(w, r, nil, e, 204)
}
func (a *API) profile(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "user_id")
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.PublicProfile(r.Context(), n)
	a.respond(w, r, v, e, 200)
}
func (a *API) me(w http.ResponseWriter, r *http.Request) {
	v, e := a.Service.Me(r.Context(), actor(r))
	a.respond(w, r, v, e, 200)
}
func (a *API) patchMe(w http.ResponseWriter, r *http.Request) {
	var b domain.ProfilePatch
	e := decode(r, &b, true)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.UpdateProfile(r.Context(), actor(r), b)
	a.respond(w, r, v, e, 200)
}
func (a *API) listArticles(w http.ResponseWriter, r *http.Request, own, user bool) {
	f, e := filter(r, false)
	if e == nil && user {
		f.Author, e = id(r, "user_id")
		if e == nil {
			_, e = a.Service.PublicProfile(r.Context(), f.Author)
		}
	}
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	f.Own = own
	if !own && f.Status != "" {
		a.respond(w, r, nil, domain.Invalid("status", "Фильтр только для /me/articles"), 0)
		return
	}
	v, e := a.Service.Articles(r.Context(), actor(r), f)
	a.respond(w, r, v, e, 200)
}
func (a *API) articles(w http.ResponseWriter, r *http.Request)     { a.listArticles(w, r, false, false) }
func (a *API) myArticles(w http.ResponseWriter, r *http.Request)   { a.listArticles(w, r, true, false) }
func (a *API) userArticles(w http.ResponseWriter, r *http.Request) { a.listArticles(w, r, false, true) }
func (a *API) articleResponse(w http.ResponseWriter, r *http.Request, v domain.Article, e error, status int) {
	if e == nil {
		w.Header().Set("ETag", `"`+strconv.FormatInt(v.Version, 10)+`"`)
	}
	a.respond(w, r, v, e, status)
}
func (a *API) article(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Article(r.Context(), actor(r), n)
	a.articleResponse(w, r, v, e, 200)
}
func (a *API) slug(w http.ResponseWriter, r *http.Request) {
	v, e := a.Service.Slug(r.Context(), actor(r), chi.URLParam(r, "slug"))
	a.articleResponse(w, r, v, e, 200)
}
func (a *API) createArticle(w http.ResponseWriter, r *http.Request) {
	var b domain.ArticleInput
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.CreateArticle(r.Context(), actor(r), b)
	if e == nil {
		w.Header().Set("Location", "/api/v1/articles/"+strconv.FormatInt(v.ID, 10))
	}
	a.articleResponse(w, r, v, e, 201)
}
func (a *API) patchArticle(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	var ver int64
	if e == nil {
		ver, e = version(r)
	}
	var b domain.ArticlePatch
	if e == nil {
		e = decode(r, &b, true)
	}
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.UpdateArticle(r.Context(), actor(r), n, ver, b)
	a.articleResponse(w, r, v, e, 200)
}
func (a *API) deleteArticle(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	var ver int64
	if e == nil {
		ver, e = version(r)
	}
	if e == nil {
		e = a.Service.DeleteArticle(r.Context(), actor(r), n, ver)
	}
	a.respond(w, r, nil, e, 204)
}
func (a *API) view(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	ac := actor(r)
	visitor := "user:" + strconv.FormatInt(ac.ID, 10)
	if ac.ID == 0 {
		cookie, _ := r.Cookie("blog_visitor")
		raw := ""
		if cookie != nil {
			raw = cookie.Value
		}
		guest, ok := a.Service.VerifyVisitor(raw)
		if !ok {
			guest = service.Random()
			http.SetCookie(w, &http.Cookie{Name: "blog_visitor", Value: a.Service.SignVisitor(guest), Path: "/api/v1", MaxAge: 31536000, HttpOnly: true, Secure: a.Config.SecureCookie, SameSite: http.SameSiteLaxMode})
		}
		visitor = "guest:" + guest
	}
	counted, e := a.Service.View(r.Context(), ac, n, visitor)
	a.respond(w, r, map[string]bool{"counted": counted}, e, 200)
}
func (a *API) statistics(w http.ResponseWriter, r *http.Request) {
	p, e := a.period(r)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Statistics(r.Context(), actor(r), p)
	a.respond(w, r, v, e, 200)
}
func (a *API) statisticsArticles(w http.ResponseWriter, r *http.Request) {
	p, e := a.period(r)
	f, fe := filter(r, true)
	if e == nil {
		e = fe
	}
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.StatisticsArticles(r.Context(), actor(r), f, p)
	a.respond(w, r, v, e, 200)
}
func (a *API) articleStatistics(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	p, pe := a.period(r)
	if e == nil {
		e = pe
	}
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.ArticleStatistics(r.Context(), actor(r), n, p)
	a.respond(w, r, v, e, 200)
}
func (a *API) comments(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	f, fe := filter(r, false)
	if e == nil {
		e = fe
	}
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Comments(r.Context(), actor(r), n, f)
	a.respond(w, r, v, e, 200)
}
func (a *API) createComment(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	var b domain.CommentInput
	if e == nil {
		e = decode(r, &b, false)
	}
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.CreateComment(r.Context(), actor(r), n, b)
	a.respond(w, r, v, e, 201)
}
func (a *API) patchComment(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "comment_id")
	var b struct {
		Content string `json:"content"`
	}
	if e == nil {
		e = decode(r, &b, true)
	}
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.UpdateComment(r.Context(), actor(r), n, b.Content)
	a.respond(w, r, v, e, 200)
}
func (a *API) deleteComment(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "comment_id")
	if e == nil {
		e = a.Service.DeleteComment(r.Context(), actor(r), n)
	}
	a.respond(w, r, nil, e, 204)
}
func (a *API) setLike(w http.ResponseWriter, r *http.Request, on bool) {
	n, e := id(r, "article_id")
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	count, liked, e := a.Service.Like(r.Context(), actor(r), n, on)
	a.respond(w, r, map[string]any{"likes": count, "liked_by_me": liked}, e, 200)
}
func (a *API) like(w http.ResponseWriter, r *http.Request)   { a.setLike(w, r, true) }
func (a *API) unlike(w http.ResponseWriter, r *http.Request) { a.setLike(w, r, false) }
func (a *API) tags(w http.ResponseWriter, r *http.Request) {
	f, e := filter(r, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Repo.Tags(r.Context(), f)
	a.respond(w, r, v, e, 200)
}
