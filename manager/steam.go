package manager

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"main/query"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"time"
)

type SteamManager struct {
	db *query.Database
}

func NewSteamManager(db *query.Database) *SteamManager {
	return &SteamManager{db: db}
}

// SteamGame represents a game returned by GetOwnedGames
type SteamGame struct {
	AppID                    int    `json:"appid"`
	Name                     string `json:"name"`
	ImgIconURL               string `json:"img_icon_url"`
	HasCommunityVisibleStats bool   `json:"has_community_visible_stats"`
}

type GetOwnedGamesResponse struct {
	Response struct {
		GameCount int         `json:"game_count"`
		Games     []SteamGame `json:"games"`
	} `json:"response"`
}

// updateSyncStatus records the result of a synchronization attempt.
func (sm *SteamManager) updateSyncStatus(status, errMsg string) {
	now := time.Now().Format(time.RFC3339)
	_ = sm.db.SetSetting("steam_last_sync_time", now)
	_ = sm.db.SetSetting("steam_last_sync_status", status)
	_ = sm.db.SetSetting("steam_last_sync_error", errMsg)
}

func (sm *SteamManager) downloadGameIcon(appid int) string {
	localPath := fmt.Sprintf("web/static/images/%d.jpg", appid)
	localURL := fmt.Sprintf("/static/images/%d.jpg", appid)

	if _, err := os.Stat(localPath); err == nil {
		return localURL
	}

	// Use the better vertical format
	remoteURL := fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/library_600x900.jpg", appid)

	resp, err := http.Get(remoteURL)
	if err != nil || resp.StatusCode != http.StatusOK {
		// Fallback to capsule if vertical not available
		remoteURL = fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/library_capsule.jpg", appid)
		resp, err = http.Get(remoteURL)
		if err != nil || resp.StatusCode != http.StatusOK {
			return ""
		}
	}
	defer resp.Body.Close()

	out, err := os.Create(localPath)
	if err != nil {
		log.Printf("[Steam] Error creating image file %s: %v", localPath, err)
		return ""
	}
	defer out.Close()

	_, err = io.Copy(out, resp.Body)
	if err != nil {
		log.Printf("[Steam] Error saving image %s: %v", localPath, err)
		return ""
	}

	return localURL
}

// SyncOwnedGames fetches all owned games from Steam and updates the local cache.
func (sm *SteamManager) SyncOwnedGames() error {
	if !sm.db.GetSteamEnabled() {
		return nil
	}
	apiKey, err := sm.db.GetSteamAPIKey()
	if err != nil || apiKey == "" {
		err = fmt.Errorf("Steam API Key missing")
		sm.updateSyncStatus("Error", err.Error())
		return err
	}
	steamID, err := sm.db.GetSteamID()
	if err != nil || steamID == "" {
		err = fmt.Errorf("Steam ID missing")
		sm.updateSyncStatus("Error", err.Error())
		return err
	}

	u := fmt.Sprintf("http://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=%s&steamid=%s&format=json&include_appinfo=true", apiKey, steamID)
	resp, err := http.Get(u)
	if err != nil {
		sm.updateSyncStatus("Error", err.Error())
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusTooManyRequests {
		err = fmt.Errorf("Steam API rate limit exceeded (429)")
		sm.updateSyncStatus("Rate Limited", err.Error())
		return err
	}

	var data GetOwnedGamesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		sm.updateSyncStatus("Error", err.Error())
		return err
	}

	// Update local cache of owned games
	tx, err := sm.db.Beginx()
	if err != nil {
		sm.updateSyncStatus("Error", "Database transaction failed")
		return err
	}
	defer tx.Rollback()

	for _, game := range data.Response.Games {
		iconURL := sm.downloadGameIcon(game.AppID)
		if iconURL == "" {
			iconURL = fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/library_capsule.jpg", game.AppID)
		}

		_, err := tx.Exec(`
			INSERT INTO steam_owned_games (appid, game_name, icon_url)
			VALUES (?, ?, ?)
			ON CONFLICT(appid) DO UPDATE SET
				game_name = excluded.game_name,
				icon_url = excluded.icon_url`,
			game.AppID, game.Name, iconURL)
		if err != nil {
			log.Printf("[Steam] Error caching game %d: %v", game.AppID, err)
		}
	}
	if err := tx.Commit(); err != nil {
		sm.updateSyncStatus("Error", "Failed to commit owned games cache")
		return err
	}

	sm.updateSyncStatus("Success", "")
	return nil
}

