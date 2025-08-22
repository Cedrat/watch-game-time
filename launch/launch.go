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
	// Vous devez remplacer "icon.ico" par le chemin vers votre fichier d'icône
	icon, err := os.ReadFile("./icon.ico")
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
		fmt.Println(err)
	}
	// Extraire le nom du programme à partir du chemin

	// Vérifier d'abord si le programme est dans la blacklist
	if listManager.IsBlacklisted(path) {
		fmt.Printf("Programme blacklisté ignoré: %s\n", path)
		return
	}

	// Ensuite vérifier s'il est dans la whitelist
	if listManager.IsWhitelisted(path) {
		pm.StartTracking(p.Pid)
		return
	}
}
