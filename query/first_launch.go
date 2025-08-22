package query

// operations for first launch overrides

func (db *Database) UpsertFirstLaunchOverride(name, date string) error {
	_, err := db.Exec(`INSERT INTO first_launch_override (name, first_date) VALUES (?, ?) 
	ON CONFLICT(name) DO UPDATE SET first_date=excluded.first_date`, name, date)
	return err
}
