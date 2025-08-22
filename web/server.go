package web

import (
	"embed"
	"encoding/json"
	"log"
	"net/http"
	"sort"
	"strings"
	"time"

	"main/manager"
	"main/query"
)

//go:embed static/*
var staticFS embed.FS

type Server struct {
	db *query.Database
	lm *manager.ListManager
}

func StartServer(db *query.Database, lm *manager.ListManager) {
	s := &Server{db: db, lm: lm}

	http.HandleFunc("/", s.handleIndex)
	http.HandleFunc("/history", s.handleHistoryPage)
	http.HandleFunc("/config", s.handleConfigPage)
	http.Handle("/static/", http.FileServer(http.FS(staticFS)))

	http.HandleFunc("/api/summary", s.handleSummary)
	http.HandleFunc("/api/history", s.handleHistory)
	http.HandleFunc("/api/blacklist", s.handleBlacklist)
	http.HandleFunc("/api/unblacklist", s.handleUnblacklist)
	http.HandleFunc("/api/whitelist", s.handleWhitelist)
	http.HandleFunc("/api/unwhitelist", s.handleUnwhitelist)
	http.HandleFunc("/api/known_processes", s.handleKnownProcesses)
	http.HandleFunc("/api/rename", s.handleRename)
	http.HandleFunc("/api/finished", s.handleFinished)
	http.HandleFunc("/api/series", s.handleSeries)
	http.HandleFunc("/api/games_meta", s.handleGamesMeta)
	http.HandleFunc("/api/calendar", s.handleCalendar)
	http.HandleFunc("/api/set_first_launch_date", s.handleSetFirstLaunchDate)
	http.HandleFunc("/api/set_finished_date", s.handleSetFinishedDate)
	// Export / Import API
	http.HandleFunc("/api/export", s.handleExport)
	http.HandleFunc("/api/import", s.handleImport)
	// History delete API
	http.HandleFunc("/api/history_delete", s.handleHistoryDelete)
	// Day timeline API
	http.HandleFunc("/api/day_timeline", s.handleDayTimeline)

	go func() {
		// Bind explicitly to localhost to avoid Windows Firewall prompts
		addr := "127.0.0.1:8080"
		log.Printf("Web UI disponible sur http://%v\n", addr)
		if err := http.ListenAndServe(addr, nil); err != nil {
			log.Println("Erreur serveur web:", err)
		}
	}()
}

func (s *Server) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, _ := staticFS.ReadFile("static/index.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleHistoryPage(w http.ResponseWriter, r *http.Request) {
	data, _ := staticFS.ReadFile("static/history.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleConfigPage(w http.ResponseWriter, r *http.Request) {
	data, _ := staticFS.ReadFile("static/config.html")
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (s *Server) handleSummary(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" { period = "week" }
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		start, end = query.PeriodRange(period, time.Now())
	}
	items, err := s.db.GetSummaryBetween(start, end)
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError); return
	}
	resp := map[string]any{"start": start, "end": end, "items": items}
	writeJSON(w, resp)
}

func (s *Server) handleHistory(w http.ResponseWriter, r *http.Request) {
	hb := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("hide_blacklisted")))
	hide := hb == "1" || hb == "true" || hb == "yes"
	items, err := s.db.GetHistory(hide)
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, items)
}

func (s *Server) handleBlacklist(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		names, err := s.db.GetAllBlacklisted()
		if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
		writeJSON(w, names); return
	}
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{ Name string `json:"name"` }
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	name := strings.TrimSpace(body.Name)
	if name == "" { http.Error(w, "name empty", http.StatusBadRequest); return }
	// Try to also blacklist original executable names if this is a display name
	if err := s.lm.AddToBlacklist(name); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	if originals, err := s.db.GetOriginalsForDisplay(name); err == nil {
		for _, o := range originals { _ = s.lm.AddToBlacklist(o) }
	}
	writeJSON(w, map[string]string{"status":"ok"})
}

