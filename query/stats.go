package query

import (
	"fmt"
	"time"
)

type SummaryItem struct {
	Name     string  `db:"name" json:"name"`
	Seconds  float64 `db:"seconds" json:"seconds"`
}

// GameMeta represents flags for games within a period
type GameMeta struct {
	Name             string `db:"name" json:"name"`
	IsNew            bool   `db:"is_new" json:"is_new"`
	FinishedInPeriod bool   `db:"finished_in_period" json:"finished_in_period"`
}

// KnownProc summarizes a known (display) process with flags
type KnownProc struct {
	Name        string `db:"name" json:"name"`
	Original    string `db:"original" json:"original"`
	Sessions    int    `db:"sessions" json:"sessions"`
	Whitelisted bool   `db:"whitelisted" json:"whitelisted"`
	Blacklisted bool   `db:"blacklisted" json:"blacklisted"`
}

// SessionItem represents a single recorded session (non-aggregated)
type SessionItem struct {
	Name        string  `db:"name" json:"name"`
	Original    string  `db:"original" json:"original"`
	Date        string  `db:"date" json:"date"`
	Start       string  `db:"start_time" json:"start_time"`
	End         string  `db:"end_time" json:"end_time"`
	Seconds     float64 `db:"duration" json:"seconds"`
	Finished    bool    `db:"finished" json:"finished"`
	Blacklisted bool    `db:"blacklisted" json:"blacklisted"`
}

// GetSummaryBetween returns aggregated durations per (renamed) process between inclusive dates (YYYY-MM-DD)
func (db *Database) GetSummaryBetween(startDate, endDate string) ([]SummaryItem, error) {
	items := []SummaryItem{}
	q := `
	WITH base AS (
	  SELECT a.*, substr(a.start_time,1,10) AS sdate
	  FROM activities a
	)
	SELECT COALESCE(r.display_name, b.process_name) AS name,
	       SUM(b.duration) as seconds
	FROM base b
	LEFT JOIN rename_map r ON r.original_name = b.process_name
	WHERE b.sdate >= ? AND b.sdate <= ?
	  AND NOT EXISTS (
	    SELECT 1 FROM blacklist bx
	    WHERE bx.name = b.process_name OR bx.name = COALESCE(r.display_name, b.process_name)
	  )
	GROUP BY COALESCE(r.display_name, b.process_name)
	ORDER BY seconds DESC`
	err := db.Select(&items, q, startDate, endDate)
	return items, err
}

// GetHistory returns successive sessions with flags for finished/blacklisted
func (db *Database) GetHistory(hideBlacklisted bool) ([]SessionItem, error) {
	items := []SessionItem{}
	// base query selecting flags
	q := `
	WITH base AS (
	  SELECT a.*, substr(a.start_time,1,10) AS sdate
	  FROM activities a
	)
	SELECT
	  COALESCE(r.display_name, b.process_name) AS name,
	  b.process_name AS original,
	  b.sdate AS date,
	  b.start_time AS start_time,
	  b.end_time AS end_time,
	  b.duration AS duration,
	  CASE WHEN fg.name IS NOT NULL THEN 1 ELSE 0 END AS finished,
	  CASE WHEN bl1.name IS NOT NULL OR bl2.name IS NOT NULL THEN 1 ELSE 0 END AS blacklisted
	FROM base b
	LEFT JOIN rename_map r ON r.original_name = b.process_name
	LEFT JOIN finished_games fg ON fg.name = COALESCE(r.display_name, b.process_name)
	LEFT JOIN blacklist bl1 ON bl1.name = b.process_name
	LEFT JOIN blacklist bl2 ON bl2.name = COALESCE(r.display_name, b.process_name)
	`
	if hideBlacklisted {
		// Exclude rows that are blacklisted either by original or display name.
		q += `
	WHERE NOT EXISTS (
	  SELECT 1 FROM blacklist bx 
	  WHERE bx.name = b.process_name OR bx.name = COALESCE(r.display_name, b.process_name)
	)`
	}
	q += `
	ORDER BY b.start_time ASC`
	if err := db.Select(&items, q); err != nil {
		return nil, err
	}

	// Merge contiguous segments for the same display name when they are back-to-back in time
	if len(items) == 0 {
		return items, nil
	}

	merged := make([]SessionItem, 0, len(items))
	cur := items[0]
	for i := 1; i < len(items); i++ {
		next := items[i]
		// Only merge if same display name and same blacklist visibility status
		if cur.Name == next.Name && cur.Blacklisted == next.Blacklisted {
			// Check exact continuity: cur.End == next.Start
			if cur.End == next.Start {
				// Extend current segment
				cur.End = next.End
				cur.Seconds += next.Seconds
				// finished is true if any part is finished
				if next.Finished {
					cur.Finished = true
				}
				continue
			}
		}
		// Flush current and start new
		merged = append(merged, cur)
		cur = next
	}
	// Add the last accumulated segment
	merged = append(merged, cur)

	// Return in descending order by start_time as before
	for i, j := 0, len(merged)-1; i < j; i, j = i+1, j-1 {
		merged[i], merged[j] = merged[j], merged[i]
	}
	return merged, nil
}

