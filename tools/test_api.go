package main

import (
	"fmt"
	"log"
	"main/query"
	"os"
	"path/filepath"
	"time"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal("Impossible de trouver le dossier de config :", err)
	}

	dbPath := filepath.Join(configDir, "SteamTracker", "activity_tracker.db")
	fmt.Printf("=== Diagnostic Direct de la Base de Données ===\n")
	fmt.Printf("Fichier : %s\n\n", dbPath)

	dbTemp, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal("Erreur ouverture DB :", err)
	}
	db := query.NewDatabase(dbTemp)
	defer db.Close()

	// 1. Test de la table activities
	var count int
	err = db.Get(&count, "SELECT COUNT(*) FROM activities")
	if err != nil {
		fmt.Printf("[ERREUR] Lecture activities : %v\n", err)
	} else {
		fmt.Printf("[OK] Nombre d'activités en base : %d\n", count)
	}

	// 2. Test de GetSummaryBetween (utilisé pour les graphiques)
	now := time.Now()
	start := now.AddDate(0, 0, -7).Format("2006-01-02")
	end := now.Format("2006-01-02")
	fmt.Printf("\nTest GetSummaryBetween (période %s à %s) :\n", start, end)
	summary, err := db.GetSummaryBetween(start, end)
	if err != nil {
		fmt.Printf("[ERREUR] GetSummaryBetween : %v\n", err)
	} else {
		fmt.Printf("[OK] Nombre de jeux trouvés pour les graphiques : %d\n", len(summary))
		for i, item := range summary {
			if i >= 5 {
				fmt.Println("  ...")
				break
			}
			fmt.Printf("  - %s : %v secondes\n", item.Name, item.Seconds)
		}
	}

	// 3. Test de GetGamesMetaBetween (le suspect n°1 pour le bug d'affichage)
	fmt.Printf("\nTest GetGamesMetaBetween :\n")
	meta, err := db.GetGamesMetaBetween(start, end)
	if err != nil {
		fmt.Printf("[ERREUR] GetGamesMetaBetween : %v\n", err)
		fmt.Println("Conseil : Vérifiez si une colonne manque ou si la requête SQL JOIN est invalide.")
	} else {
		fmt.Printf("[OK] Métadonnées trouvées : %d\n", len(meta))
		if len(meta) > 0 {
			fmt.Printf("  Exemple : %s (Icon: %s, AppID: %d)\n", meta[0].Name, meta[0].IconURL, meta[0].AppID)
		}
	}

	// 4. Vérification des nouvelles tables Steam
	tables := []string{"steam_mapping", "achievements", "app_settings"}
	fmt.Printf("\nVérification des tables Steam :\n")
	for _, t := range tables {
		var exists int
		_ = db.Get(&exists, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", t)
		if exists > 0 {
			var rowCount int
			_ = db.Get(&rowCount, fmt.Sprintf("SELECT COUNT(*) FROM %s", t))
			fmt.Printf("  - Table '%s' : OK (%d lignes)\n", t, rowCount)
		} else {
			fmt.Printf("  - Table '%s' : MANQUANTE\n", t)
		}
	}

	fmt.Println("\n=== Fin du diagnostic ===")
}
