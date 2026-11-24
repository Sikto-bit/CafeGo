package web

import (
	"database/sql"
	"errors"
	"net/http"
	"strings"

	"modernc.org/sqlite"
)

func constraint(err error) bool { var e *sqlite.Error; return errors.As(err, &e) && e.Code()&255 == 19 }
func (a *App) categories() ([]Category, error) {
	rows, err := a.DB.Query("SELECT id,name FROM categories ORDER BY name")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Category
	for rows.Next() {
		var c Category
		if err = rows.Scan(&c.ID, &c.Name); err != nil {
			return nil, err
		}
		result = append(result, c)
	}
	return result, rows.Err()
}
func (a *App) categoryPage(w http.ResponseWriter, r *http.Request) { a.showCategories(w, r, "") }
func (a *App) showCategories(w http.ResponseWriter, r *http.Request, message string) {
	p := Page{Title: "Категории", Error: message}
	var err error
	p.Categories, err = a.categories()
	if err != nil {
		a.fail(w, err)
		return
	}
	if raw := r.URL.Query().Get("edit"); raw != "" {
		id, e := number(raw)
		if e != nil {
			bad(w, e.Error())
			return
		}
		err = a.DB.QueryRow("SELECT id,name FROM categories WHERE id=?", id).Scan(&p.EditCategory.ID, &p.EditCategory.Name)
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			a.fail(w, err)
			return
		}
	}
	a.render(w, r, "categories", p)
}
func (a *App) saveCategory(w http.ResponseWriter, r *http.Request) {
	name := strings.TrimSpace(r.PostForm.Get("name"))
	if !text(name, 80) {
		a.showCategories(w, r, "Название должно содержать от 1 до 80 символов.")
		return
	}
	var err error
	if raw := r.PostForm.Get("id"); raw != "" {
		id, e := number(raw)
		if e != nil {
			bad(w, e.Error())
			return
		}
		var res sql.Result
		res, err = a.DB.Exec("UPDATE categories SET name=? WHERE id=?", name, id)
		if err == nil {
			n, e := res.RowsAffected()
			if e != nil {
				a.fail(w, e)
				return
			}
			if n == 0 {
				http.NotFound(w, r)
				return
			}
		}
	} else {
		_, err = a.DB.Exec("INSERT INTO categories(name) VALUES(?)", name)
	}
	if constraint(err) {
		a.showCategories(w, r, "Категория с таким названием уже существует.")
		return
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	redirect(w, r, "/admin/categories")
}
func (a *App) deleteCategory(w http.ResponseWriter, r *http.Request) {
	id, err := number(r.PostForm.Get("id"))
	if err != nil {
		bad(w, err.Error())
		return
	}
	res, err := a.DB.Exec("DELETE FROM categories WHERE id=?", id)
	if constraint(err) {
		a.showCategories(w, r, "Сначала перенесите или удалите блюда этой категории.")
		return
	}
	if err != nil {
		a.fail(w, err)
		return
	}
	n, err := res.RowsAffected()
	if err != nil {
		a.fail(w, err)
		return
	}
	if n == 0 {
		http.NotFound(w, r)
		return
	}
	redirect(w, r, "/admin/categories")
}
