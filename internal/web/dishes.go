package web

import "net/http"

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
