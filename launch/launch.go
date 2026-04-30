package launch

import (
	"fmt"
	"log"
	"main/entity"
	"main/manager"
	"main/query"
	"main/web"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/getlantern/systray"
	"github.com/shirou/gopsutil/process"
)

func StartProgramme() {
	systray.Run(onReady, onExit)
}

func onReady() {
	// Définir l'icône de l'application
	// On récupère le chemin de l'exécutable pour charger l'icône de manière fiable
	exePath, err := os.Executable()
	var iconPath string
	if err == nil {
		iconPath = filepath.Join(filepath.Dir(exePath), "icon.ico")
	} else {
		iconPath = "./icon.ico"
	}

	icon, err := os.ReadFile(iconPath)
	if err == nil {
		systray.SetIcon(icon)
	}

	// Définir le titre de l'icône (visible au survol)
	systray.SetTitle("SteamStracker")
	systray.SetTooltip("J'observe tes jeux")

	go mainProgram()

	// Ajouter des éléments de menu
	mOpenWeb := systray.AddMenuItem("Ouvrir l'interface Web", "Ouvrir http://localhost:8080 dans le navigateur")
	mQuit := systray.AddMenuItem("Quitter", "Quitter l'application")
	mInfo := systray.AddMenuItem("À propos", "Informations sur l'application")

	// Créer une goroutine pour gérer les clics sur les éléments de menu
	go func() {
		for {
			select {
			case <-mOpenWeb.ClickedCh:
				_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", "http://localhost:8080").Start()
			case <-mQuit.ClickedCh:
				systray.Quit()
				return
			case <-mInfo.ClickedCh:
				// Ici vous pouvez ouvrir une fenêtre d'information ou faire autre chose
				// Par exemple, utiliser un package comme walk ou lxn/win pour afficher une boîte de dialogue
				// Pour cet exemple, on se contente de changer le titre
				systray.SetTitle("Info: Version 1.0")
				// Remettre le titre original après 2 secondes
				go func() {
					time.Sleep(2 * time.Second)
					systray.SetTitle("Mon Application Go")
				}()
			}
		}
	}()
}

func onExit() {
	// Nettoyer les ressources si nécessaire
	os.Exit(0)
}

func mainProgram() {
	db, err := query.InitDatabase()
	if err != nil {
		log.Fatal(err)
	}
	processMonitor := NewProcessMonitor(db)
	lm, err := manager.NewListManager(db.DB)
	if err != nil {
		log.Fatal(err)
	}
	// Start web server
	go web.StartServer(db, lm)

	// Tenter de renommer les processus déjà en cours au lancement
	processMonitor.RunGlobalRename()

	for {
		processes, _ := process.Processes()
		time.Sleep(1 * time.Second)
		for _, p := range processes {
			if p == nil {
				continue
			}
			_, err := p.Exe()
			if err != nil {
				continue
			}
			// Vous pouvez aussi obtenir d'autres infos comme :
			// - p.Exe() pour le chemin de l'exécutable
			// - p.CreateTime() pour le moment où le processus a démarré
			// - p.Cmdline() pour la ligne de commande
			processMonitor.processCheck(p, lm)
		}
	}
}

type ProcessMonitor struct {
	trackers     map[int32]*ProcessTracker
	db           *query.Database
	trackerMutex sync.Mutex
}

func NewProcessMonitor(db *query.Database) *ProcessMonitor {
	return &ProcessMonitor{
		trackers: make(map[int32]*ProcessTracker),
		db:       db,
	}
}

func (pm *ProcessMonitor) StartTracking(pid int32) error {
	pm.trackerMutex.Lock()
	defer pm.trackerMutex.Unlock()

	// Vérifier si on suit déjà ce processus
	if _, exists := pm.trackers[pid]; exists {
		return nil
	}

	tracker, err := TrackProcess(pid)
	if err != nil {
		return err
	}

	tracker.OnProcessExit = func(t *ProcessTracker) {
		pm.handleProcessExit(t)
	}

	pm.trackers[pid] = tracker
	return nil
}

func (pm *ProcessMonitor) handleProcessExit(tracker *ProcessTracker) {
	// Retirer le tracker de la liste
	pm.trackerMutex.Lock()
	delete(pm.trackers, tracker.PID)
	pm.trackerMutex.Unlock()

	// Enregistrer l'activité
	pm.db.SaveActivity(entity.ActivityRecord{
		ProcessName: tracker.Name,
		StartTime:   tracker.StartTime,
		EndTime:     tracker.EndTime,
		Duration:    tracker.EndTime.Sub(tracker.StartTime),
	})
}

