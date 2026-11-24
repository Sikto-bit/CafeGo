package web

import (
	"errors"

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
