//go:build windows

// Package desktop = petits utilitaires Windows pour l'intégration bureau :
// démarrage auto (clé Run du registre), ouverture du navigateur / d'un terminal,
// et boîte de message d'erreur (utile quand l'app n'a pas de console).
package desktop

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"

	"golang.org/x/sys/windows/registry"
)

const runKey = `Software\Microsoft\Windows\CurrentVersion\Run`

// AutoStartEnabled indique si l'app est lancée au démarrage de Windows.
func AutoStartEnabled(name string) bool {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.QUERY_VALUE)
	if err != nil {
		return false
	}
	defer k.Close()
	_, _, err = k.GetStringValue(name)
	return err == nil
}

// SetAutoStart active/désactive le démarrage auto pour l'exécutable courant.
func SetAutoStart(name string, enabled bool) error {
	exe := ""
	if enabled {
		e, err := os.Executable()
		if err != nil {
			return err
		}
		exe = e
	}
	return SetAutoStartPath(name, exe, enabled)
}

// SetAutoStartPath active/désactive le démarrage auto pour un exe précis. Sert à
// l'installeur, qui doit enregistrer server.exe et non lui-même.
func SetAutoStartPath(name, exePath string, enabled bool) error {
	k, err := registry.OpenKey(registry.CURRENT_USER, runKey, registry.SET_VALUE)
	if err != nil {
		return err
	}
	defer k.Close()
	if !enabled {
		if err := k.DeleteValue(name); err != nil && err != registry.ErrNotExist {
			return err
		}
		return nil
	}
	return k.SetStringValue(name, `"`+exePath+`"`)
}

// OpenBrowser ouvre une URL dans le navigateur par défaut.
func OpenBrowser(url string) {
	_ = exec.Command("rundll32", "url.dll,FileProtocolHandler", url).Start()
}

// OpenTerminal ouvre une fenêtre PowerShell qui suit le journal en direct. On
// passe par un petit script temporaire (.ps1) plutôt qu'un -Command : c'est plus
// robuste (pas de souci de guillemets) et la fenêtre affiche toujours un en-tête,
// donc elle n'est jamais vide même si le journal est momentanément absent.
func OpenTerminal(logPath string) {
	p := strings.ReplaceAll(logPath, "'", "''")
	script := "$Host.UI.RawUI.WindowTitle = 'Calendarr - journal serveur'\n" +
		"[Console]::OutputEncoding = [Text.Encoding]::UTF8\n" +
		"$log = '" + p + "'\n" +
		"Write-Host '=== Journal Calendarr (en direct, Ctrl+C pour fermer) ===' -ForegroundColor Cyan\n" +
		"Write-Host $log -ForegroundColor DarkGray\n" +
		"while (-not (Test-Path -LiteralPath $log)) { Write-Host 'En attente du journal...' -ForegroundColor Yellow; Start-Sleep -Seconds 1 }\n" +
		"Get-Content -LiteralPath $log -Tail 500 -Wait -Encoding UTF8\n"
	tmp := filepath.Join(os.TempDir(), "calendarr-log.ps1")
	if err := os.WriteFile(tmp, []byte(script), 0o644); err != nil {
		return
	}
	cmd := exec.Command("powershell", "-NoExit", "-ExecutionPolicy", "Bypass", "-File", tmp)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010} // CREATE_NEW_CONSOLE
	_ = cmd.Start()
}

// MessageBox affiche une boîte d'erreur native (l'app n'ayant pas de console).
func MessageBox(title, text string) {
	proc := syscall.NewLazyDLL("user32.dll").NewProc("MessageBoxW")
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	proc.Call(0, uintptr(unsafe.Pointer(t)), uintptr(unsafe.Pointer(c)), 0x10) // MB_ICONERROR
}
