package query

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"time"

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
            date TEXT NOT NULL,
            first_launch BOOLEAN DEFAULT FALSE
        )
    `)
		if err != nil {
			return nil, err
		}

		// Créer les tables de whitelist/blacklist/rename pour une nouvelle base
		_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS whitelist (
		name TEXT PRIMARY KEY
	);

	CREATE TABLE IF NOT EXISTS blacklist (
		name TEXT PRIMARY KEY
	);

	CREATE TABLE IF NOT EXISTS rename_map (
		original_name TEXT PRIMARY KEY,
		display_name TEXT NOT NULL
	);
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

		// Create finished_games table for fresh DB
		_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS finished_games (
		name TEXT PRIMARY KEY,
		finished_at TEXT
	);
	`)
		if err != nil {
			return nil, err
		}

		// Create first_launch_override table for fresh DB
		_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS first_launch_override (
		name TEXT PRIMARY KEY,
		first_date TEXT
	);
	`)
		if err != nil {
			return nil, err
		}

		// Set latest version (8) for fresh DB
		_, err = db.Exec(`
			UPDATE database_version SET db_version=8;
		`)
		if err != nil {
			return nil, err
		}

		// Créer des index
		_, err = db.Exec(`
        CREATE INDEX IF NOT EXISTS idx_activities_date ON activities(date);
        CREATE INDEX IF NOT EXISTS idx_activities_unique ON activities(process_name, start_time, end_time);
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

	if dbVersion < 3 {
		_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS rename_map (
			original_name TEXT PRIMARY KEY,
			display_name TEXT NOT NULL
		);

		UPDATE database_version SET db_version=3;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 3: %w", err)
		}
		fmt.Println("db version up to 3")
	}

	if dbVersion < 4 {
		_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS finished_games (
			name TEXT PRIMARY KEY
		);

		UPDATE database_version SET db_version=4;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 4: %w", err)
		}
		fmt.Println("db version up to 4")
	}

	if dbVersion < 5 {
		_, err = db.Exec(`
		ALTER TABLE finished_games ADD COLUMN finished_at TEXT;
		UPDATE database_version SET db_version=5;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 5: %w", err)
		}
		fmt.Println("db version up to 5")
	}

	if dbVersion < 6 {
		_, err = db.Exec(`
		CREATE TABLE IF NOT EXISTS first_launch_override (
			name TEXT PRIMARY KEY,
			first_date TEXT
		);
		UPDATE database_version SET db_version=6;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 6: %w", err)
		}
		fmt.Println("db version up to 6")
	}

	if dbVersion < 7 {
		_, err = db.Exec(`
		CREATE INDEX IF NOT EXISTS idx_activities_unique ON activities(process_name, start_time, end_time);
		UPDATE database_version SET db_version=7;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 7: %w", err)
		}
		fmt.Println("db version up to 7")
	}

	if dbVersion < 8 {
		// Retro-compatibility: split historical sessions spanning midnight into day-bounded segments
		type actRow struct {
			ID          int64   `db:"id"`
			ProcessName string  `db:"process_name"`
			Start       string  `db:"start_time"`
			End         string  `db:"end_time"`
			Duration    float64 `db:"duration"`
			DateStr     string  `db:"date"`
			FirstLaunch bool    `db:"first_launch"`
		}
		rows := []actRow{}
		q := `SELECT id, process_name, start_time, end_time, duration, date, first_launch
		      FROM activities
		      WHERE substr(start_time,1,10) != substr(end_time,1,10)
		         OR date != substr(start_time,1,10)`
		if err := db.Select(&rows, q); err != nil {
			return fmt.Errorf("updateDb version 8 select: %w", err)
		}

		if len(rows) > 0 {
			trx := db.MustBegin()
			for _, r := range rows {
				start, err1 := time.Parse(time.RFC3339, r.Start)
				end, err2 := time.Parse(time.RFC3339, r.End)
				if err1 != nil || err2 != nil || !end.After(start) {
					// If parse fails or invalid, skip splitting; keep original row
					continue
				}
				currentStart := start
				firstFlag := r.FirstLaunch
				for currentStart.Before(end) {
					year, month, day := currentStart.Date()
					loc := currentStart.Location()
					nextDayStart := time.Date(year, month, day, 0, 0, 0, 0, loc).Add(24 * time.Hour)
					segmentEnd := end
					if end.After(nextDayStart) {
						segmentEnd = nextDayStart
					}
					if segmentEnd.After(currentStart) {
						dateStr := currentStart.Format("2006-01-02")
						_, err := trx.Exec(`
							INSERT INTO activities (process_name, start_time, end_time, duration, date, first_launch)
							VALUES (?, ?, ?, ?, ?, ?)
						`, r.ProcessName, currentStart.Format(time.RFC3339), segmentEnd.Format(time.RFC3339), segmentEnd.Sub(currentStart).Seconds(), dateStr, firstFlag)
						if err != nil {
							trx.Rollback()
							return fmt.Errorf("updateDb version 8 insert: %w", err)
						}
						firstFlag = false
					}
					currentStart = segmentEnd
				}
				// Delete original row after inserting the split segments
				if _, err := trx.Exec(`DELETE FROM activities WHERE id = ?`, r.ID); err != nil {
					trx.Rollback()
					return fmt.Errorf("updateDb version 8 delete: %w", err)
				}
			}
			if err := trx.Commit(); err != nil {
				return fmt.Errorf("updateDb version 8 commit: %w", err)
			}
		}

		_, err = db.Exec(`UPDATE database_version SET db_version=8;`)
		if err != nil {
			return fmt.Errorf("updateDb version 8: %w", err)
		}
		fmt.Println("db version up to 8 (historical sessions split)")
	}

	err = tx.Commit()
	if err != nil {
		tx.Rollback()
		return fmt.Errorf("updateDb: error at commit rollback: %w", err)
	}
	return nil
}