func (s *Server) handleUnblacklist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{ Name string `json:"name"` }
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	name := strings.TrimSpace(body.Name)
	if name == "" { http.Error(w, "name empty", http.StatusBadRequest); return }
	// Remove both the provided name and any originals mapped to it (if it is a display name)
	if err := s.lm.RemoveFromBlacklist(name); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	if originals, err := s.db.GetOriginalsForDisplay(name); err == nil {
		for _, o := range originals { _ = s.lm.RemoveFromBlacklist(o) }
	}
	writeJSON(w, map[string]string{"status":"ok"})
}

func (s *Server) handleWhitelist(w http.ResponseWriter, r *http.Request) {
	if r.Method == http.MethodGet {
		names, err := s.db.GetAllWhitelisted()
		if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
		writeJSON(w, names); return
	}
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{ Name string `json:"name"` }
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	name := strings.TrimSpace(body.Name)
	if name == "" { http.Error(w, "name empty", http.StatusBadRequest); return }
	if err := s.lm.AddToWhitelist(name); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, map[string]string{"status":"ok"})
}

func (s *Server) handleUnwhitelist(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{ Name string `json:"name"` }
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	name := strings.TrimSpace(body.Name)
	if name == "" { http.Error(w, "name empty", http.StatusBadRequest); return }
	if err := s.lm.RemoveFromWhitelist(name); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, map[string]string{"status":"ok"})
}

func (s *Server) handleKnownProcesses(w http.ResponseWriter, r *http.Request) {
	rows, err := s.db.GetAllKnownProcesses()
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, rows)
}

func (s *Server) handleRename(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{ From string `json:"from"`; To string `json:"to"` }
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	from := strings.TrimSpace(body.From)
	to := strings.TrimSpace(body.To)
	if from == "" || to == "" { http.Error(w, "from/to empty", http.StatusBadRequest); return }
	if err := s.db.RenameSmart(from, to); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, map[string]string{"status":"ok"})
}

func (s *Server) handleFinished(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{ Name string `json:"name"`; Done *bool `json:"done"` }
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	name := strings.TrimSpace(body.Name)
	if name == "" { http.Error(w, "name empty", http.StatusBadRequest); return }
	// If Done is nil, toggle; else set state
	if body.Done == nil {
		if finished, err := s.db.IsFinished(name); err == nil {
			if finished { _ = s.db.DeleteFinished(name) } else { _ = s.db.InsertFinished(name) }
		} else {
			http.Error(w, err.Error(), http.StatusInternalServerError); return
		}
	} else if *body.Done {
		if err := s.db.InsertFinished(name); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	} else {
		if err := s.db.DeleteFinished(name); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	}
	writeJSON(w, map[string]string{"status":"ok"})
}

