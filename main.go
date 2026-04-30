package main

import (
	"log"
	"net"
	"os"

	_ "modernc.org/sqlite"

	"main/launch"
)

func main() {
	// Single instance check using a local TCP socket
	l, err := net.Listen("tcp", "127.0.0.1:45678")
	if err != nil {
		log.Println("Une instance de SteamTracker est déjà en cours d'exécution.")
		os.Exit(0)
	}
	defer l.Close()

	launch.StartProgramme()
}
