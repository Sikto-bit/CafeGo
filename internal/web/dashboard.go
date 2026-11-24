package web

import "net/http"

func (a *App) dashboard(w http.ResponseWriter, r *http.Request) {
	p := Page{Title: "Панель управления"}
	err := a.DB.QueryRow(`SELECT (SELECT count(*) FROM categories),(SELECT count(*) FROM dishes),
  (SELECT count(*) FROM orders WHERE status IN ('new','cooking','ready')),(SELECT count(*) FROM users WHERE role='client')`).Scan(&p.Counts[0], &p.Counts[1], &p.Counts[2], &p.Counts[3])
	if err != nil {
		a.fail(w, err)
		return
	}
	a.render(w, r, "dashboard", p)
}