// handleSeries builds a matrix suitable for stacked bar chart
func (s *Server) handleSeries(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" { period = "week" }
	by := r.URL.Query().Get("by")
	if period == "year" && by == "" { by = "month" }
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" {
		start, end = query.PeriodRange(period, time.Now())
	}
	rows, err := s.db.GetSeries(period, start, end, by)
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	// Build full labels between start and end (inclusive) with appropriate step
	var labels []string
	if period == "year" {
		startT, err1 := time.Parse("2006-01-02", start)
		endT, err2 := time.Parse("2006-01-02", end)
		if err1 != nil || err2 != nil { http.Error(w, "invalid date range", http.StatusBadRequest); return }
		if by == "week" {
			// labels are Mondays (YYYY-MM-DD) for each ISO week-like bucket used in SQL
			// Find Monday of the week for start
			cur := time.Date(startT.Year(), startT.Month(), startT.Day(), 0, 0, 0, 0, time.UTC)
			for cur.Weekday() != time.Monday { cur = cur.AddDate(0,0,-1) }
			last := time.Date(endT.Year(), endT.Month(), endT.Day(), 0, 0, 0, 0, time.UTC)
			for last.Weekday() != time.Monday { last = last.AddDate(0,0,1) }
			for !cur.After(last) {
				labels = append(labels, cur.Format("2006-01-02"))
				cur = cur.AddDate(0, 0, 7)
			}
		} else {
			// monthly buckets YYYY-MM
			cur := time.Date(startT.Year(), startT.Month(), 1, 0, 0, 0, 0, time.UTC)
			last := time.Date(endT.Year(), endT.Month(), 1, 0, 0, 0, 0, time.UTC)
			for !cur.After(last) {
				labels = append(labels, cur.Format("2006-01"))
				cur = cur.AddDate(0, 1, 0)
			}
		}
	} else {
		// daily buckets YYYY-MM-DD
		startT, err1 := time.Parse("2006-01-02", start)
		endT, err2 := time.Parse("2006-01-02", end)
		if err1 != nil || err2 != nil { http.Error(w, "invalid date range", http.StatusBadRequest); return }
		cur := time.Date(startT.Year(), startT.Month(), startT.Day(), 0, 0, 0, 0, time.UTC)
		last := time.Date(endT.Year(), endT.Month(), endT.Day(), 0, 0, 0, 0, time.UTC)
		for !cur.After(last) {
			labels = append(labels, cur.Format("2006-01-02"))
			cur = cur.AddDate(0, 0, 1)
		}
	}
	indexByLabel := map[string]int{}
	for i, lb := range labels { indexByLabel[lb] = i }
	// games set and totals for sorting datasets
	gameSet := map[string]float64{}
	for _, r := range rows { gameSet[r.Name] += r.Seconds }
	games := make([]string, 0, len(gameSet))
	for g := range gameSet { games = append(games, g) }
	sort.Slice(games, func(i, j int) bool { return gameSet[games[i]] > gameSet[games[j]] })
	indexByGame := map[string]int{}
	for i, g := range games { indexByGame[g] = i }
	// matrix [games][labels]
	matrix := make([][]float64, len(games))
	for i := range matrix { matrix[i] = make([]float64, len(labels)) }
	for _, row := range rows {
		gi := indexByGame[row.Name]
		li, ok := indexByLabel[row.Bucket]
		if !ok { continue }
		matrix[gi][li] = row.Seconds
	}
	writeJSON(w, map[string]any{
		"start":  start,
		"end":    end,
		"labels": labels,
		"games":  games,
		"matrix": matrix,
	})
}

func (s *Server) handleGamesMeta(w http.ResponseWriter, r *http.Request) {
	period := r.URL.Query().Get("period")
	if period == "" { period = "week" }
	start := r.URL.Query().Get("start")
	end := r.URL.Query().Get("end")
	if start == "" || end == "" { start, end = query.PeriodRange(period, time.Now()) }
	items, err := s.db.GetGamesMetaBetween(start, end)
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, map[string]any{"start":start, "end":end, "items":items})
}

// handleCalendar returns daily totals and new/finished lists for a given year or range
func (s *Server) handleCalendar(w http.ResponseWriter, r *http.Request) {
	yearStr := strings.TrimSpace(r.URL.Query().Get("year"))
	var start, end string
	if yearStr != "" {
		y, err := time.Parse("2006", yearStr)
		if err != nil { http.Error(w, "bad year", http.StatusBadRequest); return }
		start = y.Format("2006") + "-01-01"
		end = y.Format("2006") + "-12-31"
	} else {
		// fallback to period=year logic
		s, e := query.PeriodRange("year", time.Now())
		start, end = s, e
	}
	rows, err := s.db.GetCalendarDays(start, end)
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	// transform to map with arrays instead of CSVs
	type Day struct{
		Date string `json:"date"`
		Seconds float64 `json:"seconds"`
		New []string `json:"new"`
		Finished []string `json:"finished"`
	}
	out := make([]Day, 0, len(rows))
	for _, r1 := range rows {
		var nlist, flist []string
		if strings.TrimSpace(r1.NewCSV) != "" { nlist = strings.Split(r1.NewCSV, "||") }
		if strings.TrimSpace(r1.FinishedCSV) != "" { flist = strings.Split(r1.FinishedCSV, "||") }
		out = append(out, Day{ Date: r1.Date, Seconds: r1.Seconds, New: nlist, Finished: flist })
	}
	writeJSON(w, map[string]any{ "start": start, "end": end, "days": out })
}

func (s *Server) handleSetFirstLaunchDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{ Name string `json:"name"`; Date string `json:"date"` }
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	name := strings.TrimSpace(body.Name)
	date := strings.TrimSpace(body.Date)
	if name == "" || date == "" { http.Error(w, "name/date empty", http.StatusBadRequest); return }
	// Validate date format YYYY-MM-DD
	if _, err := time.Parse("2006-01-02", date); err != nil { http.Error(w, "bad date", http.StatusBadRequest); return }
	if err := s.db.UpsertFirstLaunchOverride(name, date); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, map[string]string{"status":"ok"})
}

