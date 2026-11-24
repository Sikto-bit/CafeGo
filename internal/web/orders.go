package web

import (
	"net/http"
	"strings"
)

func (a *App) checkout(w http.ResponseWriter, r *http.Request) {
	comment := strings.TrimSpace(r.PostForm.Get("comment"))
	if len([]rune(comment)) > 500 {
		bad(w, "Комментарий: не более 500 символов.")
		return
	}
	tx, err := a.DB.Begin()
	if err != nil {
		a.fail(w, err)
		return
	}
	defer tx.Rollback()
	uid := session(r).User.ID
	items, total, err := cartItems(tx, uid)
	if err != nil {
		a.fail(w, err)
		return
	}
	if len(items) == 0 {
		bad(w, "Корзина пуста. Повторное оформление не создаёт заказ.")
		return
	}
	res, err := tx.Exec("INSERT INTO orders(user_id,total,comment) VALUES(?,?,?)", uid, total, comment)
	if err != nil {
		a.fail(w, err)
		return
	}
	id, err := res.LastInsertId()
	if err != nil {
		a.fail(w, err)
		return
	}
	for _, item := range items {
		if _, err = tx.Exec("INSERT INTO order_items(order_id,dish_id,name,price,quantity) VALUES(?,?,?,?,?)", id, item.ID, item.Name, item.Price, item.Quantity); err != nil {
			a.fail(w, err)
			return
		}
	}
	if _, err = tx.Exec("DELETE FROM cart WHERE user_id=?", uid); err != nil {
		a.fail(w, err)
		return
	}
	if err = tx.Commit(); err != nil {
		a.fail(w, err)
		return
	}
	redirect(w, r, "/orders?created=1")
}
func (a *App) orders(w http.ResponseWriter, r *http.Request)      { a.showOrders(w, r, false) }
func (a *App) adminOrders(w http.ResponseWriter, r *http.Request) { a.showOrders(w, r, true) }
func (a *App) showOrders(w http.ResponseWriter, r *http.Request, all bool) {
	p := Page{Title: "Мои заказы"}
	name := "orders"
	if all {
		p.Title = "Управление заказами"
		name = "adminorders"
	}
	rows, err := a.DB.Query(`SELECT o.id,u.login,o.created_at,o.status,o.total,o.comment FROM orders o JOIN users u ON u.id=o.user_id WHERE (? OR o.user_id=?) ORDER BY o.id DESC`, all, session(r).User.ID)
	if err != nil {
		a.fail(w, err)
		return
	}
	for rows.Next() {
		var o Order
		if err = rows.Scan(&o.ID, &o.Login, &o.Created, &o.Status, &o.Total, &o.Comment); err != nil {
			rows.Close()
			a.fail(w, err)
			return
		}
		p.Orders = append(p.Orders, o)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		a.fail(w, err)
		return
	}
	for i := range p.Orders {
		items, e := a.DB.Query("SELECT name,price,quantity FROM order_items WHERE order_id=? ORDER BY id", p.Orders[i].ID)
		if e != nil {
			a.fail(w, e)
			return
		}
		for items.Next() {
			var item OrderItem
			if e = items.Scan(&item.Name, &item.Price, &item.Quantity); e != nil {
				items.Close()
				a.fail(w, e)
				return
			}
			p.Orders[i].Items = append(p.Orders[i].Items, item)
		}
		e = items.Err()
		items.Close()
		if e != nil {
			a.fail(w, e)
			return
		}
	}
	if r.URL.Query().Get("created") == "1" {
		p.Notice = "Заказ принят! Его статус появится здесь."
	}
	a.render(w, r, name, p)
}
func (a *App) changeStatus(w http.ResponseWriter, r *http.Request) {
	id, err := number(r.PostForm.Get("id"))
	if err != nil {
		bad(w, err.Error())
		return
	}
	next := r.PostForm.Get("status")
	if statusLabel(next) == "" {
		bad(w, "Неизвестный статус заказа.")
		return
	}
	// Completed/cancelled orders are terminal; status changes only move forward.
	res, err := a.DB.Exec(`UPDATE orders SET status=? WHERE id=? AND
  ((status='new' AND ? IN ('cooking','cancelled')) OR
   (status='cooking' AND ? IN ('ready','cancelled')) OR
   (status='ready' AND ? IN ('completed','cancelled')))`, next, id, next, next, next)
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
		bad(w, "Заказ не найден или переход статуса недоступен. Обновите страницу.")
		return
	}
	redirect(w, r, "/admin/orders")
}