// UpsertRename sets the display name for an original process_name
func (db *Database) UpsertRename(original, display string) error {
	_, err := db.Exec(`INSERT INTO rename_map (original_name, display_name) VALUES (?, ?) 
	ON CONFLICT(original_name) DO UPDATE SET display_name=excluded.display_name`, original, display)
	return err
}

// RenameSmart supports renaming when `from` is either an original_name or an existing display_name.
// - If there are rows having display_name = from, we update them to display_name = to.
// - Otherwise, we upsert a mapping original_name = from -> display_name = to.
func (db *Database) RenameSmart(from, to string) error {
	res, err := db.Exec(`UPDATE rename_map SET display_name = ? WHERE display_name = ?`, to, from)
	if err != nil { return err }
	if res != nil {
		if n, _ := res.RowsAffected(); n > 0 { return nil }
	}
	return db.UpsertRename(from, to)
}

// GetOriginalsForDisplay returns original process names mapped to a given display name
func (db *Database) GetOriginalsForDisplay(display string) ([]string, error) {
	var names []string
	err := db.Select(&names, `SELECT original_name FROM rename_map WHERE display_name = ?`, display)
	return names, err
}

// SeriesRow is a single bucketed record used for bar chart
type SeriesRow struct {
	Bucket  string  `db:"bucket" json:"bucket"`
	Name    string  `db:"name" json:"name"`
	Seconds float64 `db:"seconds" json:"seconds"`
}

// GetSeries returns bucketed rows between start and end.
// period determines bucket granularity: for "year", use monthly (YYYY-MM) or weekly (YYYY-MM-DD Monday) depending on by; otherwise by day (YYYY-MM-DD).
func (db *Database) GetSeries(period, startDate, endDate, by string) ([]SeriesRow, error) {
	rows := []SeriesRow{}
	var q string
	if period == "year" {
		if by == "week" {
			q = `
			WITH base AS (
			  SELECT a.*, substr(a.start_time,1,10) AS sdate
			  FROM activities a
			)
			SELECT date(b.sdate,'weekday 1','-7 days') AS bucket,
			       COALESCE(r.display_name, b.process_name) AS name,
			       SUM(b.duration) AS seconds
			FROM base b
			LEFT JOIN rename_map r ON r.original_name = b.process_name
			WHERE b.sdate >= ? AND b.sdate <= ?
			  AND NOT EXISTS (
			    SELECT 1 FROM blacklist bx
			    WHERE bx.name = b.process_name OR bx.name = COALESCE(r.display_name, b.process_name)
			  )
			GROUP BY date(b.sdate,'weekday 1','-7 days'), COALESCE(r.display_name, b.process_name)
			ORDER BY bucket`
		} else {
			q = `
			WITH base AS (
			  SELECT a.*, substr(a.start_time,1,10) AS sdate
			  FROM activities a
			)
			SELECT substr(b.sdate,1,7) AS bucket,
			       COALESCE(r.display_name, b.process_name) AS name,
			       SUM(b.duration) AS seconds
			FROM base b
			LEFT JOIN rename_map r ON r.original_name = b.process_name
			WHERE b.sdate >= ? AND b.sdate <= ?
			  AND NOT EXISTS (
			    SELECT 1 FROM blacklist bx
			    WHERE bx.name = b.process_name OR bx.name = COALESCE(r.display_name, b.process_name)
			  )
			GROUP BY substr(b.sdate,1,7), COALESCE(r.display_name, b.process_name)
			ORDER BY bucket`
		}
	} else {
		q = `
		WITH base AS (
		  SELECT a.*, substr(a.start_time,1,10) AS sdate
		  FROM activities a
		)
		SELECT b.sdate AS bucket,
		       COALESCE(r.display_name, b.process_name) AS name,
		       SUM(b.duration) AS seconds
		FROM base b
		LEFT JOIN rename_map r ON r.original_name = b.process_name
		WHERE b.sdate >= ? AND b.sdate <= ?
		  AND NOT EXISTS (
		    SELECT 1 FROM blacklist bx
		    WHERE bx.name = b.process_name OR bx.name = COALESCE(r.display_name, b.process_name)
		  )
		GROUP BY b.sdate, COALESCE(r.display_name, b.process_name)
		ORDER BY bucket`
	}
	if err := db.Select(&rows, q, startDate, endDate); err != nil {
		return nil, fmt.Errorf("GetSeries: %w", err)
	}
	return rows, nil
}