func (s *Server) handleSetFinishedDate(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{ Name string `json:"name"`; Date string `json:"date"` }
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	name := strings.TrimSpace(body.Name)
	date := strings.TrimSpace(body.Date)
	if name == "" || date == "" { http.Error(w, "name/date empty", http.StatusBadRequest); return }
	if _, err := time.Parse("2006-01-02", date); err != nil { http.Error(w, "bad date", http.StatusBadRequest); return }
	if err := s.db.UpsertFinishedAt(name, date); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, map[string]string{"status":"ok"})
}

// handleHistoryDelete removes a single activity entry from the history (irreversible)
func (s *Server) handleHistoryDelete(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	type req struct{
		ProcessName string `json:"process_name"`
		StartTime   string `json:"start_time"`
		EndTime     string `json:"end_time"`
	}
	var body req
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil { http.Error(w, "bad request", http.StatusBadRequest); return }
	p := strings.TrimSpace(body.ProcessName)
	st := strings.TrimSpace(body.StartTime)
	et := strings.TrimSpace(body.EndTime)
	if p=="" || st=="" || et=="" { http.Error(w, "missing fields", http.StatusBadRequest); return }
	// Optional: basic RFC3339 validation
	if _, err := time.Parse(time.RFC3339, st); err != nil { http.Error(w, "bad start_time", http.StatusBadRequest); return }
	if _, err := time.Parse(time.RFC3339, et); err != nil { http.Error(w, "bad end_time", http.StatusBadRequest); return }
	if err := s.db.DeleteActivity(p, st, et); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, map[string]string{"status":"ok"})
}

// handleDayTimeline returns clipped intervals for a given date (YYYY-MM-DD)
func (s *Server) handleDayTimeline(w http.ResponseWriter, r *http.Request) {
	date := strings.TrimSpace(r.URL.Query().Get("date"))
	if date == "" { http.Error(w, "missing date", http.StatusBadRequest); return }
	if _, err := time.Parse("2006-01-02", date); err != nil { http.Error(w, "bad date", http.StatusBadRequest); return }
	rows, err := s.db.GetIntervalsForDate(date)
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	// Determine timezone from query: tz=IANA name (e.g., Europe/Paris). Empty => system local.
	tz := strings.TrimSpace(r.URL.Query().Get("tz"))
	var loc *time.Location
	if tz == "" {
		loc = time.Local
	} else {
		l, err := time.LoadLocation(tz)
		if err != nil { loc = time.Local } else { loc = l }
	}
	// Build [00:00,24:00) of that date in the chosen timezone
	dayStartLocal, _ := time.ParseInLocation("2006-01-02", date, loc)
	dayStart := time.Date(dayStartLocal.Year(), dayStartLocal.Month(), dayStartLocal.Day(), 0, 0, 0, 0, loc)
	dayEnd := dayStart.Add(24 * time.Hour)
	type Seg struct{ Name string `json:"name"`; StartSec int `json:"start_sec"`; EndSec int `json:"end_sec"` }
	segs := make([]Seg,0,len(rows))
	for _, row := range rows {
		st, err1 := time.Parse(time.RFC3339, row.StartTime)
		et, err2 := time.Parse(time.RFC3339, row.EndTime)
		if err1 != nil || err2 != nil { continue }
		// Convert to selected timezone for clipping relative to local midnight
		st = st.In(loc)
		et = et.In(loc)
		if et.Before(dayStart) || st.After(dayEnd) { continue }
		if st.Before(dayStart) { st = dayStart }
		if et.After(dayEnd) { et = dayEnd }
		ss := int(st.Sub(dayStart).Seconds())
		es := int(et.Sub(dayStart).Seconds())
		if es > ss { segs = append(segs, Seg{Name: row.Name, StartSec: ss, EndSec: es}) }
	}
	writeJSON(w, map[string]any{"date": date, "segments": segs})
}

// Export / Import structures

