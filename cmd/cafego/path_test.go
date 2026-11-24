package main

import (
	"path/filepath"
	"testing"
)

func TestPortableDatabasePath(t *testing.T) {
	packageDir := t.TempDir()
	executable := filepath.Join(packageDir, "cafego.exe")
	got, err := databasePath(executable, "")
	want := filepath.Join(packageDir, "data", "cafego-local.db")
	if err != nil || got != want {
		t.Fatalf("default database: %q, %v; want %q", got, err, want)
	}
	// An explicitly supplied absolute path is still supported for development.
	override := filepath.Join(t.TempDir(), "custom.db")
	got, err = databasePath(executable, override)
	if err != nil || got != override {
		t.Fatalf("explicit database: %q, %v", got, err)
	}
}