// GetAllKnownProcesses returns distinct display names seen in activities with flags and session counts
func (db *Database) GetAllKnownProcesses() ([]KnownProc, error) {
	rows := []KnownProc{}
	q := `
	SELECT
	  COALESCE(r.display_name, a.process_name) AS name,
	  MIN(a.process_name) AS original,
	  COUNT(*) AS sessions,
	  CASE WHEN wl1.name IS NOT NULL OR wl2.name IS NOT NULL THEN 1 ELSE 0 END AS whitelisted,
	  CASE WHEN bl1.name IS NOT NULL OR bl2.name IS NOT NULL THEN 1 ELSE 0 END AS blacklisted
	FROM activities a
	LEFT JOIN rename_map r ON r.original_name = a.process_name
	LEFT JOIN whitelist wl1 ON wl1.name = a.process_name
	LEFT JOIN whitelist wl2 ON wl2.name = COALESCE(r.display_name, a.process_name)
	LEFT JOIN blacklist bl1 ON bl1.name = a.process_name
	LEFT JOIN blacklist bl2 ON bl2.name = COALESCE(r.display_name, a.process_name)
	GROUP BY COALESCE(r.display_name, a.process_name)
	ORDER BY name COLLATE NOCASE`
	if err := db.Select(&rows, q); err != nil {
		return nil, fmt.Errorf("GetAllKnownProcesses: %w", err)
	}
	return rows, nil
}

// Period helpers
func PeriodRange(period string, now time.Time) (string, string) {
	nowDate := now.Format("2006-01-02")
	var start time.Time
	switch period {
	case "week":
		start = now.AddDate(0, 0, -6) // include today + previous 6 days
	case "month":
		start = now.AddDate(0, -1, 1) // approximately last month inclusive
	case "year":
		start = now.AddDate(-1, 0, 1)
	default:
		start = now.AddDate(0, 0, -6)
	}
	return start.Format("2006-01-02"), nowDate
}


// GetGamesMetaBetween returns list of games played in [start,end] with flags
func (db *Database) GetGamesMetaBetween(startDate, endDate string) ([]GameMeta, error) {
	rows := []GameMeta{}
	q := `
	WITH base AS (
	    SELECT a.*, substr(a.start_time,1,10) AS sdate
	    FROM activities a
	), games_in_period AS (
	    SELECT DISTINCT COALESCE(r.display_name, b.process_name) AS name
	    FROM base b
	    LEFT JOIN rename_map r ON r.original_name = b.process_name
	    WHERE b.sdate >= ? AND b.sdate <= ?
	      AND NOT EXISTS (
	        SELECT 1 FROM blacklist bx
	        WHERE bx.name = b.process_name OR bx.name = COALESCE(r.display_name, b.process_name)
	      )
	), first_ever AS (
	    SELECT COALESCE(r.display_name, b.process_name) AS name,
	           MIN(b.sdate) AS first_date
	    FROM base b
	    LEFT JOIN rename_map r ON r.original_name = b.process_name
	    GROUP BY COALESCE(r.display_name, b.process_name)
	)
	SELECT gip.name AS name,
	       CASE WHEN COALESCE(ov.first_date, fe.first_date) >= ? AND COALESCE(ov.first_date, fe.first_date) <= ? THEN 1 ELSE 0 END AS is_new,
	       CASE WHEN fg.finished_at IS NOT NULL AND fg.finished_at >= ? AND fg.finished_at <= ? THEN 1 ELSE 0 END AS finished_in_period
	FROM games_in_period gip
	LEFT JOIN first_ever fe ON fe.name = gip.name
	LEFT JOIN first_launch_override ov ON ov.name = gip.name
	LEFT JOIN finished_games fg ON fg.name = gip.name
	ORDER BY gip.name COLLATE NOCASE
	`
	if err := db.Select(&rows, q, startDate, endDate, startDate, endDate, startDate, endDate); err != nil {
		return nil, fmt.Errorf("GetGamesMetaBetween: %w", err)
	}
	return rows, nil
}


