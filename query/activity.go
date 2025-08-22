package query

import (
	"main/entity"
	"time"
)

// SaveActivity persists an activity. If the session spans across midnight boundaries,
// it will be split into multiple day-bounded segments so that each segment contributes
// to the correct calendar day statistics.
func (db *Database) SaveActivity(activity entity.ActivityRecord) error {
	start := activity.StartTime
	end := activity.EndTime
	if end.Before(start) || end.Equal(start) {
		// Nothing to save
		return nil
	}

	first := !db.processExist(activity.ProcessName)

	currentStart := start
	for currentStart.Before(end) {
		// Compute start of next day (local time) = today at 00:00 + 24h
		year, month, day := currentStart.Date()
		loc := currentStart.Location()
		nextDayStart := time.Date(year, month, day, 0, 0, 0, 0, loc).Add(24 * time.Hour)

		segmentEnd := end
		if end.After(nextDayStart) {
			segmentEnd = nextDayStart
		}

		// Guard: avoid zero/negative durations
		if segmentEnd.After(currentStart) {
			dateStr := currentStart.Format("2006-01-02")
			_, err := db.Exec(`
        INSERT INTO activities 
        (process_name, start_time, end_time, duration, date, first_launch) 
        VALUES (?, ?, ?, ?, ?, ?)`,
				activity.ProcessName,
				currentStart.Format(time.RFC3339),
				segmentEnd.Format(time.RFC3339),
				segmentEnd.Sub(currentStart).Seconds(), // seconds
				dateStr,
				first,
			)
			if err != nil {
				return err
			}
			// Only the very first inserted segment should carry first_launch=true
			first = false
		}

		// Advance to next segment start: exactly the previous segment end
		currentStart = segmentEnd
	}

	return nil
}

// DeleteActivity deletes a single activity row identified by process_name and exact start/end times (RFC3339)
func (db *Database) DeleteActivity(processName, startTime, endTime string) error {
	_, err := db.Exec(`DELETE FROM activities WHERE process_name = ? AND start_time = ? AND end_time = ?`, processName, startTime, endTime)
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
