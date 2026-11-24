package web

import "net/http"

func client(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if session(r).User.ID == 0 {
			redirect(w, r, "/login")
			return
		}
		next(w, r)
	}
}
func admin(next http.HandlerFunc) http.HandlerFunc {
	return client(func(w http.ResponseWriter, r *http.Request) {
		if session(r).User.Role != "admin" {
			http.Error(w, "Доступ разрешён только администратору.", 403)
			return
		}
		next(w, r)
	})
}
