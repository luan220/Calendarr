//go:build windows

package discovery

import (
	"net"
	"syscall"
	"time"
)

// Broadcast diffuse le phare en continu (toutes les 2 s) vers TOUTES les
// interfaces réseau. Bloquant : à lancer dans une goroutine. Nécessite
// SO_BROADCAST, sinon Windows refuse l'envoi vers une adresse de broadcast.
func Broadcast(httpPort, host string) error {
	conn, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.IPv4zero, Port: 0})
	if err != nil {
		return err
	}
	defer conn.Close()

	if rc, e := conn.SyscallConn(); e == nil {
		_ = rc.Control(func(fd uintptr) {
			_ = syscall.SetsockoptInt(syscall.Handle(fd), syscall.SOL_SOCKET, syscall.SO_BROADCAST, 1)
		})
	}

	msg := []byte(Message(httpPort, host))
	for {
		// Une émission par interface (192.168.x.255, etc.) + le broadcast global
		// en filet de sécurité. Si aucune interface trouvée, au moins le global.
		for _, bc := range BroadcastAddrs() {
			_, _ = conn.WriteToUDP(msg, &net.UDPAddr{IP: bc, Port: Port})
		}
		_, _ = conn.WriteToUDP(msg, &net.UDPAddr{IP: net.IPv4bcast, Port: Port})
		time.Sleep(2 * time.Second)
	}
}
