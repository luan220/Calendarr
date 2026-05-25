package main

import (
	"log"
	"net/http"
	"os/exec"
	"time"

	"calendarr-local/internal/discovery"
)

// openCalendar trouve le serveur tout seul et ouvre le calendrier dans le
// navigateur. Best-effort, à lancer dans une goroutine au démarrage du helper.
//  1. serveur sur CETTE machine (install mono) → localhost
//  2. sinon, écoute le phare LAN d'un serveur ailleurs (install multi-PC)
func openCalendar() {
	if pingServer("http://127.0.0.1:8787/api/calendar") {
		openBrowser("http://localhost:8787")
		return
	}
	if url, ok := discovery.Listen(25 * time.Second); ok {
		openBrowser(url)
		return
	}
	log.Printf("aucun serveur détecté sur le réseau (lance server.exe sur le PC qui a Sonarr)")
}

func pingServer(url string) bool {
	c := &http.Client{Timeout: 1500 * time.Millisecond}
	resp, err := c.Get(url)
	if err != nil {
		return false
	}
	_ = resp.Body.Close()
	return resp.StatusCode == http.StatusOK
}

func openBrowser(url string) {
	log.Printf("ouverture du calendrier dans le navigateur : %s", url)
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}
