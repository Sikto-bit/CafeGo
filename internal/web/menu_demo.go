package web

import "net/http"

var demoCategories = []Category{
	{ID: 3, Name: "Десерты"},
	{ID: 2, Name: "Напитки"},
	{ID: 1, Name: "Пицца"},
}
var demoDishes = []Dish{
	{ID: 1, CategoryID: 1, Name: "Маргарита", Description: "Томаты, моцарелла, базилик. Итальянская классика.", Category: "Пицца", Price: 39000},
	{ID: 2, CategoryID: 1, Name: "Пепперони", Description: "Пряная пепперони, моцарелла и томатный соус.", Category: "Пицца", Price: 49000},
	{ID: 3, CategoryID: 1, Name: "Четыре сыра", Description: "Моцарелла, пармезан, чеддер и дорблю.", Category: "Пицца", Price: 54000},
	{ID: 4, CategoryID: 2, Name: "Капучино", Description: "Двойной эспрессо и нежная молочная пенка, 250 мл.", Category: "Напитки", Price: 19000},
	{ID: 5, CategoryID: 2, Name: "Ягодный лимонад", Description: "Смородина, мята и немного летнего настроения, 400 мл.", Category: "Напитки", Price: 22000},
	{ID: 6, CategoryID: 3, Name: "Чизкейк", Description: "Сливочный десерт на хрустящей песочной основе.", Category: "Десерты", Price: 29000},
}

func (a *App) menu(w http.ResponseWriter, r *http.Request) {
	page := Page{Title: "Меню", Notice: "Демонстрационное меню. Заказы и аккаунты пока недоступны.", Categories: demoCategories}
	if raw := r.URL.Query().Get("category"); raw != "" {
		filter, err := number(raw)
		if err != nil {
			bad(w, err.Error())
			return
		}
		page.Filter = filter
	}
	for _, dish := range demoDishes {
		if page.Filter == 0 || page.Filter == dish.CategoryID {
			page.Dishes = append(page.Dishes, dish)
		}
	}
	a.render(w, r, "home", page)
}
