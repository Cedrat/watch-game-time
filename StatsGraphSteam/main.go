package statsgraphsteam

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"log"
	"net/http"
	"strconv"

	"main/query"
)

//go:embed static
var staticFS embed.FS

type handler struct {
	db *query.Database
}

// Register monte les routes StatsGraphSteam sur le mux par défaut sous /stats.
// La clé API Steam est lue depuis la base de données principale (table app_settings),
// partagée avec le reste de l'application.
func Register(db *query.Database) {
	staticSub, err := fs.Sub(staticFS, "static")
	if err != nil {
		log.Fatalf("[StatsGraphSteam] embed static: %v", err)
	}

	h := &handler{db: db}

	http.HandleFunc("/stats", h.handleIndex)
	http.Handle("/stats/static/", http.StripPrefix("/stats/static/", http.FileServer(http.FS(staticSub))))
	http.HandleFunc("/stats/api/settings", h.handleSettings)
	http.HandleFunc("/stats/api/achievements", h.handleAchievements)
	http.HandleFunc("/stats/api/search", h.handleSearch)

	fmt.Println("[StatsGraphSteam] routes disponibles sous /stats")
}

func (h *handler) handleIndex(w http.ResponseWriter, r *http.Request) {
	data, err := staticFS.ReadFile("static/index.html")
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
		return
	}
	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	w.Write(data)
}

func (h *handler) handleSettings(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch r.Method {
	case http.MethodGet:
		key, _ := h.db.GetSteamAPIKey()
		masked := ""
		if key != "" {
			if len(key) > 4 {
				masked = key[:2] + "…" + key[len(key)-2:]
			} else {
				masked = "****"
			}
		}
		json.NewEncoder(w).Encode(map[string]any{
			"has_key": key != "",
			"masked":  masked,
		})
	case http.MethodPost:
		var body struct {
			SteamAPIKey string `json:"steam_api_key"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			http.Error(w, "JSON invalide", http.StatusBadRequest)
			return
		}
		if err := h.db.SetSetting("steam_api_key", body.SteamAPIKey); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{"ok": true})
	default:
		http.Error(w, "Méthode non autorisée", http.StatusMethodNotAllowed)
	}
}

func (h *handler) handleAchievements(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	appid := r.URL.Query().Get("appid")
	if appid == "" {
		http.Error(w, `{"error":"appid manquant"}`, http.StatusBadRequest)
		return
	}
	if _, err := strconv.Atoi(appid); err != nil {
		http.Error(w, `{"error":"appid doit être numérique"}`, http.StatusBadRequest)
		return
	}
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "french"
	}

	apiKey, _ := h.db.GetSteamAPIKey()
	gameName, achs, err := GetAchievements(appid, lang, apiKey)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"gameName":     gameName,
		"appid":        appid,
		"achievements": achs,
	})
}

func (h *handler) handleSearch(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	term := r.URL.Query().Get("term")
	lang := r.URL.Query().Get("lang")
	if lang == "" {
		lang = "french"
	}

	results, err := SearchGames(term, lang)
	if err != nil {
		w.WriteHeader(http.StatusBadGateway)
		json.NewEncoder(w).Encode(map[string]any{"error": err.Error()})
		return
	}
	json.NewEncoder(w).Encode(map[string]any{
		"results": results,
	})
}
