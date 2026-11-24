package store

import (
	"path/filepath"
	"testing"
)

func TestSchemaSeedAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "cafego.db")
	db, err := Open(path)
	if err != nil {
		t.Fatal(err)
	}
	for range 2 {
		if err = Seed(db); err != nil {
			t.Fatal(err)
		}
	}
	var n int
	if err = db.QueryRow("SELECT count(*) FROM dishes").Scan(&n); err != nil || n != 6 {
		t.Fatal(n, err)
	}
	if _, err = db.Exec("INSERT INTO dishes(category_id,name,description,price) VALUES(999,'x','',100)"); err == nil {
		t.Fatal("foreign keys disabled")
	}
	if _, err = db.Exec("DELETE FROM categories WHERE id=1"); err == nil {
		t.Fatal("occupied category deleted")
	}
	if err = db.Close(); err != nil {
		t.Fatal(err)
	}
	db, err = Open(path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err = db.QueryRow("SELECT count(*) FROM dishes").Scan(&n); err != nil || n != 6 {
		t.Fatal("persistence", n, err)
	}
}
