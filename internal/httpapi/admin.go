package httpapi

import (
	"blog-api/internal/domain"
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/go-chi/chi/v5"
)

// adminRoutes is the platform admin API for the CRM: it is not bound to X-Site-Key,
// the site is part of the path instead.
func (a *API) adminRoutes(r chi.Router) {
	r.Post("/auth/login", a.adminLogin)
	r.Group(func(r chi.Router) {
		r.Use(a.requireAdmin)
		r.Post("/auth/logout", a.adminLogout)
		r.Get("/me", a.adminMe)
		r.Get("/sites", a.adminSites)
		r.Post("/sites", a.adminCreateSite)
		r.Get("/sites/{site_key}", a.adminSite)
		r.Patch("/sites/{site_key}", a.adminPatchSite)
		r.Get("/sites/{site_key}/users", a.adminUsers)
		r.Post("/sites/{site_key}/users", a.adminCreateUser)
		r.Get("/sites/{site_key}/users/{user_id}", a.adminUser)
		r.Patch("/sites/{site_key}/users/{user_id}", a.adminPatchUser)
		r.Delete("/sites/{site_key}/users/{user_id}", a.adminDeleteUser)
		r.Get("/sites/{site_key}/articles", a.adminArticles)
		r.Post("/sites/{site_key}/articles", a.adminCreateArticle)
		r.Get("/sites/{site_key}/articles/{article_id}", a.adminArticle)
		r.Patch("/sites/{site_key}/articles/{article_id}", a.adminPatchArticle)
		r.Delete("/sites/{site_key}/articles/{article_id}", a.adminDeleteArticle)
		r.Get("/sites/{site_key}/invites", a.adminInvites)
		r.Post("/sites/{site_key}/invites", a.adminCreateInvite)
		r.Delete("/sites/{site_key}/invites/{invite_id}", a.adminDeleteInvite)
	})
}
func (a *API) requireAdmin(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		h := r.Header.Get("Authorization")
		if !strings.HasPrefix(h, "Bearer ") {
			a.respond(w, r, nil, domain.Fail(401, "authentication_required", "Требуется токен администратора"), 0)
			return
		}
		ad, e := a.Service.AuthenticateAdmin(r.Context(), strings.TrimPrefix(h, "Bearer "))
		if e != nil {
			a.respond(w, r, nil, e, 0)
			return
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), adminKey, ad)))
	})
}
func admin(r *http.Request) domain.Admin {
	v, _ := r.Context().Value(adminKey).(domain.Admin)
	return v
}
func siteKey(r *http.Request) string { return chi.URLParam(r, "site_key") }

func (a *API) adminLogin(w http.ResponseWriter, r *http.Request) {
	var b credentials
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.AdminLogin(r.Context(), b.Email, b.Password)
	a.respond(w, r, v, e, 200)
}
func (a *API) adminLogout(w http.ResponseWriter, r *http.Request) {
	e := a.Service.AdminLogout(r.Context(), strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer "))
	a.respond(w, r, nil, e, 204)
}
func (a *API) adminMe(w http.ResponseWriter, r *http.Request) { a.respond(w, r, admin(r), nil, 200) }
func (a *API) adminSites(w http.ResponseWriter, r *http.Request) {
	f, e := filter(r, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Sites(r.Context(), f)
	a.respond(w, r, v, e, 200)
}
func (a *API) adminSite(w http.ResponseWriter, r *http.Request) {
	v, e := a.Service.SiteInfo(r.Context(), siteKey(r))
	a.respond(w, r, v, e, 200)
}
func (a *API) adminCreateSite(w http.ResponseWriter, r *http.Request) {
	var b domain.SiteInput
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.CreateSite(r.Context(), b)
	if e == nil {
		w.Header().Set("Location", "/api/admin/v1/sites/"+v.Key)
	}
	a.respond(w, r, v, e, 201)
}
func (a *API) adminPatchSite(w http.ResponseWriter, r *http.Request) {
	var b domain.SitePatch
	e := decode(r, &b, true)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.UpdateSite(r.Context(), siteKey(r), b)
	a.respond(w, r, v, e, 200)
}
func (a *API) adminUsers(w http.ResponseWriter, r *http.Request) {
	f, e := filter(r, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	f.Role = r.URL.Query().Get("role")
	v, e := a.Service.Users(r.Context(), siteKey(r), f)
	a.respond(w, r, v, e, 200)
}
func (a *API) adminUser(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "user_id")
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.User(r.Context(), siteKey(r), n)
	a.respond(w, r, v, e, 200)
}
func (a *API) adminCreateUser(w http.ResponseWriter, r *http.Request) {
	var b domain.UserInput
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.CreateUser(r.Context(), siteKey(r), b)
	if e == nil {
		w.Header().Set("Location", "/api/admin/v1/sites/"+siteKey(r)+"/users/"+strconv.FormatInt(v.ID, 10))
	}
	a.respond(w, r, v, e, 201)
}
func (a *API) adminPatchUser(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "user_id")
	var b domain.UserPatch
	if e == nil {
		e = decode(r, &b, true)
	}
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.UpdateUser(r.Context(), siteKey(r), n, b)
	a.respond(w, r, v, e, 200)
}
func (a *API) adminDeleteUser(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "user_id")
	if e == nil {
		e = a.Service.DeleteUser(r.Context(), siteKey(r), n)
	}
	a.respond(w, r, nil, e, 204)
}
func (a *API) adminArticles(w http.ResponseWriter, r *http.Request) {
	f, e := filter(r, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.AdminArticles(r.Context(), siteKey(r), f)
	a.respond(w, r, v, e, 200)
}
func (a *API) adminArticle(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.AdminArticle(r.Context(), siteKey(r), n)
	a.articleResponse(w, r, v, e, 200)
}
func (a *API) adminCreateArticle(w http.ResponseWriter, r *http.Request) {
	var b domain.AdminArticleInput
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.AdminCreateArticle(r.Context(), siteKey(r), b)
	if e == nil {
		w.Header().Set("Location", "/api/admin/v1/sites/"+siteKey(r)+"/articles/"+strconv.FormatInt(v.ID, 10))
	}
	a.articleResponse(w, r, v, e, 201)
}
func (a *API) adminPatchArticle(w http.ResponseWriter, r *http.Request) {
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
	v, e := a.Service.AdminUpdateArticle(r.Context(), siteKey(r), n, ver, b)
	a.articleResponse(w, r, v, e, 200)
}
func (a *API) adminDeleteArticle(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "article_id")
	var ver int64
	if e == nil {
		ver, e = version(r)
	}
	if e == nil {
		e = a.Service.AdminDeleteArticle(r.Context(), siteKey(r), n, ver)
	}
	a.respond(w, r, nil, e, 204)
}
func (a *API) adminInvites(w http.ResponseWriter, r *http.Request) {
	f, e := filter(r, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.Invites(r.Context(), siteKey(r), f)
	a.respond(w, r, v, e, 200)
}
func (a *API) adminCreateInvite(w http.ResponseWriter, r *http.Request) {
	var b domain.InviteInput
	e := decode(r, &b, false)
	if e != nil {
		a.respond(w, r, nil, e, 0)
		return
	}
	v, e := a.Service.CreateInvite(r.Context(), admin(r), siteKey(r), b)
	a.respond(w, r, v, e, 201)
}
func (a *API) adminDeleteInvite(w http.ResponseWriter, r *http.Request) {
	n, e := id(r, "invite_id")
	if e == nil {
		e = a.Service.DeleteInvite(r.Context(), siteKey(r), n)
	}
	a.respond(w, r, nil, e, 204)
}
