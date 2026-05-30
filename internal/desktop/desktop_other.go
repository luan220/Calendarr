//go:build !windows

// Non-Windows build of the desktop helpers. Calendarr's desktop integration
// (system tray, registry auto-start, MPC-BE) is Windows-only; on Linux/macOS
// the binary runs headless (server mode), so these are minimal cross-platform
// equivalents: browser-open via the OS default opener, auto-start left to the
// OS (systemd/launchd), and the "message box" downgraded to a stderr line.
package desktop

import (
	"fmt"
	"os"
	"os/exec"
	"runtime"
)

// AutoStartEnabled always reports false: on a headless host, launch-at-boot is
// handled by the init system (systemd user service / launchd), not by us.
func AutoStartEnabled(name string) bool { return false }

// SetAutoStart is a no-op on non-Windows (see AutoStartEnabled).
func SetAutoStart(name string, enabled bool) error { return nil }

// SetAutoStartPath is a no-op on non-Windows (kept for API parity).
func SetAutoStartPath(name, exePath string, enabled bool) error { return nil }

// RefreshAutoStart is a no-op on non-Windows.
func RefreshAutoStart(name string) error { return nil }

// OpenBrowser opens a URL with the OS default handler (open on macOS,
// xdg-open elsewhere). Best-effort; harmless on a headless host with no display.
func OpenBrowser(url string) {
	opener := "xdg-open"
	if runtime.GOOS == "darwin" {
		opener = "open"
	}
	_ = exec.Command(opener, url).Start()
}

// MessageBox has no GUI on a headless host — surface the text on stderr.
func MessageBox(title, text string) {
	fmt.Fprintf(os.Stderr, "%s: %s\n", title, text)
}
