package query

import (
	"fmt"
	"time"
)

type LibraryItem struct {
	Name                 string  `json:"name"`
	Original             string  `json:"original"`
	IconURL              string  `json:"icon_url"`
	AppID                int     `json:"appid"`
	TotalSeconds         float64 `json:"total_seconds"`
	SessionCount         int     `json:"session_count"`
	AvgSessionSeconds    float64 `json:"avg_session_seconds"`
	FirstSession         string  `json:"first_session"`
	LastSession          string  `json:"last_session"`
	Finished             bool    `json:"finished"`
	Blacklisted          bool    `json:"blacklisted"`
	AchievementsTotal    int     `json:"achievements_total"`
	AchievementsUnlocked int     `json:"achievements_unlocked"`
	AchievementsPct      float64 `json:"achievements_pct"`
}

func (db *Database) GetLibrary(minDur, maxGap float64, includeBlacklisted bool) ([]LibraryItem, error) {
	// 1. Get all games and their basic meta
	// We use a query similar to GetGamesMetaBetween but without the date filter
	qMeta := `
	WITH base AS (
	    SELECT a.process_name, COALESCE(r.display_name, a.process_name) AS name, a.duration, a.start_time, a.end_time
	    FROM activities a
	    LEFT JOIN rename_map r ON r.original_name = a.process_name
	), games AS (
	    SELECT
	        b.process_name,
	        COALESCE(r.display_name, b.process_name) AS name
	    FROM activities b
	    LEFT JOIN rename_map r ON r.original_name = b.process_name
	    GROUP BY b.process_name, COALESCE(r.display_name, b.process_name)
	)
	SELECT
	    g.name,
	    g.process_name,
	    COALESCE(MAX(sm.icon_url), MAX(sog.icon_url), '') AS icon_url,
	    COALESCE(MAX(sm.appid), MAX(sog.appid), 0) AS appid,
	    (SELECT COUNT(*) FROM finished_games fg WHERE fg.name = g.name) > 0 AS finished,
	    (SELECT COUNT(*) FROM blacklist bl WHERE bl.name = g.process_name OR bl.name = g.name) > 0 AS blacklisted
	FROM games g
	LEFT JOIN steam_mapping sm ON sm.process_name = g.process_name
	LEFT JOIN steam_owned_games sog ON sog.game_name = g.name
	GROUP BY g.name, g.process_name
	`

	type metaRow struct {
		Name        string `db:"name"`
		ProcessName string `db:"process_name"`
		IconURL     string `db:"icon_url"`
		AppID       int    `db:"appid"`
		Finished    bool   `db:"finished"`
		Blacklisted bool   `db:"blacklisted"`
	}
	var metas []metaRow
	if err := db.Select(&metas, qMeta); err != nil {
		return nil, fmt.Errorf("GetLibrary meta: %w", err)
	}

	// 2. Get all activities for session merging in one go to avoid N queries
	qActs := `
	SELECT a.process_name, a.start_time, a.end_time, a.duration
	FROM activities a
	ORDER BY a.process_name, a.start_time ASC
	`
	type actRow struct {
		ProcessName string  `db:"process_name"`
		StartTime   string  `db:"start_time"`
		EndTime     string  `db:"end_time"`
		Duration    float64 `db:"duration"`
	}
	var allActs []actRow
	if err := db.Select(&allActs, qActs); err != nil {
		return nil, fmt.Errorf("GetLibrary activities: %w", err)
	}

	// Group activities by process name
	actsByProc := make(map[string][]actRow)
	for _, a := range allActs {
		actsByProc[a.ProcessName] = append(actsByProc[a.ProcessName], a)
	}

	var library []LibraryItem

	for _, m := range metas {
		if !includeBlacklisted && m.Blacklisted {
			continue
		}

		// Calculate session stats using virtual merging
		raw := actsByProc[m.ProcessName]
		var virtualSessions []struct {
			start time.Time
			end   time.Time
		}

		var totalSeconds float64
		var firstSession, lastSession time.Time

		if len(raw) > 0 {
			// Parse first activity to start
			st, _ := time.Parse(time.RFC3339, raw[0].StartTime)
			et, _ := time.Parse(time.RFC3339, raw[0].EndTime)
			current := struct {
				start time.Time
				end   time.Time
			}{st, et}

			for i := 1; i < len(raw); i++ {
				s, _ := time.Parse(time.RFC3339, raw[i].StartTime)
				e, _ := time.Parse(time.RFC3339, raw[i].EndTime)
				if s.Sub(current.end).Seconds() <= maxGap {
					current.end = e
				} else {
					if current.end.Sub(current.start).Seconds() >= minDur {
						virtualSessions = append(virtualSessions, current)
						totalSeconds += current.end.Sub(current.start).Seconds()
					}
					current = struct {
						start time.Time
						end   time.Time
					}{s, e}
				}
			}
			if current.end.Sub(current.start).Seconds() >= minDur {
				virtualSessions = append(virtualSessions, current)
				totalSeconds += current.end.Sub(current.start).Seconds()
			}

			if len(virtualSessions) > 0 {
				firstSession = virtualSessions[0].start
				lastSession = virtualSessions[len(virtualSessions)-1].end
			}
		}

		// Achievements
		unlocked, total, err := db.GetUnlockedCount(m.AppID)
		pct := -1.0
		if err == nil && total > 0 {
			pct = (float64(unlocked) / float64(total)) * 100
		}

		avgSession := 0.0
		if len(virtualSessions) > 0 {
			avgSession = totalSeconds / float64(len(virtualSessions))
		}

		library = append(library, LibraryItem{
			Name:                 m.Name,
			Original:             m.ProcessName,
			IconURL:              m.IconURL,
			AppID:                m.AppID,
			TotalSeconds:         totalSeconds,
			SessionCount:         len(virtualSessions),
			AvgSessionSeconds:    avgSession,
			FirstSession:         firstSession.Local().Format("2006-01-02 15:04:05"),
			LastSession:          lastSession.Local().Format("2006-01-02 15:04:05"),
			Finished:             m.Finished,
			Blacklisted:          m.Blacklisted,
			AchievementsTotal:    total,
			AchievementsUnlocked: unlocked,
			AchievementsPct:      pct,
		})
	}

	return library, nil
}
