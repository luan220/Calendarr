// Package discovery : découverte automatique du serveur sur le LAN. Le serveur
// diffuse en continu un petit "phare" UDP en broadcast ; le client l'écoute pour
// trouver l'adresse du calendrier sans aucune configuration (ni nom, ni IP à
// taper). Objectif : l'hôte ouvre server.exe, un proche ouvre
// client.exe sur un autre PC → le navigateur s'ouvre tout seul.
package discovery

import (
	"net"
	"strconv"
	"strings"
	"time"
)

// Port UDP du phare et préfixe magique (pour ne pas confondre avec du trafic tiers).
const (
	Port  = 8786
	Magic = "CALENDARR-LOCAL"
)

// Message construit le contenu du phare : "CALENDARR-LOCAL|<port http>|<nom pc>".
func Message(httpPort, host string) string {
	return Magic + "|" + httpPort + "|" + host
}

// Listen écoute les phares pendant au plus timeout et renvoie l'URL HTTP du
// premier serveur entendu (http://<ip source>:<port annoncé>). On utilise l'IP
// source du paquet (toujours résoluble) plutôt que le nom : rien à configurer.
func Listen(timeout time.Duration) (string, bool) {
	pc, err := net.ListenUDP("udp4", &net.UDPAddr{Port: Port})
	if err != nil {
		return "", false
	}
	defer pc.Close()
	_ = pc.SetReadDeadline(time.Now().Add(timeout))

	buf := make([]byte, 256)
	for {
		n, src, err := pc.ReadFromUDP(buf)
		if err != nil {
			return "", false // délai dépassé : aucun serveur trouvé
		}
		parts := strings.Split(string(buf[:n]), "|")
		if len(parts) < 2 || parts[0] != Magic {
			continue
		}
		if _, e := strconv.Atoi(parts[1]); e != nil {
			continue
		}
		return "http://" + src.IP.String() + ":" + parts[1], true
	}
}

// BroadcastAddrs renvoie l'adresse de broadcast de chaque interface réseau
// active (ex: 192.168.1.255, 172.23.63.255…). On émet le phare vers TOUTES,
// sinon il ne sortirait que par l'interface par défaut (souvent un adaptateur
// virtuel WSL/Hyper-V) et n'atteindrait jamais le vrai LAN.
func BroadcastAddrs() []net.IP {
	var out []net.IP
	ifaces, err := net.Interfaces()
	if err != nil {
		return out
	}
	for _, ifi := range ifaces {
		if ifi.Flags&net.FlagUp == 0 || ifi.Flags&net.FlagLoopback != 0 || ifi.Flags&net.FlagBroadcast == 0 {
			continue
		}
		addrs, _ := ifi.Addrs()
		for _, a := range addrs {
			ipnet, ok := a.(*net.IPNet)
			if !ok {
				continue
			}
			ip4 := ipnet.IP.To4()
			if ip4 == nil {
				continue
			}
			mask := ipnet.Mask
			if len(mask) == 16 {
				mask = mask[12:] // masque IPv4 parfois stocké sur 16 octets
			}
			if len(mask) != 4 {
				continue
			}
			bc := make(net.IP, 4)
			for i := 0; i < 4; i++ {
				bc[i] = ip4[i] | ^mask[i]
			}
			out = append(out, bc)
		}
	}
	return out
}
