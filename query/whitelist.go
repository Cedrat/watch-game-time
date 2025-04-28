package query

func (db *Database) InsertWhitelist(name string) error {
	_, err := db.Exec("INSERT INTO whitelist (name) VALUES (?)", name)
	return err
}

func (db *Database) DeleteFromWhitelist(name string) error {
	_, err := db.Exec("DELETE FROM whitelist WHERE name = ?", name)
	return err
}

func (db *Database) SelectFromWhitelist(name string) (bool, error) {
	var exists bool
	err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM whitelist WHERE name = ?)", name)
	return exists, err
}

func (db *Database) GetAllWhitelisted() ([]string, error) {
	var names []string
	err := db.Select(&names, "SELECT name FROM whitelist")
	return names, err
}
