// Package store opens SQLite and initializes the schema. Prices are integer kopecks.
package store

import (
	"database/sql"
	_ "embed"
	"fmt"

	_ "modernc.org/sqlite"
)

//go:embed schema.sql
var Schema string

func Open(path string) (*sql.DB, error) {
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	// A single connection makes PRAGMA settings consistent and serializes writes.
	db.SetMaxOpenConns(1)
	if _, err = db.Exec("PRAGMA foreign_keys=ON; PRAGMA busy_timeout=5000; PRAGMA journal_mode=WAL;"); err == nil {
		_, err = db.Exec(Schema)
	}
	if err != nil {
		db.Close()
		return nil, fmt.Errorf("initialize database: %w", err)
	}
	return db, nil
}

// Seed is atomic and runs only for a completely empty menu. It never creates users.
func Seed(db *sql.DB) error {
	tx, err := db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err = tx.QueryRow("SELECT (SELECT count(*) FROM categories)+(SELECT count(*) FROM dishes)").Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return tx.Commit()
	}
	for _, name := range []string{"Пицца", "Напитки", "Десерты"} {
		if _, err = tx.Exec("INSERT INTO categories(name) VALUES(?)", name); err != nil {
			return err
		}
	}
	for _, d := range []struct {
		category          int
		name, description string
		price             int
	}{
		{1, "Маргарита", "Томаты, моцарелла, базилик. Итальянская классика.", 39000},
		{1, "Пепперони", "Пряная пепперони, моцарелла и томатный соус.", 49000},
		{1, "Четыре сыра", "Моцарелла, пармезан, чеддер и дорблю.", 54000},
		{2, "Капучино", "Двойной эспрессо и нежная молочная пенка, 250 мл.", 19000},
		{2, "Ягодный лимонад", "Смородина, мята и немного летнего настроения, 400 мл.", 22000},
		{3, "Чизкейк", "Сливочный десерт на хрустящей песочной основе.", 29000},
	} {
		if _, err = tx.Exec("INSERT INTO dishes(category_id,name,description,price) VALUES(?,?,?,?)", d.category, d.name, d.description, d.price); err != nil {
			return err
		}
	}
	return tx.Commit()
}
