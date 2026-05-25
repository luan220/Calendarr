// client : petit helper à lancer sur la machine de visionnage. Il vit dans la
// zone de notification (system tray) : clic gauche = ouvrir le calendrier, clic
// droit = menu (démarrage auto, fermer). En tâche de fond il écoute en local et,
// quand on clique "play" dans le calendrier (navigateur), lance MPC-BE sur l'URL
// du fichier servie par le serveur — le navigateur ne peut pas lancer une appli.
package main

import (
	"embed"
	"flag"
	"log"
	"net"
	"net/http"
	"os"
	"path/filepath"

	"fyne.io/systray"

	"calendarr-local/internal/desktop"
	"calendarr-local/internal/player"
)

//go:embed icon.ico
var iconFS embed.FS

func main() {
	addr := flag.String("addr", "127.0.0.1:8788", "adresse locale du helper")
	mpc := flag.String("mpc", "", "chemin de MPC-BE (vide = auto-détection)")
	flag.Parse()

	exePath, _ := os.Executable()
	logPath := filepath.Join(filepath.Dir(exePath), "client.log")
	if lf, e := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644); e == nil {
		log.SetOutput(lf)
	}

	if p := player.FindMPCBE(*mpc); p != "" {
		log.Printf("MPC-BE détecté: %s", p)
	} else {
		log.Printf("MPC-BE introuvable — précise -mpc <chemin> si besoin (le helper tourne quand même)")
	}

	ln, err := net.Listen("tcp", *addr)
	if err != nil {
		// Port déjà pris = une autre instance du helper tourne déjà.
		desktop.MessageBox("Calendarr", "client.exe est déjà lancé sur cette machine.")
		return
	}

	http.HandleFunc("/play", func(w http.ResponseWriter, r *http.Request) {
		// La page est servie depuis le LAN (serveur:8787) et appelle ce helper
		// en loopback : Chrome envoie un preflight Private Network Access qu'il
		// faut autoriser explicitement, sinon le fetch est bloqué.
		if cors(w, r) {
			return
		}
		url := r.URL.Query().Get("url")
		if url == "" {
			http.Error(w, "url requise", http.StatusBadRequest)
			return
		}
		p := player.FindMPCBE(*mpc)
		if p == "" {
			http.Error(w, "MPC-BE introuvable", http.StatusServiceUnavailable)
			return
		}
		if err := player.Play(p, url); err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		log.Printf("lecture: %s", url)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	http.HandleFunc("/ping", func(w http.ResponseWriter, r *http.Request) {
		if cors(w, r) {
			return
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	})

	go func() {
		log.Printf("client prêt : http://%s (helper de lecture MPC-BE)", *addr)
		if err := http.Serve(ln, nil); err != nil {
			log.Printf("helper arrêté : %v", err)
		}
	}()

	runTray()
}

// runTray installe l'icône dans la zone de notification. Clic gauche ouvre le
// calendrier ; clic droit ouvre le menu (démarrage auto, fermer).
func runTray() {
	const appName = "CalendarrClient"
	iconBytes, _ := iconFS.ReadFile("icon.ico")
	onReady := func() {
		systray.SetIcon(iconBytes)
		systray.SetTooltip("Calendarr — clic pour ouvrir le calendrier")
		systray.SetOnTapped(func() { go openCalendar() }) // clic gauche

		mAuto := systray.AddMenuItemCheckbox("Démarrer avec Windows", "Lancer automatiquement à l'ouverture de Windows", desktop.AutoStartEnabled(appName))
		systray.AddSeparator()
		mQuit := systray.AddMenuItem("Fermer", "Quitter Calendarr")

		go func() {
			for {
				select {
				case <-mAuto.ClickedCh:
					enable := !mAuto.Checked()
					if err := desktop.SetAutoStart(appName, enable); err != nil {
						desktop.MessageBox("Calendarr", "Démarrage auto : "+err.Error())
						continue
					}
					if enable {
						mAuto.Check()
					} else {
						mAuto.Uncheck()
					}
				case <-mQuit.ClickedCh:
					systray.Quit()
					return
				}
			}
		}()
	}
	systray.Run(onReady, func() { os.Exit(0) })
}

// cors pose les en-têtes CORS + Private Network Access. Renvoie true si la
// requête était un preflight OPTIONS (déjà répondu, l'appelant doit s'arrêter).
func cors(w http.ResponseWriter, r *http.Request) bool {
	h := w.Header()
	h.Set("Access-Control-Allow-Origin", "*")
	h.Set("Access-Control-Allow-Methods", "GET, OPTIONS")
	h.Set("Access-Control-Allow-Headers", "*")
	h.Set("Access-Control-Allow-Private-Network", "true")
	if r.Method == http.MethodOptions {
		w.WriteHeader(http.StatusNoContent)
		return true
	}
	return false
}
