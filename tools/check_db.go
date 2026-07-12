package main

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
	_ "modernc.org/sqlite"
)

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal("Could not find config dir:", err)
	}

	dbPath := filepath.Join(configDir, "SteamTracker", "activity_tracker.db")
	fmt.Printf("Checking database at: %s\n", dbPath)

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		log.Fatal("Database file does not exist.")
	}

	db, err := sqlx.Open("sqlite", dbPath)
	if err != nil {
		log.Fatal("Could not open database:", err)
	}
	defer db.Close()

	var version int
	err = db.Get(&version, "SELECT db_version FROM database_version LIMIT 1")
	if err != nil {
		fmt.Printf("Error getting version: %v (Maybe table doesn't exist?)\n", err)
	} else {
		fmt.Printf("Current DB Version: %d\n", version)
	}

	var count int
	err = db.Get(&count, "SELECT COUNT(*) FROM activities")
	if err != nil {
		fmt.Printf("Error counting activities: %v\n", err)
	} else {
		fmt.Printf("Number of activity records: %d\n", count)
	}

	tables := []string{"app_settings", "steam_mapping", "achievements", "rename_map"}
	for _, table := range tables {
		var exists int
		err = db.Get(&exists, "SELECT COUNT(*) FROM sqlite_master WHERE type='table' AND name=?", table)
		if exists > 0 {
			fmt.Printf("Table '%s' exists.\n", table)
		} else {
			fmt.Printf("Table '%s' MISSING.\n", table)
		}
	}
}
