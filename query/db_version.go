package query

import (
	"fmt"
	"log"
	"os"
	"path/filepath"

	"github.com/jmoiron/sqlx"
)

const (
	TableDatabaseVersion = "database_version"
)

func (db *Database) GetDbVersion() (int, error) {
	var dbVersion int
	query := "SELECT db_version FROM database_version LIMIT 1"
	err := db.Get(&dbVersion, query)
	if err != nil {
		return 0, fmt.Errorf("GetDbVersion: %w", err)
	}
	return dbVersion, nil
}

func (db *Database) TableExists(tableName string) (bool, error) {
	query := `
		SELECT count(name) 
		FROM sqlite_master 
		WHERE type='table' AND name=?
	`

	var count int
	err := db.QueryRow(query, tableName).Scan(&count)
	if err != nil {
		return false, err
	}

	return count > 0, nil
}

func getPathFileData() string {
	// Méthode 1: Utiliser la variable d'environnement APPDATA directement
	appData := os.Getenv("APPDATA")
	fmt.Println("Chemin APPDATA:", appData)

	// Méthode 2: Utiliser UserConfigDir() (Go 1.13+)
	configDir, err := os.UserConfigDir()
	if err != nil {
		fmt.Println("Erreur lors de la récupération du dossier de configuration:", err)
	} else {
		fmt.Println("Dossier de configuration utilisateur:", configDir)
	}

	// Exemple: Créer un chemin vers un dossier spécifique dans APPDATA
	monAppDir := filepath.Join(appData, ".steam_watcher")

	// Créer le dossier si nécessaire
	err = os.MkdirAll(monAppDir, 0755)
	if err != nil {
		fmt.Println("Erreur lors de la création du dossier:", err)
	} else {
		fmt.Println("Dossier application créé:", monAppDir)
	}
	return monAppDir
}

func InitDatabase() (*Database, error) {
	saveFolder := getPathFileData()
	saveFile := filepath.Join(saveFolder, "activity_tracker.db")
	fmt.Println("test", saveFile)
	// Ouvrir ou créer la base de données
	dbTemp, err := sqlx.Open("sqlite", saveFile)
	if err != nil {
		return nil, err
	}

	db := NewDatabase(dbTemp)

	exist, err := db.TableExists(TableDatabaseVersion)
	if err != nil {
		log.Fatal(err)
	}
	if exist {
		db.updateDb()

	} else {

		// Créer la table si elle n'existe pas
		_, err = db.Exec(`
        CREATE TABLE IF NOT EXISTS activities (
            id INTEGER PRIMARY KEY AUTOINCREMENT,
            process_name TEXT NOT NULL,
            window_title TEXT,
            start_time DATETIME NOT NULL,
            end_time DATETIME NOT NULL,
            duration INTEGER NOT NULL,
            date TEXT NOT NULL
        )
    `)
		if err != nil {
			return nil, err
		}

		_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS database_version (
		db_version INTEGER default 0)`)

		if err != nil {
			return nil, err
		}

		_, err = db.Exec(`
		INSERT INTO database_version VALUES(0)
	`)
		if err != nil {
			return nil, err
		}

		// Créer un index pour les recherches par date
		_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_activities_date ON activities(date)
    `)

		if err != nil {
			return nil, err
		}
	}
	return db, nil
}

func (db *Database) updateDb() error {
	var err error
	dbVersion, err := db.GetDbVersion()
	if err != nil {
		return fmt.Errorf("updateDb: %w", err)
	}
	tx := db.MustBegin().Tx
	if dbVersion < 1 {
		_, err = tx.Exec(`ALTER TABLE activities DROP COLUMN window_title`)
		if err != nil {
			return fmt.Errorf("updateDb: %w", err)
		}
		_, err = tx.Exec(`ALTER TABLE activities ADD COLUMN first_launch BOOLEAN DEFAULT FALSE`)
		if err != nil {
			return fmt.Errorf("updateDb: %w", err)
		}
		_, err = tx.Exec(`UPDATE database_version SET db_version=1`)
		if err != nil {
			return fmt.Errorf("updateDb: %w", err)
		}
		fmt.Println("db version up to 1")
	}

	if dbVersion < 2 {
		_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS whitelist (
			name TEXT PRIMARY KEY
		);
	
		CREATE TABLE IF NOT EXISTS blacklist (
			name TEXT PRIMARY KEY
		);

		UPDATE database_version SET db_version=2;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 2: %w", err)
		}
		fmt.Println("db version up to 2")
	}

	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("updateDb: error at commit rollback: %w", err)
	}
	return nil
}
