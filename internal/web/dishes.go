package web

import (
	"database/sql"
	"errors"
	"fmt"
	"net/http"
	"strconv"
	"strings"
)

// ParsePrice accepts decimal rubles without floating point rounding.
func ParsePrice(raw string) (int64, error) {
	raw = strings.ReplaceAll(strings.TrimSpace(raw), ",", ".")
	parts := strings.Split(raw, ".")
	if len(parts) > 2 || len(parts[0]) == 0 || len(parts[0]) > 6 {
		return 0, fmt.Errorf("Укажите цену от 0,01 до 100000 рублей.")
	}
	for _, p := range parts {
		if p == "" {
			return 0, fmt.Errorf("Некорректная цена")
		}
		for _, c := range p {
			if c < '0' || c > '9' {
				return 0, fmt.Errorf("Некорректная цена")
			}
		}
	}
	whole, err := strconv.ParseInt(parts[0], 10, 64)
	if err != nil {
		return 0, err
	}
	var cents int64
	if len(parts) == 2 {
		if len(parts[1]) > 2 {
			return 0, fmt.Errorf("Не более двух знаков после запятой")
		}
		fraction := parts[1]
		if len(fraction) == 1 {
			fraction += "0"
		}
		cents, err = strconv.ParseInt(fraction, 10, 64)
		if err != nil {
			return 0, err
		}
	}
	n := whole*100 + cents
	if n < 1 || n > 10000000 {
		return 0, fmt.Errorf("Цена должна быть от 0,01 до 100000 рублей")
	}
	return n, nil
}
func (a *App) dishes(filter int) ([]Dish, error) {
	rows, err := a.DB.Query(`SELECT d.id,d.category_id,d.name,d.description,d.price,c.name FROM dishes d JOIN categories c ON c.id=d.category_id WHERE (?=0 OR c.id=?) ORDER BY d.id`, filter, filter)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Dish
	for rows.Next() {
		var d Dish
		if err = rows.Scan(&d.ID, &d.CategoryID, &d.Name, &d.Description, &d.Price, &d.Category); err != nil {
			return nil, err
		}
		result = append(result, d)
	}
	return result, rows.Err()
}
func (a *App) menu(w http.ResponseWriter, r *http.Request) {
	p := Page{Title: "Меню"}
	var err error
	if raw := r.URL.Query().Get("category"); raw != "" {
		p.Filter, err = number(raw)
		if err != nil {
			bad(w, err.Error())
			return
		}
	}
	p.Categories, err = a.categories()
	if err != nil {
		a.fail(w, err)
		return
	}
	p.Dishes, err = a.dishes(p.Filter)
	if err != nil {
		a.fail(w, err)
		return
	}
	a.render(w, r, "home", p)
}
func (a *App) dishPage(w http.ResponseWriter, r *http.Request) { a.showDishes(w, r, "") }
func (a *App) showDishes(w http.ResponseWriter, r *http.Request, message string) {
	p := Page{Title: "Блюда", Error: message}
	var err error
	p.Categories, err = a.categories()
	if err != nil {
		a.fail(w, err)
		return
	}
	p.Dishes, err = a.dishes(0)
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
		err = a.DB.QueryRow("SELECT id,category_id,name,description,price FROM dishes WHERE id=?", id).Scan(&p.EditDish.ID, &p.EditDish.CategoryID, &p.EditDish.Name, &p.EditDish.Description, &p.EditDish.Price)
		if errors.Is(err, sql.ErrNoRows) {
			http.NotFound(w, r)
			return
		}
		if err != nil {
			a.fail(w, err)
			return
		}
	}
	a.render(w, r, "dishes", p)
}
func (a *App) saveDish(w http.ResponseWriter, r *http.Request) {
	name, description := strings.TrimSpace(r.PostForm.Get("name")), strings.TrimSpace(r.PostForm.Get("description"))
	price, err := ParsePrice(r.PostForm.Get("price"))
	if err != nil {
		a.showDishes(w, r, err.Error())
		return
	}
	category, err := number(r.PostForm.Get("category_id"))
	if err != nil || !text(name, 120) || len([]rune(description)) > 1000 {
		a.showDishes(w, r, "Проверьте категорию, название (1–120 символов) и описание (до 1000 символов).")
		return
	}
	var res sql.Result
	if raw := r.PostForm.Get("id"); raw != "" {
		id, e := number(raw)
		if e != nil {
			bad(w, e.Error())
			return
		}
		res, err = a.DB.Exec("UPDATE dishes SET category_id=?,name=?,description=?,price=? WHERE id=?", category, name, description, price, id)
	} else {
		res, err = a.DB.Exec("INSERT INTO dishes(category_id,name,description,price) VALUES(?,?,?,?)", category, name, description, price)
	}
	if constraint(err) {
		a.showDishes(w, r, "Выберите существующую категорию.")
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
	redirect(w, r, "/admin/dishes")
}
func (a *App) deleteDish(w http.ResponseWriter, r *http.Request) {
	id, err := number(r.PostForm.Get("id"))
	if err != nil {
		bad(w, err.Error())
		return
	}
	res, err := a.DB.Exec("DELETE FROM dishes WHERE id=?", id)
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
	redirect(w, r, "/admin/dishes")
}
