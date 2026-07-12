package statsgraphsteam

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strconv"
	"strings"
)

const steamBase = "https://api.steampowered.com"
const storeBase = "https://store.steampowered.com/api"

// Achievement représente un succès Steam fusionné (schéma + % global).
type Achievement struct {
	Name        string  `json:"name"`        // identifiant interne (ex: ACH_KILL_100)
	DisplayName string  `json:"displayName"` // nom affiché
	Description string  `json:"description"` // description
	Icon        string  `json:"icon"`        // URL icône débloqué
	IconGray    string  `json:"iconGray"`    // URL icône verrouillé
	Hidden      int     `json:"hidden"`
	Percent     float64 `json:"percent"` // % global de joueurs ayant débloqué
	Abandon     float64 `json:"abandon"` // % d'abandon = 100 - percent
}

// schemaResponse reflet de GetSchemaForGame/v2.
type schemaResponse struct {
	Game struct {
		GameName           string `json:"gameName"`
		AvailableGameStats *struct {
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

// percentResponse reflet de GetGlobalAchievementPercentagesForApp/v2.
// Steam renvoie parfois "percent" comme une chaîne (ex: "39.7").
type percentResponse struct {
	AchievementPercentages struct {
		Achievements []struct {
			Name    string    `json:"name"`
			Percent flexFloat `json:"percent"`
		} `json:"achievements"`
	} `json:"achievementpercentages"`
}

// flexFloat décode indifféremment un nombre ou une chaîne numérique.
type flexFloat float64

func (f *flexFloat) UnmarshalJSON(b []byte) error {
	s := string(b)
	// Si c'est une chaîne quotée, on retire les guillemets.
	if len(s) >= 2 && s[0] == '"' && s[len(s)-1] == '"' {
		s = s[1 : len(s)-1]
	}
	v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
	if err != nil {
		return err
	}
	*f = flexFloat(v)
	return nil
}

// GetAchievements récupère et fusionne schéma + pourcentages globaux.
// La clé API Steam est fournie par l'appelant (partagée avec le reste de l'application).
func GetAchievements(appid string, lang string, apiKey string) (string, []Achievement, error) {
	if apiKey == "" {
		return "", nil, fmt.Errorf("clé API Steam non configurée. Ouvrez la page Configuration pour la définir.")
	}

	schema, err := fetchSchema(appid, apiKey, lang)
	if err != nil {
		return "", nil, fmt.Errorf("schéma: %w", err)
	}

	gameName := schema.Game.GameName
	var achs []Achievement
	if schema.Game.AvailableGameStats != nil {
		for _, a := range schema.Game.AvailableGameStats.Achievements {
			achs = append(achs, Achievement{
				Name:        a.Name,
				DisplayName: a.DisplayName,
				Description: a.Description,
				Icon:        a.Icon,
				IconGray:    a.IconGray,
				Hidden:      a.Hidden,
			})
		}
	}

	// Pourcentages globaux (ne nécessite pas de clé mais on l'envoie quand même).
	pct, err := fetchPercentages(appid)
	if err != nil {
		// Non fatal : on renvoie juste le schéma.
		return gameName, achs, nil
	}
	byName := map[string]float64{}
	for _, p := range pct.AchievementPercentages.Achievements {
		byName[p.Name] = float64(p.Percent)
	}
	for i := range achs {
		if p, ok := byName[achs[i].Name]; ok {
			achs[i].Percent = p
			achs[i].Abandon = 100 - p
		}
	}
	return gameName, achs, nil
}

func fetchSchema(appid, apiKey, lang string) (*schemaResponse, error) {
	if lang == "" {
		lang = "french"
	}
	q := url.Values{}
	q.Set("appid", appid)
	q.Set("key", apiKey)
	q.Set("l", lang)
	endpoint := steamBase + "/ISteamUserStats/GetSchemaForGame/v2/?" + q.Encode()

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("steam schema HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var sr schemaResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, err
	}
	return &sr, nil
}

func fetchPercentages(appid string) (*percentResponse, error) {
	q := url.Values{}
	q.Set("gameid", appid)
	endpoint := steamBase + "/ISteamUserStats/GetGlobalAchievementPercentagesForApp/v2/?" + q.Encode()

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("steam pct HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}
	var pr percentResponse
	if err := json.Unmarshal(body, &pr); err != nil {
		return nil, err
	}
	return &pr, nil
}

// SearchResult représente un jeu trouvé via la recherche Steam Store.
type SearchResult struct {
	AppID int    `json:"appid"`
	Name  string `json:"name"`
	Img   string `json:"img"`
}

// storeSearchResponse reflet de l'API storesearch de Steam.
type storeSearchResponse struct {
	Total int `json:"total"`
	Items []struct {
		Type      string `json:"type"`
		Name      string `json:"name"`
		ID        int    `json:"id"`
		TinyImage string `json:"tiny_image"`
	} `json:"items"`
}

// SearchGames recherche des jeux par nom via l'API Steam Store (sans clé API).
func SearchGames(term, lang string) ([]SearchResult, error) {
	if term == "" {
		return nil, nil
	}
	if lang == "" {
		lang = "french"
	}
	cc := "FR"
	if lang == "english" {
		cc = "US"
	}

	q := url.Values{}
	q.Set("term", term)
	q.Set("l", lang)
	q.Set("cc", cc)
	endpoint := storeBase + "/storesearch/?" + q.Encode()

	resp, err := http.Get(endpoint)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != 200 {
		return nil, fmt.Errorf("steam storesearch HTTP %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var sr storeSearchResponse
	if err := json.Unmarshal(body, &sr); err != nil {
		return nil, err
	}

	results := make([]SearchResult, 0, len(sr.Items))
	for _, it := range sr.Items {
		if it.Type != "app" || it.ID == 0 {
			continue
		}
		results = append(results, SearchResult{
			AppID: it.ID,
			Name:  it.Name,
			Img:   it.TinyImage,
		})
	}
	return results, nil
}
