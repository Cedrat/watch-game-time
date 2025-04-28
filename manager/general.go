package manager

import (
	"strings"
	"sync"

	"github.com/jmoiron/sqlx"
	_ "github.com/mattn/go-sqlite3"
)

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
	_, err := lm.db.Exec("INSERT INTO blacklist (name) VALUES (?)", name)
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

// Votre fonction modifiée
