package web

import (
	"bytes"
	"database/sql"
	"embed"
	"fmt"
	"html/template"
	"log"
	"net/http"
	"strconv"
	"strings"
)

//go:embed templates/*.html static/*
var assets embed.FS

type App struct {
	DB        *sql.DB
	templates *template.Template
	Secure    bool
}
type User struct {
	ID          int
	Login, Role string
}
type Session struct {
	User       User
	CSRF, Hash string
}
type Page struct {
	Title, Error, Notice string
	Session              Session
	Categories           []Category
	Dishes               []Dish
	EditCategory         Category
	EditDish             Dish
	Filter               int
}
type Category struct {
	ID   int
	Name string
}
type Dish struct {
	ID, CategoryID              int
	Name, Description, Category string
	Price                       int64
}

func Money(n int64) string { return fmt.Sprintf("%d,%02d ₽", n/100, n%100) }
func New(db *sql.DB, secure bool) *App {
	funcs := template.FuncMap{"money": Money, "initial": func(s string) string {
		r := []rune(s)
		if len(r) == 0 {
			return "C"
		}
		return string(r[0])
	}, "decimal": func(n int64) string { return fmt.Sprintf("%d,%02d", n/100, n%100) }}
	return &App{DB: db, Secure: secure, templates: template.Must(template.New("pages").Funcs(funcs).ParseFS(assets, "templates/*.html"))}
}
func (a *App) render(w http.ResponseWriter, r *http.Request, name string, p Page) {
	p.Session = session(r)
	var b bytes.Buffer
	if err := a.templates.ExecuteTemplate(&b, name, p); err != nil {
		a.fail(w, err)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(b.Bytes())
}
func (a *App) fail(w http.ResponseWriter, err error) {
	log.Printf("request error: %v", err)
	http.Error(w, "Внутренняя ошибка. Попробуйте позже.", 500)
}
func bad(w http.ResponseWriter, message string) { http.Error(w, message, http.StatusBadRequest) }
func redirect(w http.ResponseWriter, r *http.Request, path string) {
	http.Redirect(w, r, path, http.StatusSeeOther)
}
func number(s string) (int, error) {
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 {
		return 0, fmt.Errorf("некорректный идентификатор")
	}
	return n, nil
}
func text(s string, max int) bool { return strings.TrimSpace(s) != "" && len([]rune(s)) <= max }
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.menu)
	mux.HandleFunc("GET /register", a.authPage)
	mux.HandleFunc("POST /register", a.register)
	mux.HandleFunc("GET /login", a.authPage)
	mux.HandleFunc("POST /login", a.login)
	mux.HandleFunc("POST /logout", client(a.logout))
	mux.HandleFunc("GET /admin/categories", admin(a.categoryPage))
	mux.HandleFunc("POST /admin/categories/save", admin(a.saveCategory))
	mux.HandleFunc("POST /admin/categories/delete", admin(a.deleteCategory))
	mux.HandleFunc("GET /admin/dishes", admin(a.dishPage))
	mux.HandleFunc("POST /admin/dishes/save", admin(a.saveDish))
	mux.HandleFunc("POST /admin/dishes/delete", admin(a.deleteDish))
	mux.HandleFunc("GET /admin", admin(func(w http.ResponseWriter, r *http.Request) {
		redirect(w, r, "/admin/categories")
	}))
	mux.HandleFunc("GET /static/style.css", func(w http.ResponseWriter, r *http.Request) {
		b, _ := assets.ReadFile("static/style.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(b)
	})
	return a.security(mux)
}
