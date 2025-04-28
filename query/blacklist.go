package query

// Blacklist operations
func (db *Database) InsertBlacklist(name string) error {
	_, err := db.Exec("INSERT INTO blacklist (name) VALUES (?)", name)
	return err
}

func (db *Database) DeleteFromBlacklist(name string) error {
	_, err := db.Exec("DELETE FROM blacklist WHERE name = ?", name)
	return err
}

func (db *Database) SelectFromBlacklist(name string) (bool, error) {
	var exists bool
	err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM blacklist WHERE name = ?)", name)
	return exists, err
}

func (db *Database) GetAllBlacklisted() ([]string, error) {
	var names []string
	err := db.Select(&names, "SELECT name FROM blacklist")
	return names, err
}
