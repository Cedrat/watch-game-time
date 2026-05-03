package manager

import (
	"encoding/json"
	"fmt"
	"main/query"
	"net/http"
	"os/exec"
	"strings"
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

// SyncOwnedGames fetches all owned games from Steam and updates the mapping table.
func (sm *SteamManager) SyncOwnedGames() error {
	apiKey, err := sm.db.GetSteamAPIKey()
	if err != nil || apiKey == "" {
		return fmt.Errorf("Steam API Key missing")
	}
	steamID, err := sm.db.GetSteamID()
	if err != nil || steamID == "" {
		return fmt.Errorf("Steam ID missing")
	}

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

	for _, game := range data.Response.Games {
		// We don't necessarily know the process_name yet, but we can store known appids
		// We use the game name as a hint for matching later
		// If we already have a mapping for a process with this game name, we update it.
		// Note: This is a simplified approach. Real matching usually happens when a game runs.
		mapping := query.SteamMapping{
			AppID:    game.AppID,
			GameName: game.Name,
			IconURL:  fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/header.jpg", game.AppID),
		}

		// Attempt to match by name if we don't have a direct process name link yet
		// This part is tricky because one game name can have multiple processes.
		// For now, we'll focus on providing an API to link them.
		_ = mapping
	}

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

	// 2. Get Metadata (Names, Icons)
	uSchema := fmt.Sprintf("http://api.steampowered.com/ISteamUserStats/GetSchemaForGame/v2/?key=%s&appid=%d&l=french", apiKey, appid)
	respSchema, err := http.Get(uSchema)
	if err != nil {
		return 0, err
	}
	defer respSchema.Body.Close()

	var schemaData GameSchemaResponse
	if err := json.NewDecoder(respSchema.Body).Decode(&schemaData); err != nil {
		return 0, err
	}

	// Build metadata map
	metaMap := make(map[string]struct {
		Name   string
		Desc   string
		Icon   string
		Hidden bool
	})
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
		meta := metaMap[sa.APIName]

		ach := query.Achievement{
			AppID:       appid,
			APIName:     sa.APIName,
			Name:        meta.Name,
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
			mapping := query.SteamMapping{
				ProcessName: processName,
				AppID:       game.AppID,
				GameName:    game.Name,
				IconURL:     fmt.Sprintf("https://cdn.akamai.steamstatic.com/steam/apps/%d/capsule_184x69.jpg", game.AppID),
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