type SteamAchievement struct {
	APIName    string `json:"apiname"`
	Achieved   int    `json:"achieved"`
	UnlockTime int64  `json:"unlocktime"`
}

type GetPlayerAchievementsResponse struct {
	PlayerStats struct {
		GameName     string             `json:"gameName"`
		Achievements []SteamAchievement `json:"achievements"`
		Success      bool               `json:"success"`
	} `json:"playerstats"`
}

type GameSchemaResponse struct {
	Game struct {
		AvailableGameStats struct {
			Achievements []struct {
				Name         string `json:"name"`
				DefaultValue int    `json:"defaultvalue"`
				DisplayName  string `json:"displayName"`
				Hidden       int    `json:"hidden"`
				Description  string `json:"description"`
				Icon         string `json:"icon"`
				IconGray     string `json:"icongray"`
			} `json:"achievements"`
		} `json:"availableGameStats"`
	} `json:"game"`
}

// SyncAchievements fetches achievements for a specific game and updates the database.
// It returns the number of newly unlocked achievements found.
func (sm *SteamManager) SyncAchievements(appid int) (int, error) {
	if !sm.db.GetSteamEnabled() {
		return 0, nil
	}
	apiKey, _ := sm.db.GetSteamAPIKey()
	steamID, _ := sm.db.GetSteamID()
	if apiKey == "" || steamID == "" {
		return 0, nil
	}

	// 1. Get Unlock Status
	uStatus := fmt.Sprintf("http://api.steampowered.com/ISteamUserStats/GetPlayerAchievements/v0001/?appid=%d&key=%s&steamid=%s", appid, apiKey, steamID)
	respStatus, err := http.Get(uStatus)
	if err != nil {
		return 0, err
	}
	defer respStatus.Body.Close()

	var statusData GetPlayerAchievementsResponse
	if err := json.NewDecoder(respStatus.Body).Decode(&statusData); err != nil {
		return 0, err
	}

	if !statusData.PlayerStats.Success {
		return 0, nil
	}

	// 2. Get Metadata (Names, Icons) - Optional
	uSchema := fmt.Sprintf("http://api.steampowered.com/ISteamUserStats/GetSchemaForGame/v2/?key=%s&appid=%d&l=french", apiKey, appid)
	respSchema, err := http.Get(uSchema)

	metaMap := make(map[string]struct {
		Name   string
		Desc   string
		Icon   string
		Hidden bool
	})

	if err == nil {
		defer respSchema.Body.Close()
		var schemaData GameSchemaResponse
		if decodeErr := json.NewDecoder(respSchema.Body).Decode(&schemaData); decodeErr == nil {
			for _, a := range schemaData.Game.AvailableGameStats.Achievements {
				metaMap[a.Name] = struct {
					Name   string
					Desc   string
					Icon   string
					Hidden bool
				}{
					Name:   a.DisplayName,
					Desc:   a.Description,
					Icon:   a.Icon,
					Hidden: a.Hidden == 1,
				}
			}
		}
	} else {
		log.Printf("[Steam] Could not fetch schema for appid %d: %v", appid, err)
	}

	// 3. Compare with local DB to find "newly" tracked unlocks
	existing, _ := sm.db.GetAchievementsForApp(appid)
	unlockedSet := make(map[string]int64)
	for _, e := range existing {
		if e.UnlockedAt > 0 {
			unlockedSet[e.APIName] = e.UnlockedAt
		}
	}

	var toSave []query.Achievement
	newlyUnlockedCount := 0

	for _, sa := range statusData.PlayerStats.Achievements {
		meta, ok := metaMap[sa.APIName]
		name := meta.Name
		if !ok || name == "" {
			name = sa.APIName
		}

		ach := query.Achievement{
			AppID:       appid,
			APIName:     sa.APIName,
			Name:        name,
			Description: meta.Desc,
			IconURL:     meta.Icon,
			UnlockedAt:  sa.UnlockTime,
			IsHidden:    meta.Hidden,
		}

		if sa.Achieved == 1 {
			if _, exists := unlockedSet[sa.APIName]; !exists {
				// This is new to our database
				newlyUnlockedCount++
			}
		}
		toSave = append(toSave, ach)
	}

	if err := sm.db.SaveAchievements(toSave); err != nil {
		return 0, err
	}

	return newlyUnlockedCount, nil
}

