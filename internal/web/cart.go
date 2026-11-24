package web

import (
	"database/sql"
	"net/http"
	"strconv"
)

type queryer interface {
	Query(string, ...any) (*sql.Rows, error)
}

func cartItems(db queryer, uid int) ([]CartItem, int64, error) {
	rows, err := db.Query(`SELECT d.id,d.name,d.description,d.price,c.quantity FROM cart c JOIN dishes d ON d.id=c.dish_id WHERE c.user_id=? ORDER BY d.id`, uid)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()
	var items []CartItem
	var total int64
	for rows.Next() {
		var i CartItem
		if err = rows.Scan(&i.ID, &i.Name, &i.Description, &i.Price, &i.Quantity); err != nil {
			return nil, 0, err
		}
		i.Subtotal = i.Price * int64(i.Quantity)
		total += i.Subtotal
		items = append(items, i)
	}
	return items, total, rows.Err()
}
func (a *App) cartPage(w http.ResponseWriter, r *http.Request) {
	p := Page{Title: "Корзина"}
	var err error
	p.Cart, p.Total, err = cartItems(a.DB, session(r).User.ID)
	if err != nil {
		a.fail(w, err)
		return
	}
	a.render(w, r, "cart", p)
}
func (a *App) addCart(w http.ResponseWriter, r *http.Request) {
	id, err := number(r.PostForm.Get("dish_id"))
	if err != nil {
		bad(w, err.Error())
		return
	}
	res, err := a.DB.Exec(`INSERT INTO cart(user_id,dish_id,quantity) SELECT ?,id,1 FROM dishes WHERE id=?
  ON CONFLICT(user_id,dish_id) DO UPDATE SET quantity=quantity+1 WHERE quantity<99`, session(r).User.ID, id)
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
		bad(w, "Блюдо удалено или уже добавлено 99 порций.")
		return
	}
	redirect(w, r, "/?added=1#menu")
}
func (a *App) updateCart(w http.ResponseWriter, r *http.Request) {
	ids, quantities := r.PostForm["dish_id"], r.PostForm["quantity"]
	if len(ids) == 0 || len(ids) != len(quantities) {
		bad(w, "Некорректная форма корзины.")
		return
	}
	// Validate every row before writing, then save the whole form atomically.
	type change struct{ id, quantity int }
	changes := make([]change, 0, len(ids))
	seen := make(map[int]bool)
	for i, raw := range ids {
		id, err := number(raw)
		if err != nil || seen[id] {
			bad(w, "Некорректный или повторяющийся идентификатор блюда.")
			return
		}
		quantity, err := strconv.Atoi(quantities[i])
		if err != nil || quantity < 1 || quantity > 99 {
			bad(w, "Количество должно быть от 1 до 99.")
			return
		}
		seen[id] = true
		changes = append(changes, change{id, quantity})
	}
	tx, err := a.DB.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	for _, item := range changes {
		res, err := tx.Exec("UPDATE cart SET quantity=? WHERE user_id=? AND dish_id=?", item.quantity, session(r).User.ID, item.id)
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
	}
	if err = tx.Commit(); err != nil {
		a.fail(w, err)
		return
	}
	redirect(w, r, "/cart")
}
func (a *App) removeCart(w http.ResponseWriter, r *http.Request) {
	id, err := number(r.PostForm.Get("dish_id"))
	if err != nil {
		bad(w, err.Error())
		return
	}
	if _, err = a.DB.Exec("DELETE FROM cart WHERE user_id=? AND dish_id=?", session(r).User.ID, id); err != nil {
		a.fail(w, err)
		return
	}
	redirect(w, r, "/cart")
}
