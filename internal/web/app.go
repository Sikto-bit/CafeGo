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
)

//go:embed templates/*.html static/*
var assets embed.FS

type App struct {
	DB        *sql.DB
	templates *template.Template
}
type Page struct {
	Title, Error, Notice string
	Categories           []Category
	Dishes               []Dish
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
func New(db *sql.DB) *App {
	funcs := template.FuncMap{"money": Money, "initial": func(s string) string {
		r := []rune(s)
		if len(r) == 0 {
			return "C"
		}
		return string(r[0])
	}}
	return &App{DB: db, templates: template.Must(template.New("pages").Funcs(funcs).ParseFS(assets, "templates/*.html"))}
}
func (a *App) render(w http.ResponseWriter, r *http.Request, name string, p Page) {
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
func number(s string) (int, error) {
	n, e := strconv.Atoi(s)
	if e != nil || n < 1 {
		return 0, fmt.Errorf("некорректный идентификатор")
	}
	return n, nil
}
func (a *App) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /{$}", a.menu)
	mux.HandleFunc("GET /static/style.css", func(w http.ResponseWriter, r *http.Request) {
		b, _ := assets.ReadFile("static/style.css")
		w.Header().Set("Content-Type", "text/css; charset=utf-8")
		w.Write(b)
	})
	return mux
}