type activityRow struct {
	ProcessName string  `db:"process_name" json:"process_name"`
	StartTime   string  `db:"start_time" json:"start_time"`
	EndTime     string  `db:"end_time" json:"end_time"`
	Duration    float64 `db:"duration" json:"duration"`
	Date        string  `db:"date" json:"date"`
	FirstLaunch bool    `db:"first_launch" json:"first_launch"`
}

type renameRow struct {
	OriginalName string `db:"original_name" json:"original_name"`
	DisplayName  string `db:"display_name" json:"display_name"`
}

type finishedRow struct {
	Name       string `db:"name" json:"name"`
	FinishedAt string `db:"finished_at" json:"finished_at"`
}

type firstLaunchRow struct {
	Name      string `db:"name" json:"name"`
	FirstDate string `db:"first_date" json:"first_date"`
}

type metaInfo struct {
	SchemaVersion int    `json:"schema_version"`
	ExportedAt    string `json:"exported_at"`
	Timezone      string `json:"timezone"`
}

type exportPayload struct {
	Mode                 string           `json:"mode,omitempty"`
	Meta                 metaInfo         `json:"meta"`
	Activities           []activityRow    `json:"activities"`
	Whitelist            []string         `json:"whitelist"`
	Blacklist            []string         `json:"blacklist"`
	RenameMap            []renameRow      `json:"rename_map"`
	FinishedGames        []finishedRow    `json:"finished_games"`
	FirstLaunchOverrides []firstLaunchRow `json:"first_launch_override"`
}

func (s *Server) handleExport(w http.ResponseWriter, r *http.Request) {
	// Read all tables
	var acts []activityRow
	if err := s.db.Select(&acts, `SELECT process_name, start_time, end_time, duration, date, first_launch FROM activities ORDER BY start_time`); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError); return
	}
	wl, err := s.db.GetAllWhitelisted()
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	bl, err := s.db.GetAllBlacklisted()
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	var ren []renameRow
	if err := s.db.Select(&ren, `SELECT original_name, display_name FROM rename_map ORDER BY original_name`); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError); return
	}
	var fin []finishedRow
	if err := s.db.Select(&fin, `SELECT name, COALESCE(finished_at,'') AS finished_at FROM finished_games ORDER BY name`); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError); return
	}
	var flo []firstLaunchRow
	if err := s.db.Select(&flo, `SELECT name, COALESCE(first_date,'') AS first_date FROM first_launch_override ORDER BY name`); err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError); return
	}
 // Build meta
	ver, _ := s.db.GetDbVersion()
	now := time.Now()
	payload := exportPayload{
		Mode: "",
		Meta: metaInfo{ SchemaVersion: ver, ExportedAt: now.Format(time.RFC3339), Timezone: now.Format("-0700") },
		Activities: acts, Whitelist: wl, Blacklist: bl, RenameMap: ren, FinishedGames: fin, FirstLaunchOverrides: flo,
	}
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	fname := "steam_tracker_export_" + now.Format("20060102_150405") + ".json"
	w.Header().Set("Content-Disposition", "attachment; filename=\""+fname+"\"")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	_ = enc.Encode(payload)
}

