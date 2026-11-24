package main

import "path/filepath"

// The portable package always owns the database next to its executable.
// Only an explicit -db flag may select a database outside this directory.
func databasePath(executable, override string) (string, error) {
	if override != "" {
		return filepath.Abs(override)
	}
	return filepath.Abs(filepath.Join(filepath.Dir(executable), "data", "cafego-local.db"))
}