// CalendarDay aggregates per-day totals and lists for heatmap
type CalendarDay struct {
	Date         string  `db:"date" json:"date"`
	Seconds      float64 `db:"seconds" json:"seconds"`
	NewCSV       string  `db:"new_csv" json:"-"`
	FinishedCSV  string  `db:"finished_csv" json:"-"`
}

// GetCalendarDays returns, for each day in [startDate,endDate],
// the total seconds played (excluding blacklisted) and CSV lists of
// display names that are first played that day (new) and games finished that day.
func (db *Database) GetCalendarDays(startDate, endDate string) ([]CalendarDay, error) {
	rows := []CalendarDay{}
	q := `
	WITH base AS (
	    SELECT a.*, substr(a.start_time,1,10) AS sdate
	    FROM activities a
	), daily AS (
	    SELECT b.sdate AS day, SUM(b.duration) AS seconds
	    FROM base b
	    LEFT JOIN rename_map r ON r.original_name = b.process_name
	    WHERE b.sdate >= ? AND b.sdate <= ?
	      AND NOT EXISTS (
	        SELECT 1 FROM blacklist bx
	        WHERE bx.name = b.process_name OR bx.name = COALESCE(r.display_name, b.process_name)
	      )
	    GROUP BY b.sdate
	), first_ever AS (
	    SELECT COALESCE(r.display_name, b.process_name) AS name,
	           MIN(b.sdate) AS first_date
	    FROM base b
	    LEFT JOIN rename_map r ON r.original_name = b.process_name
	    GROUP BY COALESCE(r.display_name, b.process_name)
	), newd AS (
	    SELECT COALESCE(ov.first_date, fe.first_date) AS day,
	           GROUP_CONCAT(fe.name, '||') AS new_csv
	    FROM first_ever fe
	    LEFT JOIN first_launch_override ov ON ov.name = fe.name
	    LEFT JOIN blacklist bl ON bl.name = fe.name
	    WHERE COALESCE(ov.first_date, fe.first_date) >= ? AND COALESCE(ov.first_date, fe.first_date) <= ? AND bl.name IS NULL
	    GROUP BY COALESCE(ov.first_date, fe.first_date)
	), fin AS (
	    SELECT fg.finished_at AS day,
	           GROUP_CONCAT(fg.name, '||') AS finished_csv
	    FROM finished_games fg
	    LEFT JOIN blacklist bl ON bl.name = fg.name
	    WHERE fg.finished_at >= ? AND fg.finished_at <= ? AND bl.name IS NULL
	    GROUP BY fg.finished_at
	), days AS (
	    SELECT day FROM daily
	    UNION
	    SELECT day FROM newd
	    UNION
	    SELECT day FROM fin
	)
	SELECT d.day AS date,
	       COALESCE(daily.seconds, 0) AS seconds,
	       COALESCE(newd.new_csv, '') AS new_csv,
	       COALESCE(fin.finished_csv, '') AS finished_csv
	FROM days d
	LEFT JOIN daily ON daily.day = d.day
	LEFT JOIN newd ON newd.day = d.day
	LEFT JOIN fin ON fin.day = d.day
	ORDER BY d.day`
	if err := db.Select(&rows, q, startDate, endDate, startDate, endDate, startDate, endDate); err != nil {
		return nil, fmt.Errorf("GetCalendarDays: %w", err)
	}
	return rows, nil
}

// DayIntervalRow represents raw activity intervals for a specific date with display names
type DayIntervalRow struct {
	Name      string `db:"name" json:"name"`
	StartTime string `db:"start_time" json:"start_time"`
	EndTime   string `db:"end_time" json:"end_time"`
}

// GetIntervalsForDate returns all activity intervals for the given date (by activities.date),
// with display names applied and excluding blacklisted items. Intervals will be clipped by the caller if needed.
func (db *Database) GetIntervalsForDate(date string) ([]DayIntervalRow, error) {
	rows := []DayIntervalRow{}
	q := `
	WITH base AS (
	  SELECT a.*, substr(a.start_time,1,10) AS sdate
	  FROM activities a
	)
	SELECT COALESCE(r.display_name, b.process_name) AS name,
	       b.start_time AS start_time,
	       b.end_time AS end_time
	FROM base b
	LEFT JOIN rename_map r ON r.original_name = b.process_name
	WHERE b.sdate = ?
	  AND NOT EXISTS (
	    SELECT 1 FROM blacklist bx
	    WHERE bx.name = b.process_name OR bx.name = COALESCE(r.display_name, b.process_name)
	  )
	ORDER BY b.start_time`
	if err := db.Select(&rows, q, date); err != nil {
		return nil, fmt.Errorf("GetIntervalsForDate: %w", err)
	}
	return rows, nil
}