type ProcessTracker struct {
	PID           int32
	Name          string
	StartTime     time.Time
	EndTime       time.Time
	IsRunning     bool
	OnProcessExit func(tracker *ProcessTracker)
}

func TrackProcess(pid int32) (*ProcessTracker, error) {
	// Vérifier que le processus existe
	proc, err := process.NewProcess(pid)
	if err != nil {
		return nil, fmt.Errorf("processus non trouvé: %v", err)
	}

	// Obtenir le nom du processus
	name, err := proc.Name()
	if err != nil {
		return nil, fmt.Errorf("impossible d'obtenir le nom du processus: %v", err)
	}

	// Créer le tracker
	tracker := &ProcessTracker{
		PID:       pid,
		Name:      name,
		StartTime: time.Now(),
		IsRunning: true,
	}

	// Démarrer la surveillance en arrière-plan
	go func() {
		ticker := time.NewTicker(500 * time.Millisecond)
		defer ticker.Stop()

		for range ticker.C {
			if !processExists(pid) {
				tracker.IsRunning = false
				tracker.EndTime = time.Now()

				// Appeler le callback de notification si défini
				if tracker.OnProcessExit != nil {
					tracker.OnProcessExit(tracker)
				}

				break
			}
		}
	}()

	return tracker, nil
}

func processExists(pid int32) bool {
	_, err := process.NewProcess(pid)
	return err == nil
}

// Formater la durée de façon plus lisible
func formatDuration(d time.Duration) string {
	hours := int(d.Hours())
	minutes := int(d.Minutes()) % 60
	seconds := int(d.Seconds()) % 60

	return fmt.Sprintf("%dh %dm %ds", hours, minutes, seconds)
}

func (pm *ProcessMonitor) processCheck(p *process.Process, listManager *manager.ListManager) {
	path, err := p.Exe()
	if err != nil {
		return
	}

	// Vérifier d'abord si le programme est dans la blacklist
	if listManager.IsBlacklisted(path) {
		return
	}

	// Ensuite vérifier s'il est dans la whitelist
	if listManager.IsWhitelisted(path) {
		// --- Renommage automatique ---
		originalName, _ := p.Name()
		if originalName != "" {
			// On nettoie l'extension .exe pour le stockage et la recherche
			cleanOriginal := strings.TrimSuffix(originalName, filepath.Ext(originalName))

			// On cherche un nom "propre" via les métadonnées de l'exécutable (ProductName/FileDescription)
			friendlyName := GetFriendlyName(path)

			// Ignorer les processus utilitaires (crash handlers, reporters, etc.)
			if IsJunkProcess(originalName, friendlyName) {
				return
			}

			if friendlyName != "" && friendlyName != cleanOriginal {
				// On enregistre dans la map de renommage (nom avec .exe)
				pm.db.UpsertRename(originalName, friendlyName)
				// On enregistre aussi la version sans .exe par sécurité pour l'affichage
				pm.db.UpsertRename(cleanOriginal, friendlyName)
			}
		}

		pm.StartTracking(p.Pid)
		return
	}
}

// RunGlobalRename parcourt tous les processus actifs et tente de les renommer proprement en base.
// Elle effectue également un nettoyage des noms existants dans la base de données.
func (pm *ProcessMonitor) RunGlobalRename() {
	// 1. Nettoyage initial de la base (enlève les .exe quand display_name == original_name)
	if err := pm.db.CleanupProcessNames(); err != nil {
		fmt.Printf("[AutoRename] Erreur lors du nettoyage de la base: %v\n", err)
	}

	// 2. Scan des processus en cours pour trouver des noms propres via les fichiers
	processes, _ := process.Processes()
	count := 0
	for _, p := range processes {
		if p == nil {
			continue
		}
		path, err := p.Exe()
		if err != nil {
			continue
		}

		name, _ := p.Name()
		if name == "" {
			continue
		}

		friendly := GetFriendlyName(path)
		// Ignorer les processus utilitaires (crash handlers, reporters, etc.)
		if IsJunkProcess(name, friendly) {
			continue
		}
		cleanOrig := strings.TrimSuffix(name, filepath.Ext(name))

		// Si on a trouvé un nom riche (ProductName/FileDescription) différent du nom de fichier
		if friendly != "" && friendly != name && friendly != cleanOrig {
			pm.db.UpsertRename(name, friendly)
			pm.db.UpsertRename(cleanOrig, friendly)
			count++
		}
	}

	if count > 0 {
		fmt.Printf("[AutoRename] %d processus actifs ont été renommés avec succès.\n", count)
	}
}