func (s *Server) handleImport(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost { http.Error(w, "method not allowed", http.StatusMethodNotAllowed); return }
	// Limit body to 25MB to avoid excessive memory usage
	limited := http.MaxBytesReader(w, r.Body, 25<<20)
	defer limited.Close()
	var payload exportPayload
	if err := json.NewDecoder(limited).Decode(&payload); err != nil { http.Error(w, "bad json", http.StatusBadRequest); return }
	mode := strings.ToLower(strings.TrimSpace(r.URL.Query().Get("mode")))
	if mode == "" { mode = strings.ToLower(strings.TrimSpace(payload.Mode)) }
	if mode != "replace" { mode = "merge" }
	// Begin transaction
	tx, err := s.db.Beginx()
	if err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	rollback := func(){ _ = tx.Rollback() }
	if mode == "replace" {
		// Clear all data tables (keep database_version)
		stmts := []string{
			"DELETE FROM activities",
			"DELETE FROM whitelist",
			"DELETE FROM blacklist",
			"DELETE FROM rename_map",
			"DELETE FROM finished_games",
			"DELETE FROM first_launch_override",
		}
		for _, q := range stmts {
			if _, err := tx.Exec(q); err != nil { rollback(); http.Error(w, err.Error(), http.StatusInternalServerError); return }
		}
	}
	// Insert / merge
	// Activities (validate and normalize)
	for _, a := range payload.Activities {
		pname := strings.TrimSpace(a.ProcessName)
		if pname == "" { continue }
		// Parse times
		st, err1 := time.Parse(time.RFC3339, strings.TrimSpace(a.StartTime))
		et, err2 := time.Parse(time.RFC3339, strings.TrimSpace(a.EndTime))
		if err1 != nil || err2 != nil { continue }
		if et.Before(st) { continue }
		// Normalize to RFC3339 (UTC)
		stUTC := st.UTC().Format(time.RFC3339)
		etUTC := et.UTC().Format(time.RFC3339)
		// Derive date from start UTC
		dateStr := st.UTC().Format("2006-01-02")
		if mode == "merge" {
			var exists bool
			if err := tx.Get(&exists, `SELECT EXISTS(SELECT 1 FROM activities WHERE process_name=? AND start_time=? AND end_time=?)`, pname, stUTC, etUTC); err != nil { rollback(); http.Error(w, err.Error(), http.StatusInternalServerError); return }
			if exists { continue }
		}
		if _, err := tx.Exec(`INSERT INTO activities (process_name, start_time, end_time, duration, date, first_launch) VALUES (?,?,?,?,?,?)`, pname, stUTC, etUTC, a.Duration, dateStr, a.FirstLaunch); err != nil { rollback(); http.Error(w, err.Error(), http.StatusInternalServerError); return }
	}
	// Whitelist / Blacklist
	for _, name := range payload.Whitelist {
		n := strings.TrimSpace(name); if n=="" { continue }
		if _, err := tx.Exec(`INSERT OR IGNORE INTO whitelist (name) VALUES (?)`, n); err != nil { rollback(); http.Error(w, err.Error(), http.StatusInternalServerError); return }
	}
	for _, name := range payload.Blacklist {
		n := strings.TrimSpace(name); if n=="" { continue }
		if _, err := tx.Exec(`INSERT OR IGNORE INTO blacklist (name) VALUES (?)`, n); err != nil { rollback(); http.Error(w, err.Error(), http.StatusInternalServerError); return }
	}
	// Rename map upsert
	for _, r1 := range payload.RenameMap {
		orig := strings.TrimSpace(r1.OriginalName)
		disp := strings.TrimSpace(r1.DisplayName)
		if orig=="" || disp=="" { continue }
		if _, err := tx.Exec(`INSERT INTO rename_map (original_name, display_name) VALUES (?, ?) ON CONFLICT(original_name) DO UPDATE SET display_name=excluded.display_name`, orig, disp); err != nil { rollback(); http.Error(w, err.Error(), http.StatusInternalServerError); return }
	}
	// Finished upsert with validation
	for _, f := range payload.FinishedGames {
		name := strings.TrimSpace(f.Name); if name=="" { continue }
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(f.FinishedAt)); err != nil { continue }
		if _, err := tx.Exec(`INSERT INTO finished_games (name, finished_at) VALUES (?, ?) ON CONFLICT(name) DO UPDATE SET finished_at=excluded.finished_at`, name, f.FinishedAt); err != nil { rollback(); http.Error(w, err.Error(), http.StatusInternalServerError); return }
	}
	// First launch override upsert with validation
	for _, fl := range payload.FirstLaunchOverrides {
		name := strings.TrimSpace(fl.Name); if name=="" { continue }
		if _, err := time.Parse("2006-01-02", strings.TrimSpace(fl.FirstDate)); err != nil { continue }
		if _, err := tx.Exec(`INSERT INTO first_launch_override (name, first_date) VALUES (?, ?) ON CONFLICT(name) DO UPDATE SET first_date=excluded.first_date`, name, fl.FirstDate); err != nil { rollback(); http.Error(w, err.Error(), http.StatusInternalServerError); return }
	}
	if err := tx.Commit(); err != nil { http.Error(w, err.Error(), http.StatusInternalServerError); return }
	writeJSON(w, map[string]string{"status":"ok","mode":mode})
}

func writeJSON(w http.ResponseWriter, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.SetIndent("", "  ")
	enc.Encode(v)
}
