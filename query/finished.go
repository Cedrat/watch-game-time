package query

// operations for finished games

func (db *Database) InsertFinished(name string) error {
	// record finish date so we can know if it happened during a selected period
	_, err := db.Exec("INSERT INTO finished_games (name, finished_at) VALUES (?, date('now'))", name)
	return err
}

func (db *Database) UpsertFinishedAt(name, date string) error {
	_, err := db.Exec(`INSERT INTO finished_games (name, finished_at) VALUES (?, ?) 
	ON CONFLICT(name) DO UPDATE SET finished_at=excluded.finished_at`, name, date)
	return err
}

func (db *Database) DeleteFinished(name string) error {
	_, err := db.Exec("DELETE FROM finished_games WHERE name = ?", name)
	return err
}

func (db *Database) IsFinished(name string) (bool, error) {
	var exists bool
	err := db.Get(&exists, "SELECT EXISTS(SELECT 1 FROM finished_games WHERE name = ?)", name)
	return exists, err
}
