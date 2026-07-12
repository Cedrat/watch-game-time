package query

import (
	"fmt"
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
	// Utilise le dossier standard des configurations utilisateur (AppData/Roaming sur Windows)
	configDir, err := os.UserConfigDir()
	if err != nil {
		// Fallback sur le dossier local si erreur
		return "."
	}

	monAppDir := filepath.Join(configDir, "SteamTracker")

	// Créer le dossier si nécessaire
	_ = os.MkdirAll(monAppDir, 0755)

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
		return nil, fmt.Errorf("InitDatabase: %w", err)
	}
	if exist {
		fmt.Printf("[DB] Database found, checking for updates...\n")
		err = db.updateDb()
		if err != nil {
			fmt.Printf("[DB] CRITICAL: Update failed: %v\n", err)
			return nil, err
		}
	} else {
		fmt.Println("[DB] Initializing new database...")

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

		// Create steam integration tables
		_, err = db.Exec(`
	CREATE TABLE IF NOT EXISTS app_settings (
		key TEXT PRIMARY KEY,
		value TEXT
	);

	CREATE TABLE IF NOT EXISTS steam_mapping (
		process_name TEXT PRIMARY KEY,
		appid INTEGER NOT NULL,
		game_name TEXT,
		icon_url TEXT
	);

	CREATE TABLE IF NOT EXISTS achievements (
		appid INTEGER NOT NULL,
		apiname TEXT NOT NULL,
		name TEXT,
		description TEXT,
		icon_url TEXT,
		unlocked_at INTEGER,
		is_hidden BOOLEAN DEFAULT FALSE,
		PRIMARY KEY (appid, apiname)
	);
	`)
		if err != nil {
			return nil, err
		}

		// Set latest version (9) for fresh DB
		_, err = db.Exec(`
			UPDATE database_version SET db_version=9;
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
		return fmt.Errorf("updateDb version fetch: %w", err)
	}

	fmt.Printf("[DB] Current version: %d\n", dbVersion)

	tx, err := db.Beginx()
	if err != nil {
		return fmt.Errorf("updateDb: failed to begin transaction: %w", err)
	}
	defer tx.Rollback()

	if dbVersion < 1 {
		fmt.Println("[DB] Updating to version 1...")
		_, err = tx.Exec(`ALTER TABLE activities DROP COLUMN window_title`)
		if err != nil {
			return fmt.Errorf("updateDb version 1 drop: %w", err)
		}
		_, err = tx.Exec(`ALTER TABLE activities ADD COLUMN first_launch BOOLEAN DEFAULT FALSE`)
		if err != nil {
			return fmt.Errorf("updateDb version 1 add: %w", err)
		}
		_, err = tx.Exec(`UPDATE database_version SET db_version=1`)
		if err != nil {
			return fmt.Errorf("updateDb version 1 update: %w", err)
		}
	}

	if dbVersion < 2 {
		fmt.Println("[DB] Updating to version 2...")
		_, err = tx.Exec(`
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
	}

	if dbVersion < 3 {
		fmt.Println("[DB] Updating to version 3...")
		_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS rename_map (
			original_name TEXT PRIMARY KEY,
			display_name TEXT NOT NULL
		);

		UPDATE database_version SET db_version=3;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 3: %w", err)
		}
	}

	if dbVersion < 4 {
		fmt.Println("[DB] Updating to version 4...")
		_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS finished_games (
			name TEXT PRIMARY KEY
		);

		UPDATE database_version SET db_version=4;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 4: %w", err)
		}
	}

	if dbVersion < 5 {
		fmt.Println("[DB] Updating to version 5...")
		_, err = tx.Exec(`
		ALTER TABLE finished_games ADD COLUMN finished_at TEXT;
		UPDATE database_version SET db_version=5;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 5: %w", err)
		}
	}

	if dbVersion < 6 {
		fmt.Println("[DB] Updating to version 6...")
		_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS first_launch_override (
			name TEXT PRIMARY KEY,
			first_date TEXT
		);
		UPDATE database_version SET db_version=6;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 6: %w", err)
		}
	}

	if dbVersion < 7 {
		fmt.Println("[DB] Updating to version 7...")
		_, err = tx.Exec(`
		CREATE INDEX IF NOT EXISTS idx_activities_unique ON activities(process_name, start_time, end_time);
		UPDATE database_version SET db_version=7;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 7: %w", err)
		}
	}

	if dbVersion < 8 {
		fmt.Println("[DB] Updating to version 8 (splitting historical sessions)...")
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
		if err := tx.Select(&rows, q); err != nil {
			return fmt.Errorf("updateDb version 8 select: %w", err)
		}

		if len(rows) > 0 {
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
						_, err := tx.Exec(`
							INSERT INTO activities (process_name, start_time, end_time, duration, date, first_launch)
							VALUES (?, ?, ?, ?, ?, ?)
						`, r.ProcessName, currentStart.Format(time.RFC3339), segmentEnd.Format(time.RFC3339), segmentEnd.Sub(currentStart).Seconds(), dateStr, firstFlag)
						if err != nil {
							return fmt.Errorf("updateDb version 8 insert: %w", err)
						}
						firstFlag = false
					}
					currentStart = segmentEnd
				}
				// Delete original row after inserting the split segments
				if _, err := tx.Exec(`DELETE FROM activities WHERE id = ?`, r.ID); err != nil {
					return fmt.Errorf("updateDb version 8 delete: %w", err)
				}
			}
		}

		_, err = tx.Exec(`UPDATE database_version SET db_version=8;`)
		if err != nil {
			return fmt.Errorf("updateDb version 8: %w", err)
		}
	}

	if dbVersion < 9 {
		fmt.Println("[DB] Updating to version 9 (Steam integration)...")
		_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS app_settings (
			key TEXT PRIMARY KEY,
			value TEXT
		);

		CREATE TABLE IF NOT EXISTS steam_mapping (
			process_name TEXT PRIMARY KEY,
			appid INTEGER NOT NULL,
			game_name TEXT,
			icon_url TEXT
		);

		CREATE TABLE IF NOT EXISTS achievements (
			appid INTEGER NOT NULL,
			apiname TEXT NOT NULL,
			name TEXT,
			description TEXT,
			icon_url TEXT,
			unlocked_at INTEGER,
			is_hidden BOOLEAN DEFAULT FALSE,
			PRIMARY KEY (appid, apiname)
		);

		UPDATE database_version SET db_version=9;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 9: %w", err)
		}
	}

	if dbVersion < 10 {
		fmt.Println("[DB] Updating to version 10 (Local Steam Games Cache)...")
		_, err = tx.Exec(`
		CREATE TABLE IF NOT EXISTS steam_owned_games (
			appid INTEGER PRIMARY KEY,
			game_name TEXT,
			icon_url TEXT
		);
		UPDATE database_version SET db_version=10;
		`)
		if err != nil {
			return fmt.Errorf("updateDb version 10: %w", err)
		}
	}

	err = tx.Commit()
	if err != nil {
		return fmt.Errorf("updateDb: error at commit: %w", err)
	}
	fmt.Println("[DB] Migration successful")
	return nil
}
