package manager

import (
	_ "embed"
	"encoding/json"
	"sort"
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

//go:embed default_blacklist.json
var defaultBlacklistJSON []byte

// Structure pour gérer les listes en mémoire
type ListManager struct {
	db        *sqlx.DB
	whitelist map[string]struct{} // Utilisation d'un map pour des lookups O(1)
	blacklist map[string]struct{}
	mutex     sync.RWMutex
}

// Créer un nouveau gestionnaire de liste
func NewListManager(db *sqlx.DB) (*ListManager, error) {
	lm := &ListManager{
		db:        db,
		whitelist: make(map[string]struct{}),
		blacklist: make(map[string]struct{}),
	}

	// Charger la blacklist par défaut si elle est vide en base
	var count int
	err := db.Get(&count, "SELECT COUNT(*) FROM blacklist")
	if err == nil && count == 0 {
		var defaults []string
		if err := json.Unmarshal(defaultBlacklistJSON, &defaults); err == nil {
			for _, name := range defaults {
				_, _ = db.Exec("INSERT OR IGNORE INTO blacklist (name) VALUES (?)", name)
			}
		}
	}

	// Charger la whitelist par défaut si elle est vide en base
	var whitelistCount int
	if err := db.Get(&whitelistCount, "SELECT COUNT(*) FROM whitelist"); err == nil && whitelistCount == 0 {
		_, _ = db.Exec("INSERT OR IGNORE INTO whitelist (name) VALUES (?)", "steamapps")
	}

	// Charger les listes initiales
	if err := lm.RefreshLists(); err != nil {
		return nil, err
	}

	return lm, nil
}

// Rafraîchir les listes depuis la base de données
func (lm *ListManager) RefreshLists() error {
	// Récupérer les listes depuis la base de données
	var whitelistedNames, blacklistedNames []string

	err := lm.db.Select(&whitelistedNames, "SELECT name FROM whitelist")
	if err != nil {
		return err
	}

	err = lm.db.Select(&blacklistedNames, "SELECT name FROM blacklist")
	if err != nil {
		return err
	}

	// Mettre à jour les maps en mémoire
	lm.mutex.Lock()
	defer lm.mutex.Unlock()

	// Recréer les maps
	newWhitelist := make(map[string]struct{}, len(whitelistedNames))
	for _, name := range whitelistedNames {
		newWhitelist[name] = struct{}{}
	}

	newBlacklist := make(map[string]struct{}, len(blacklistedNames))
	for _, name := range blacklistedNames {
		newBlacklist[name] = struct{}{}
	}

	// Remplacer les anciennes listes
	lm.whitelist = newWhitelist
	lm.blacklist = newBlacklist

	return nil
}

// Vérifier si un chemin contient une entrée de la whitelist
func (lm *ListManager) IsWhitelisted(path string) bool {
	lm.mutex.RLock()
	defer lm.mutex.RUnlock()

	// Parcourir toutes les entrées de la whitelist
	for entry := range lm.whitelist {
		if strings.Contains(path, entry) {
			return true
		}
	}
	return false
}

// Vérifier si un chemin contient une entrée de la blacklist
func (lm *ListManager) IsBlacklisted(path string) bool {
	lm.mutex.RLock()
	defer lm.mutex.RUnlock()

	// Parcourir toutes les entrées de la blacklist
	for entry := range lm.blacklist {
		if strings.Contains(path, entry) {
			return true
		}
	}
	return false
}

// Ajouter à la whitelist et mettre à jour la mémoire
func (lm *ListManager) AddToWhitelist(name string) error {
	// Insérer dans la base de données
	_, err := lm.db.Exec("INSERT INTO whitelist (name) VALUES (?)", name)
	if err != nil {
		return err
	}

	// Mettre à jour la liste en mémoire
	lm.mutex.Lock()
	lm.whitelist[name] = struct{}{}
	lm.mutex.Unlock()

	return nil
}

// Retirer de la whitelist et mettre à jour la mémoire
func (lm *ListManager) RemoveFromWhitelist(name string) error {
	// Supprimer de la base de données
	_, err := lm.db.Exec("DELETE FROM whitelist WHERE name = ?", name)
	if err != nil {
		return err
	}

	// Mettre à jour la liste en mémoire
	lm.mutex.Lock()
	delete(lm.whitelist, name)
	lm.mutex.Unlock()

	return nil
}

// Ajouter à la blacklist et mettre à jour la mémoire
func (lm *ListManager) AddToBlacklist(name string) error {
	// Insérer dans la base de données
	_, err := lm.db.Exec("INSERT OR IGNORE INTO blacklist (name) VALUES (?)", name)
	if err != nil {
		return err
	}

	// Mettre à jour la liste en mémoire
	lm.mutex.Lock()
	lm.blacklist[name] = struct{}{}
	lm.mutex.Unlock()

	return nil
}

// Retirer de la blacklist et mettre à jour la mémoire
func (lm *ListManager) RemoveFromBlacklist(name string) error {
	// Supprimer de la base de données
	_, err := lm.db.Exec("DELETE FROM blacklist WHERE name = ?", name)
	if err != nil {
		return err
	}

	// Mettre à jour la liste en mémoire
	lm.mutex.Lock()
	delete(lm.blacklist, name)
	lm.mutex.Unlock()

	return nil
}

// ExportBlacklist renvoie la liste actuelle des entrées de la blacklist triée
func (lm *ListManager) ExportBlacklist() []string {
	lm.mutex.RLock()
	defer lm.mutex.RUnlock()

	res := make([]string, 0, len(lm.blacklist))
	for name := range lm.blacklist {
		res = append(res, name)
	}
	sort.Strings(res)
	return res
}

// ImportBlacklist ajoute une liste de noms à la blacklist
func (lm *ListManager) ImportBlacklist(names []string) error {
	for _, name := range names {
		name = strings.TrimSpace(name)
		if name == "" {
			continue
		}
		if err := lm.AddToBlacklist(name); err != nil {
			return err
		}
	}
	return nil
}
