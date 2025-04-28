package query

import (
	"main/entity"
	"time"
)

func (db *Database) SaveActivity(activity entity.ActivityRecord) error {
	// Formater la date pour faciliter les requêtes par jour
	dateStr := activity.StartTime.Format("2006-01-02")

	_, err := db.Exec(`
        INSERT INTO activities 
        (process_name, start_time, end_time, duration, date, first_launch) 
        VALUES (?, ?, ?, ?, ?, ?)`,
		activity.ProcessName,
		activity.StartTime.Format(time.RFC3339),
		activity.EndTime.Format(time.RFC3339),
		activity.Duration.Seconds(), // Stockage en secondes
		dateStr,
		!db.processExist(activity.ProcessName),
	)
	return err
}

func (db *Database) processExist(name string) bool {
	var exist bool
	query := `SELECT EXISTS(
    SELECT 1 FROM activities 
    WHERE process_name = ?) AS entree_existe;`
	err := db.Get(&exist, query, name)
	if err != nil {
		return false
	}
	return exist
}
