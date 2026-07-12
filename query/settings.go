package query

import (
	"database/sql"
)

// SetSetting saves or updates a setting in the app_settings table.
func (db *Database) SetSetting(key, value string) error {
	_, err := db.Exec(`
		INSERT INTO app_settings (key, value)
		VALUES (?, ?)
		ON CONFLICT(key) DO UPDATE SET value = excluded.value`,
		key, value,
	)
	return err
}

// GetSetting retrieves a setting value by its key.
func (db *Database) GetSetting(key string) (string, error) {
	var value string
	err := db.Get(&value, "SELECT value FROM app_settings WHERE key = ?", key)
	if err == sql.ErrNoRows {
		return "", nil
	}
	return value, err
}

// GetSettingDefault retrieves a setting value or returns a default if not found.
func (db *Database) GetSettingDefault(key, defaultValue string) string {
	val, err := db.GetSetting(key)
	if err != nil || val == "" {
		return defaultValue
	}
	return val
}

// GetNotificationEnabled returns true if notifications are enabled (default: true).
func (db *Database) GetNotificationEnabled() bool {
	val := db.GetSettingDefault("notifications_enabled", "true")
	return val == "true"
}

// SetNotificationEnabled enables or disables notifications.
func (db *Database) SetNotificationEnabled(enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	return db.SetSetting("notifications_enabled", val)
}

// GetSteamAPIKey retrieves the saved Steam API Key.
func (db *Database) GetSteamAPIKey() (string, error) {
	return db.GetSetting("steam_api_key")
}

// GetSteamID retrieves the saved Steam ID.
func (db *Database) GetSteamID() (string, error) {
	return db.GetSetting("steam_id")
}

// GetSteamEnabled retrieves the Steam activation status.
func (db *Database) GetSteamEnabled() bool {
	val, _ := db.GetSetting("steam_enabled")
	if val == "" {
		return true // Enabled by default
	}
	return val == "true"
}

// SetSteamEnabled saves the Steam activation status.
func (db *Database) SetSteamEnabled(enabled bool) error {
	val := "false"
	if enabled {
		val = "true"
	}
	return db.SetSetting("steam_enabled", val)
}
