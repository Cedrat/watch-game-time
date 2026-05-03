package query

import (
	"database/sql"
)

// SteamMapping represents the link between a local process and a Steam App
type SteamMapping struct {
	ProcessName string `db:"process_name"`
	AppID       int    `db:"appid"`
	GameName    string `db:"game_name"`
	IconURL     string `db:"icon_url"`
}

// Achievement represents a Steam achievement
type Achievement struct {
	AppID       int    `db:"appid" json:"appid"`
	APIName     string `db:"apiname" json:"apiname"`
	Name        string `db:"name" json:"name"`
	Description string `db:"description" json:"description"`
	IconURL     string `db:"icon_url" json:"icon_url"`
	UnlockedAt  int64  `db:"unlocked_at" json:"unlocked_at"`
	IsHidden    bool   `db:"is_hidden" json:"is_hidden"`
}

// SetSteamMapping saves or updates the mapping for a process
func (db *Database) SetSteamMapping(mapping SteamMapping) error {
	_, err := db.Exec(`
		INSERT INTO steam_mapping (process_name, appid, game_name, icon_url)
		VALUES (?, ?, ?, ?)
		ON CONFLICT(process_name) DO UPDATE SET
			appid = excluded.appid,
			game_name = excluded.game_name,
			icon_url = excluded.icon_url`,
		mapping.ProcessName, mapping.AppID, mapping.GameName, mapping.IconURL,
	)
	return err
}

// GetSteamMapping returns the Steam mapping for a given process name
func (db *Database) GetSteamMapping(processName string) (*SteamMapping, error) {
	var m SteamMapping
	err := db.Get(&m, "SELECT * FROM steam_mapping WHERE process_name = ?", processName)
	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}
	return &m, nil
}

// SaveAchievements persists a list of achievements in bulk
func (db *Database) SaveAchievements(achievements []Achievement) error {
	if len(achievements) == 0 {
		return nil
	}

	tx, err := db.Beginx()
	if err != nil {
		return err
	}
	defer tx.Rollback()

	query := `
		INSERT INTO achievements (appid, apiname, name, description, icon_url, unlocked_at, is_hidden)
		VALUES (?, ?, ?, ?, ?, ?, ?)
		ON CONFLICT(appid, apiname) DO UPDATE SET
			name = excluded.name,
			description = excluded.description,
			icon_url = excluded.icon_url,
			unlocked_at = excluded.unlocked_at,
			is_hidden = excluded.is_hidden`

	for _, ach := range achievements {
		_, err := tx.Exec(query,
			ach.AppID, ach.APIName, ach.Name, ach.Description,
			ach.IconURL, ach.UnlockedAt, ach.IsHidden,
		)
		if err != nil {
			return err
		}
	}

	return tx.Commit()
}

// GetAchievementsForApp returns all achievements for a specific Steam AppID
func (db *Database) GetAchievementsForApp(appid int) ([]Achievement, error) {
	var achs []Achievement
	err := db.Select(&achs, "SELECT * FROM achievements WHERE appid = ? ORDER BY unlocked_at DESC", appid)
	return achs, err
}

// GetRecentAchievements returns the last X unlocked achievements across all games
func (db *Database) GetRecentAchievements(limit int) ([]Achievement, error) {
	var achs []Achievement
	err := db.Select(&achs, `
		SELECT * FROM achievements
		WHERE unlocked_at > 0
		ORDER BY unlocked_at DESC
		LIMIT ?`, limit)
	return achs, err
}

// GetUnlockedCount returns the number of unlocked achievements for an app
func (db *Database) GetUnlockedCount(appid int) (int, int, error) {
	var total, unlocked int
	err := db.Get(&total, "SELECT COUNT(*) FROM achievements WHERE appid = ?", appid)
	if err != nil {
		return 0, 0, err
	}
	err = db.Get(&unlocked, "SELECT COUNT(*) FROM achievements WHERE appid = ? AND unlocked_at > 0", appid)
	if err != nil {
		return 0, 0, err
	}
	return unlocked, total, nil
}
