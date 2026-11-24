package web

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"database/sql"
	"encoding/hex"
	"errors"
	"net/http"
	"regexp"
	"strings"
	"time"

	"golang.org/x/crypto/bcrypt"
)

type sessionKey struct{}

func session(r *http.Request) Session { s, _ := r.Context().Value(sessionKey{}).(Session); return s }
func token() string {
	b := make([]byte, 32)
	if _, e := rand.Read(b); e != nil {
		panic(e)
	}
	return hex.EncodeToString(b)
}
func digest(s string) string { h := sha256.Sum256([]byte(s)); return hex.EncodeToString(h[:]) }
func (a *App) newSession(w http.ResponseWriter, uid any) error {
	raw, csrf := token(), token()
	expiry := time.Now().Add(12 * time.Hour)
	tx, err := a.DB.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if _, err = tx.Exec("DELETE FROM sessions WHERE expires<=?", time.Now().Unix()); err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO sessions(token_hash,user_id,csrf,expires) VALUES(?,?,?,?)", digest(raw), uid, csrf, expiry.Unix()); err != nil {
		return err
	}
	if err = tx.Commit(); err != nil {
		return err
	}
	http.SetCookie(w, &http.Cookie{Name: "cafego_session", Value: raw, Path: "/", HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode, Expires: expiry, MaxAge: 43200})
	return nil
}
func (a *App) security(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Content-Type-Options", "nosniff")
		w.Header().Set("X-Frame-Options", "DENY")
		w.Header().Set("Referrer-Policy", "same-origin")
		w.Header().Set("Content-Security-Policy", "default-src 'self'; style-src 'self'; img-src 'self'; form-action 'self'; frame-ancestors 'none'; base-uri 'none'")
		if r.URL.Path == "/static/style.css" {
			next.ServeHTTP(w, r)
			return
		}
		w.Header().Set("Cache-Control", "no-store")
		r.Body = http.MaxBytesReader(w, r.Body, 64<<10)
		var s Session
		c, err := r.Cookie("cafego_session")
		if err == nil && len(c.Value) == 64 {
			s.Hash = digest(c.Value)
			err = a.DB.QueryRow(`SELECT s.csrf,COALESCE(u.id,0),COALESCE(u.login,''),COALESCE(u.role,'')
    FROM sessions s LEFT JOIN users u ON u.id=s.user_id WHERE s.token_hash=? AND s.expires>?`, s.Hash, time.Now().Unix()).Scan(&s.CSRF, &s.User.ID, &s.User.Login, &s.User.Role)
			if err != nil && !errors.Is(err, sql.ErrNoRows) {
				a.fail(w, err)
				return
			}
		}
		if s.CSRF == "" {
			// An unsafe request must never acquire a new session and bypass CSRF.
			if r.Method != "GET" && r.Method != "HEAD" {
				http.Error(w, "Сессия истекла. Обновите страницу.", 403)
				return
			}
			if err = a.newSession(w, nil); err != nil {
				a.fail(w, err)
				return
			}
			redirect(w, r, r.URL.RequestURI())
			return
		}
		if r.Method != "GET" && r.Method != "HEAD" {
			if err = r.ParseForm(); err != nil {
				bad(w, "Слишком большая или некорректная форма")
				return
			}
			if subtle.ConstantTimeCompare([]byte(r.PostForm.Get("csrf")), []byte(s.CSRF)) != 1 {
				http.Error(w, "Неверный CSRF-токен. Обновите страницу.", 403)
				return
			}
		}
		next.ServeHTTP(w, r.WithContext(context.WithValue(r.Context(), sessionKey{}, s)))
	})
}
func (a *App) authPage(w http.ResponseWriter, r *http.Request) {
	name := "login"
	title := "С возвращением"
	if r.URL.Path == "/register" {
		name = "register"
		title = "Создать аккаунт"
	}
	a.render(w, r, name, Page{Title: title})
}

var validLogin = regexp.MustCompile(`^[a-zA-Z0-9_]{3,32}$`)

func (a *App) register(w http.ResponseWriter, r *http.Request) {
	login, password := strings.TrimSpace(r.PostForm.Get("login")), r.PostForm.Get("password")
	if !validLogin.MatchString(login) || len(password) < 10 || len(password) > 72 {
		a.render(w, r, "register", Page{Title: "Регистрация", Error: "Логин: 3–32 латинские буквы, цифры или _. Пароль: 10–72 байта."})
		return
	}
	hash, err := bcrypt.GenerateFromPassword([]byte(password), bcrypt.DefaultCost)
	if err != nil {
		a.fail(w, err)
		return
	}
	var count int
	if err = a.DB.QueryRow("SELECT count(*) FROM users WHERE login=?", login).Scan(&count); err != nil {
		a.fail(w, err)
		return
	}
	if count > 0 {
		a.render(w, r, "register", Page{Title: "Регистрация", Error: "Этот логин уже занят."})
		return
	}
	result, err := a.DB.Exec("INSERT INTO users(login,password_hash) VALUES(?,?)", login, string(hash))
	if constraint(err) {
		a.render(w, r, "register", Page{Title: "Регистрация", Error: "Этот логин уже занят."})
		return
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	id, err := result.LastInsertId()
	if err != nil {
		a.fail(w, err)
		return
	}
	a.rotate(w, r, id)
}
func (a *App) login(w http.ResponseWriter, r *http.Request) {
	password := r.PostForm.Get("password")
	if len(password) < 10 || len(password) > 72 {
		a.render(w, r, "login", Page{Title: "Вход", Error: "Неверный логин или пароль."})
		return
	}
	var id int
	var hash string
	err := a.DB.QueryRow("SELECT id,password_hash FROM users WHERE login=?", strings.TrimSpace(r.PostForm.Get("login"))).Scan(&id, &hash)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		a.fail(w, err)
		return
	}
	if err != nil || bcrypt.CompareHashAndPassword([]byte(hash), []byte(password)) != nil {
		a.render(w, r, "login", Page{Title: "Вход", Error: "Неверный логин или пароль."})
		return
	}
	a.rotate(w, r, id)
}
func (a *App) rotate(w http.ResponseWriter, r *http.Request, id any) {
	if err := a.newSession(w, id); err != nil {
		a.fail(w, err)
		return
	}
	if _, err := a.DB.Exec("DELETE FROM sessions WHERE token_hash=? OR expires<=?", session(r).Hash, time.Now().Unix()); err != nil {
		a.fail(w, err)
		return
	}
	redirect(w, r, "/")
}
func (a *App) logout(w http.ResponseWriter, r *http.Request) {
	if _, err := a.DB.Exec("DELETE FROM sessions WHERE token_hash=?", session(r).Hash); err != nil {
		a.fail(w, err)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "cafego_session", Path: "/", MaxAge: -1, HttpOnly: true, Secure: a.Secure, SameSite: http.SameSiteLaxMode})
	redirect(w, r, "/")
}
