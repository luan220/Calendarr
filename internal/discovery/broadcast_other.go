//go:build !windows

package discovery

// Broadcast : la découverte auto n'est pas gérée hors Windows (l'appli cible
// Windows). Stub pour que la compilation passe sur les autres OS.
func Broadcast(httpPort, host string) error { return nil }
