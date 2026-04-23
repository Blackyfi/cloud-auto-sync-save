package web

import (
	"context"
	"html/template"
	"log"
	"net/http"

	"github.com/nicolasticot/cass/server/internal/auth"
	"github.com/nicolasticot/cass/server/internal/db"
)

type Handlers struct {
	DB       *db.DB
	Sessions *auth.SessionStore
	Tmpls    map[string]*template.Template
}

type ctxKey string

const userCtxKey ctxKey = "user_id"

func (h *Handlers) requireAuth(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !h.hasAnyUser() {
			http.Redirect(w, r, "/setup", http.StatusSeeOther)
			return
		}
		sess, err := h.Sessions.Read(r)
		if err != nil {
			http.Redirect(w, r, "/login", http.StatusSeeOther)
			return
		}
		ctx := context.WithValue(r.Context(), userCtxKey, sess.UserID)
		next.ServeHTTP(w, r.WithContext(ctx))
	}
}

func (h *Handlers) hasAnyUser() bool {
	var n int
	_ = h.DB.QueryRow(`SELECT COUNT(*) FROM users`).Scan(&n)
	return n > 0
}

func (h *Handlers) render(w http.ResponseWriter, name string, data map[string]any) {
	if data == nil {
		data = map[string]any{}
	}
	t, ok := h.Tmpls[name]
	if !ok {
		http.Error(w, "unknown template", http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := t.ExecuteTemplate(w, "layout", data); err != nil {
		log.Printf("render %s: %v", name, err)
	}
}

func (h *Handlers) LoginForm(w http.ResponseWriter, r *http.Request) {
	if !h.hasAnyUser() {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	h.render(w, "login.html", nil)
}

func (h *Handlers) LoginSubmit(w http.ResponseWriter, r *http.Request) {
	if !h.hasAnyUser() {
		http.Redirect(w, r, "/setup", http.StatusSeeOther)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")

	var id int64
	var hash string
	err := h.DB.QueryRow(
		`SELECT id, password_hash FROM users WHERE username = ?`, username,
	).Scan(&id, &hash)
	if err != nil {
		h.render(w, "login.html", map[string]any{"Error": "invalid credentials"})
		return
	}
	if err := auth.VerifyPassword(password, hash); err != nil {
		h.render(w, "login.html", map[string]any{"Error": "invalid credentials"})
		return
	}
	h.Sessions.SetCookie(w, id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) Logout(w http.ResponseWriter, r *http.Request) {
	h.Sessions.Clear(w)
	http.Redirect(w, r, "/login", http.StatusSeeOther)
}

func (h *Handlers) SetupForm(w http.ResponseWriter, r *http.Request) {
	if h.hasAnyUser() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	h.render(w, "setup.html", nil)
}

func (h *Handlers) SetupSubmit(w http.ResponseWriter, r *http.Request) {
	if h.hasAnyUser() {
		http.Redirect(w, r, "/login", http.StatusSeeOther)
		return
	}
	username := r.FormValue("username")
	password := r.FormValue("password")
	confirm := r.FormValue("confirm")

	if username == "" || len(password) < 8 {
		h.render(w, "setup.html", map[string]any{"Error": "username required, password min 8 chars"})
		return
	}
	if password != confirm {
		h.render(w, "setup.html", map[string]any{"Error": "passwords do not match"})
		return
	}

	hash, err := auth.HashPassword(password)
	if err != nil {
		h.render(w, "setup.html", map[string]any{"Error": "internal error"})
		return
	}
	res, err := h.DB.Exec(
		`INSERT INTO users(username, password_hash) VALUES(?, ?)`, username, hash,
	)
	if err != nil {
		h.render(w, "setup.html", map[string]any{"Error": "could not create user"})
		return
	}
	id, _ := res.LastInsertId()
	h.Sessions.SetCookie(w, id)
	http.Redirect(w, r, "/", http.StatusSeeOther)
}

func (h *Handlers) Dashboard(w http.ResponseWriter, r *http.Request) {
	uid, _ := r.Context().Value(userCtxKey).(int64)
	var username string
	_ = h.DB.QueryRow(`SELECT username FROM users WHERE id = ?`, uid).Scan(&username)
	h.render(w, "dashboard.html", map[string]any{"Username": username})
}
