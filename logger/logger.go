package logger

import (
	"fmt"
	"log"
	"os"
	"path/filepath"
	"sync"
)

var (
	mu      sync.Mutex
	logFile *os.File
)

func Init() {
	exePath, err := os.Executable()
	var path string
	if err == nil {
		path = filepath.Join(filepath.Dir(exePath), "app.log")
	} else {
		path = "app.log"
	}

	f, err := os.OpenFile(path, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		fmt.Printf("Impossible d'ouvrir le fichier de log: %v\n", err)
		return
	}
	logFile = f
	log.SetOutput(f)
}

func Info(msg string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	log.Printf("[INFO] "+msg, args...)
}

func Error(msg string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	log.Printf("[ERROR] "+msg, args...)
}

func Warn(msg string, args ...any) {
	mu.Lock()
	defer mu.Unlock()
	log.Printf("[WARN] "+msg, args...)
}

func GetLogs() (string, error) {
	mu.Lock()
	defer mu.Unlock()

	exePath, err := os.Executable()
	var path string
	if err == nil {
		path = filepath.Join(filepath.Dir(exePath), "app.log")
	} else {
		path = "app.log"
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return "", err
	}
	return string(content), nil
}

func Close() {
	if logFile != nil {
		logFile.Close()
	}
}
