package query

import (
	"fmt"
	"sort"
	"time"
)

type SummaryItem struct {
	Name    string  `db:"name" json:"name"`
	Seconds float64 `db:"seconds" json:"seconds"`
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

// CleanupProcessNames removes the .exe extension from any process names in the rename_map
// that haven't been manually renamed to something else yet, ensures entries exist for names without extensions,
// and populates rename_map for all known processes that haven't been mapped yet.
func (db *Database) CleanupProcessNames() error {
	// 1. Initialiser la rename_map pour tous les processus connus qui n'y sont pas encore
	_, err := db.Exec(`
		INSERT OR IGNORE INTO rename_map (original_name, display_name)
		SELECT DISTINCT process_name, process_name
		FROM activities
	`)
	if err != nil {
		return err
	}

	// 2. Enlever le .exe du display_name quand il est identique au nom original
	_, err = db.Exec(`
		UPDATE rename_map
		SET display_name = SUBSTR(original_name, 1, LENGTH(original_name) - 4)
		WHERE original_name LIKE '%.exe'
		  AND display_name = original_name
	`)
	if err != nil {
		return err
	}

	// 3. Créer des entrées pour les noms sans extension (compatibilité recherche)
	_, err = db.Exec(`
		INSERT OR IGNORE INTO rename_map (original_name, display_name)
		SELECT SUBSTR(original_name, 1, LENGTH(original_name) - 4), display_name
		FROM rename_map
		WHERE original_name LIKE '%.exe'
	`)
	return err
}

// RenameSmart supports renaming when `from` is either an original_name or an existing display_name.
// - If there are rows having display_name = from, we update them to display_name = to.
// - Otherwise, we upsert a mapping original_name = from -> display_name = to.
func (db *Database) RenameSmart(from, to string) error {
	res, err := db.Exec(`UPDATE rename_map SET display_name = ? WHERE display_name = ?`, to, from)
	if err != nil {
		return err
	}
	if res != nil {
		if n, _ := res.RowsAffected(); n > 0 {
			return nil
		}
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
	Date        string  `db:"date" json:"date"`
	Seconds     float64 `db:"seconds" json:"seconds"`
	NewCSV      string  `db:"new_csv" json:"-"`
	FinishedCSV string  `db:"finished_csv" json:"-"`
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

// GameStats provides detailed statistics for a specific game
type GameStats struct {
	AvgDuration   float64   `json:"avg_duration"`
	FirstSession  string    `json:"first_session"`
	LastSession   string    `json:"last_session"`
	TotalSessions int       `json:"total_sessions"`
	HourlyDist    []int     `json:"hourly_dist"`
	DailyDist     []float64 `json:"daily_dist"`
}

func (db *Database) GetGameStats(name string, minDur, maxGap float64) (*GameStats, error) {
	stats := &GameStats{
		HourlyDist: make([]int, 24),
		DailyDist:  make([]float64, 7),
	}

	// Fetch all raw sessions for this game to perform virtual merging
	qSessions := `
	WITH resolved AS (
		SELECT a.*, COALESCE(r.display_name, a.process_name) as name
		FROM activities a
		LEFT JOIN rename_map r ON r.original_name = a.process_name
	)
	SELECT start_time, end_time, duration
	FROM resolved
	WHERE name = ?
	  AND NOT EXISTS (
	    SELECT 1 FROM blacklist bx
	    WHERE bx.name = resolved.process_name OR bx.name = resolved.name
	  )
	ORDER BY start_time ASC`

	rows, err := db.Query(qSessions, name)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type rawSession struct {
		start time.Time
		end   time.Time
	}
	var raw []rawSession
	for rows.Next() {
		var s, e string
		var d float64
		if err := rows.Scan(&s, &e, &d); err == nil {
			st, _ := time.Parse(time.RFC3339, s)
			et, _ := time.Parse(time.RFC3339, e)
			raw = append(raw, rawSession{st, et})
		}
	}

	if len(raw) == 0 {
		return stats, nil
	}

	// Virtual Merging Logic
	var virtualSessions []rawSession
	if len(raw) > 0 {
		current := raw[0]
		for i := 1; i < len(raw); i++ {
			// If gap between current end and next start is within maxGap, merge
			if raw[i].start.Sub(current.end).Seconds() <= maxGap {
				current.end = raw[i].end
			} else {
				// Finalize current session if it meets min duration
				if current.end.Sub(current.start).Seconds() >= minDur {
					virtualSessions = append(virtualSessions, current)
				}
				current = raw[i]
			}
		}
		// Last one
		if current.end.Sub(current.start).Seconds() >= minDur {
			virtualSessions = append(virtualSessions, current)
		}
	}

	if len(virtualSessions) == 0 {
		return stats, nil
	}

	// Calculate stats from virtual sessions
	var totalDur float64
	first := virtualSessions[0].start
	last := virtualSessions[0].end

	// Track total duration and unique dates per day of week
	dailyTotal := make([]float64, 7)
	dailyDates := make([]map[string]bool, 7)
	for i := 0; i < 7; i++ {
		dailyDates[i] = make(map[string]bool)
	}

	for _, vs := range virtualSessions {
		dur := vs.end.Sub(vs.start).Seconds()
		totalDur += dur
		if vs.start.Before(first) {
			first = vs.start
		}
		if vs.end.After(last) {
			last = vs.end
		}

		// Distributions (using local time for probability)
		localStart := vs.start.Local()
		stats.HourlyDist[localStart.Hour()]++

		dow := int(localStart.Weekday())
		dailyTotal[dow] += dur
		dailyDates[dow][localStart.Format("2006-01-02")] = true
	}

	for i := 0; i < 7; i++ {
		if len(dailyDates[i]) > 0 {
			stats.DailyDist[i] = dailyTotal[i] / float64(len(dailyDates[i]))
		}
	}

	stats.TotalSessions = len(virtualSessions)
	stats.AvgDuration = totalDur / float64(len(virtualSessions))
	stats.FirstSession = first.Local().Format("2006-01-02 15:04:05")
	stats.LastSession = last.Local().Format("2006-01-02 15:04:05")

	return stats, nil
}

// GlobalInsights represents aggregated statistics for all games
type GlobalInsights struct {
	AvgDuration       float64        `json:"avg_duration"`
	HourlyDist        []int          `json:"hourly_dist"`
	DailyDist         []float64      `json:"daily_dist"`
	SessionBuckets    map[string]int `json:"session_buckets"`
	HourlyAvgDuration []float64      `json:"hourly_avg_duration"`
	AvgGap            float64        `json:"avg_gap"`
	ZappingIndex      float64        `json:"zapping_index"`
	AvgGamesPerDay    float64        `json:"avg_games_per_day"`
	FidelityIndex     float64        `json:"fidelity_index"`
	PeakTime          string         `json:"peak_time"`
	WeeklyHeatmap     [][]int        `json:"weekly_heatmap"`
}

func (db *Database) GetGlobalInsights(startDate, endDate string, minDur, maxGap float64) (*GlobalInsights, error) {
	insights := &GlobalInsights{
		HourlyDist:        make([]int, 24),
		DailyDist:         make([]float64, 7),
		SessionBuckets:    map[string]int{"snack": 0, "standard": 0, "immersion": 0},
		HourlyAvgDuration: make([]float64, 24),
		WeeklyHeatmap:     make([][]int, 7),
	}
	for i := 0; i < 7; i++ {
		insights.WeeklyHeatmap[i] = make([]int, 24)
	}

	q := `
	WITH resolved AS (
		SELECT a.*, COALESCE(r.display_name, a.process_name) as name, substr(a.start_time,1,10) AS sdate
		FROM activities a
		LEFT JOIN rename_map r ON r.original_name = a.process_name
	)
	SELECT name, start_time, end_time, duration
	FROM resolved
	WHERE sdate >= ? AND sdate <= ?
	  AND NOT EXISTS (
	    SELECT 1 FROM blacklist bx
	    WHERE bx.name = resolved.process_name OR bx.name = resolved.name
	  )
	ORDER BY name, start_time ASC`

	rows, err := db.Query(q, startDate, endDate)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	type session struct {
		name  string
		start time.Time
		end   time.Time
	}
	var byGame = make(map[string][]session)
	for rows.Next() {
		var name, s, e string
		var d float64
		if err := rows.Scan(&name, &s, &e, &d); err == nil {
			st, _ := time.Parse(time.RFC3339, s)
			et, _ := time.Parse(time.RFC3339, e)
			byGame[name] = append(byGame[name], session{name, st, et})
		}
	}

	var allVirtual []session
	for _, raw := range byGame {
		if len(raw) == 0 {
			continue
		}
		current := raw[0]
		for i := 1; i < len(raw); i++ {
			if raw[i].start.Sub(current.end).Seconds() <= maxGap {
				current.end = raw[i].end
			} else {
				if current.end.Sub(current.start).Seconds() >= minDur {
					allVirtual = append(allVirtual, current)
				}
				current = raw[i]
			}
		}
		if current.end.Sub(current.start).Seconds() >= minDur {
			allVirtual = append(allVirtual, current)
		}
	}

	if len(allVirtual) == 0 {
		return insights, nil
	}

	hourlyTotalDur := make([]float64, 24)
	hourlyCount := make([]int, 24)
	dailyTotal := make([]float64, 7)
	dailyDates := make([]map[string]bool, 7)
	for i := 0; i < 7; i++ {
		dailyDates[i] = make(map[string]bool)
	}

	var totalDur float64
	uniqueGames := make(map[string]bool)
	gameDurations := make(map[string]float64)
	gamesPerDay := make(map[string]map[string]bool)
	weeklyHourlyDist := [7][24]int{}

	for _, vs := range allVirtual {
		dur := vs.end.Sub(vs.start).Seconds()
		totalDur += dur
		uniqueGames[vs.name] = true

		localStart := vs.start.Local()
		localEnd := vs.end.Local()
		hour := localStart.Hour()
		dow := int(localStart.Weekday())
		dateStr := localStart.Format("2006-01-02")

		// Track presence for every hour touched by the session
		currH := time.Date(localStart.Year(), localStart.Month(), localStart.Day(), localStart.Hour(), 0, 0, 0, localStart.Location())
		for currH.Before(localEnd) {
			h := currH.Hour()
			d := int(currH.Weekday())
			insights.HourlyDist[h]++
			weeklyHourlyDist[d][h]++
			insights.WeeklyHeatmap[d][h]++
			currH = currH.Add(time.Hour)
		}

		hourlyTotalDur[hour] += dur
		hourlyCount[hour]++

		dailyTotal[dow] += dur
		dailyDates[dow][dateStr] = true

		if gamesPerDay[dateStr] == nil {
			gamesPerDay[dateStr] = make(map[string]bool)
		}
		gamesPerDay[dateStr][vs.name] = true
		gameDurations[vs.name] += dur

		if dur < 900 {
			insights.SessionBuckets["snack"]++
		} else if dur > 3600 {
			insights.SessionBuckets["immersion"]++
		} else {
			insights.SessionBuckets["standard"]++
		}
	}

	for i := 0; i < 24; i++ {
		if hourlyCount[i] > 0 {
			insights.HourlyAvgDuration[i] = hourlyTotalDur[i] / float64(hourlyCount[i])
		}
	}
	for i := 0; i < 7; i++ {
		if len(dailyDates[i]) > 0 {
			insights.DailyDist[i] = dailyTotal[i] / float64(len(dailyDates[i]))
		}
	}

	insights.AvgDuration = totalDur / float64(len(allVirtual))
	insights.ZappingIndex = float64(len(uniqueGames)) / ((totalDur / 3600.0) + 1.0)

	if len(gamesPerDay) > 0 {
		totalUnique := 0
		for d := range gamesPerDay {
			totalUnique += len(gamesPerDay[d])
		}
		insights.AvgGamesPerDay = float64(totalUnique) / float64(len(gamesPerDay))
	}

	if totalDur > 0 {
		var durs []float64
		for _, d := range gameDurations {
			durs = append(durs, d)
		}
		sort.Float64s(durs)
		top3 := 0.0
		for i := 0; i < 3 && i < len(durs); i++ {
			top3 += durs[len(durs)-1-i]
		}
		insights.FidelityIndex = top3 / totalDur
	}

	maxCount := -1
	peakD, peakH := 0, 0
	daysFR := []string{"Dimanche", "Lundi", "Mardi", "Mercredi", "Jeudi", "Vendredi", "Samedi"}
	for d := 0; d < 7; d++ {
		for h := 0; h < 24; h++ {
			if weeklyHourlyDist[d][h] > maxCount {
				maxCount = weeklyHourlyDist[d][h]
				peakD, peakH = d, h
			}
		}
	}
	if maxCount > 0 {
		insights.PeakTime = fmt.Sprintf("%s %dh", daysFR[peakD], peakH)
	}

	// Friction: average time between consecutive sessions regardless of game
	sort.Slice(allVirtual, func(i, j int) bool {
		return allVirtual[i].start.Before(allVirtual[j].start)
	})

	var totalGap float64
	var gapCount int
	for i := 0; i < len(allVirtual)-1; i++ {
		gap := allVirtual[i+1].start.Sub(allVirtual[i].end).Seconds()
		if gap > 0 {
			totalGap += gap
			gapCount++
		}
	}
	if gapCount > 0 {
		insights.AvgGap = totalGap / float64(gapCount)
	}

	return insights, nil
}
