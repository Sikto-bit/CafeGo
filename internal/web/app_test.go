package web

import (
	"cafego/internal/store"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"testing"
)

func TestMenuAndStylesheet(t *testing.T) {
	db, err := store.Open(filepath.Join(t.TempDir(), "menu.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = store.Seed(db); err != nil {
		t.Fatal(err)
	}
	if _, err = db.Exec("INSERT INTO dishes(category_id,name,description,price) VALUES(1,'Проверка SQLite','',19900)"); err != nil {
		t.Fatal(err)
	}
	handler := New(db).Handler()
	checks := []struct {
		path            string
		status          int
		present, absent string
	}{
		{"/", 200, "Проверка SQLite", ""},
		{"/?category=2", 200, "Капучино", "Маргарита"},
		{"/?category=999", 200, "В этой категории", "Капучино"},
		{"/?category=bad", 400, "", ""},
		{"/static/style.css", 200, ".menu-grid", ""},
		{"/login", 404, "", ""},
		{"/cart", 404, "", ""},
	}
	for _, check := range checks {
		response := httptest.NewRecorder()
		handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, check.path, nil))
		body := response.Body.String()
		if response.Code != check.status || !strings.Contains(body, check.present) ||
			(check.absent != "" && strings.Contains(body, check.absent)) {
			t.Errorf("%s: status=%d, expected content=%q, excluded content=%q",
				check.path, response.Code, check.present, check.absent)
		}
	}
}
