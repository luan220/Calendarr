// Package player lance le lecteur vidéo local (MPC-BE) sur un fichier ou une URL.
package player

import (
	"errors"
	"os"
	"os/exec"
)

// ErrNotFound est renvoyé quand aucun exécutable MPC-BE n'a pu être localisé.
var ErrNotFound = errors.New("MPC-BE introuvable")

// candidates liste les emplacements d'install classiques de MPC-BE (64 puis 32 bits).
var candidates = []string{
	`C:\Program Files\MPC-BE\mpc-be64.exe`,
	`C:\Program Files\MPC-BE x64\mpc-be64.exe`,
	`C:\Program Files (x86)\MPC-BE\mpc-be.exe`,
	`C:\Program Files (x86)\MPC-BE x86\mpc-be.exe`,
}

// FindMPCBE renvoie le chemin de l'exécutable MPC-BE. Si override pointe vers un
// fichier existant, il est prioritaire ; sinon on teste les emplacements connus.
func FindMPCBE(override string) string {
	if override != "" {
		if exists(override) {
			return override
		}
	}
	for _, p := range candidates {
		if exists(p) {
			return p
		}
	}
	return ""
}

// Play lance MPC-BE sur target (un chemin local ou une URL http) sans bloquer.
func Play(mpcPath, target string) error {
	if mpcPath == "" {
		return ErrNotFound
	}
	return exec.Command(mpcPath, target).Start()
}

func exists(p string) bool {
	info, err := os.Stat(p)
	return err == nil && !info.IsDir()
}