// TryMatchProcessToSteam attempts to find an AppID for a given process and friendly name.
func (sm *SteamManager) TryMatchProcessToSteam(processName, friendlyName string) error {
	if !sm.db.GetSteamEnabled() {
		return nil
	}
	// If already mapped, skip
	m, _ := sm.db.GetSteamMapping(processName)
	if m != nil {
		return nil
	}

	apiKey, _ := sm.db.GetSteamAPIKey()
	if apiKey == "" {
		return nil
	}

	// Search on Steam via a public search or a predefined list if we had one.
	// As a fallback, we use the Steam Store search API (or just hope for a name match in GetOwnedGames)
	// For this implementation, we will search through owned games for a name match.
	steamID, _ := sm.db.GetSteamID()
	u := fmt.Sprintf("http://api.steampowered.com/IPlayerService/GetOwnedGames/v0001/?key=%s&steamid=%s&format=json&include_appinfo=true", apiKey, steamID)
	resp, err := http.Get(u)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	var data GetOwnedGamesResponse
	if err := json.NewDecoder(resp.Body).Decode(&data); err != nil {
		return err
	}

	cleanProcess := strings.ToLower(strings.TrimSuffix(processName, ".exe"))
	cleanFriendly := strings.ToLower(friendlyName)

	for _, game := range data.Response.Games {
		steamGameName := strings.ToLower(game.Name)
		if steamGameName == cleanProcess || steamGameName == cleanFriendly || strings.Contains(cleanFriendly, steamGameName) || strings.Contains(steamGameName, cleanFriendly) {
			// Found a potential match
			iconURL := sm.downloadGameIcon(game.AppID)
			if iconURL == "" {
				iconURL = fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/library_capsule.jpg", game.AppID)
			}

			mapping := query.SteamMapping{
				ProcessName: processName,
				AppID:       game.AppID,
				GameName:    game.Name,
				IconURL:     iconURL,
			}
			return sm.db.SetSteamMapping(mapping)
		}
	}

	return nil
}

// GetGameIcon returns the Steam icon URL for a process if available.
func (sm *SteamManager) GetGameIcon(processName string) string {
	m, err := sm.db.GetSteamMapping(processName)
	if err == nil && m != nil {
		return m.IconURL
	}
	return ""
}

// SendNotification sends a Windows toast notification using PowerShell.
// SyncSingleGame fetches achievements and updates the icon for a specific game.
func (sm *SteamManager) SyncSingleGame(appid int, processName string) (int, string, error) {
	if !sm.db.GetSteamEnabled() {
		return 0, "", fmt.Errorf("Steam is disabled")
	}

	// 1. Sync achievements
	newlyUnlocked, err := sm.SyncAchievements(appid)
	if err != nil {
		return 0, "", err
	}

	// 2. Update/Ensure Icon URL
	iconURL := sm.downloadGameIcon(appid)
	if iconURL == "" {
		iconURL = fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/library_capsule.jpg", appid)
	}

	// We need to update the mapping without erasing the GameName
	// Let's use a custom query to only update the icon_url
	_, err = sm.db.Exec(`
		UPDATE steam_mapping
		SET icon_url = ?
		WHERE process_name = ?`,
		iconURL, processName)

	return newlyUnlocked, iconURL, err
}

func (sm *SteamManager) SendNotification(title, message string) {
	// Simple PowerShell script to show a toast notification without extra dependencies
	psCommand := fmt.Sprintf(`
$title = "%s"
$msg = "%s"
[Windows.UI.Notifications.ToastNotificationManager, Windows.UI.Notifications, ContentType = WindowsRuntime] | Out-Null
$template = [Windows.UI.Notifications.ToastNotificationManager]::GetTemplateContent([Windows.UI.Notifications.ToastTemplateType]::ToastText02)
$toastXml = [xml]$template.GetXml()
$toastXml.GetElementsByTagName("text")[0].AppendChild($toastXml.CreateTextNode($title)) | Out-Null
$toastXml.GetElementsByTagName("text")[1].AppendChild($toastXml.CreateTextNode($msg)) | Out-Null
$xml = New-Object Windows.Data.Xml.Dom.XmlDocument
$xml.LoadXml($toastXml.OuterXml)
$toast = New-Object Windows.UI.Notifications.ToastNotification $xml
[Windows.UI.Notifications.ToastNotificationManager]::CreateToastNotifier("SteamTracker").Show($toast)
`, strings.ReplaceAll(title, `"`, "`\""), strings.ReplaceAll(message, `"`, "`\""))

	_ = exec.Command("powershell", "-NoProfile", "-Command", psCommand).Run()
}
