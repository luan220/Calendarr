//go:build windows

// Package desktop provides small Windows utilities for desktop integration:
// auto-start (via the Task Scheduler COM API — no schtasks.exe spawn),
// opening the browser via ShellExecute, opening a terminal that tails the
// log, and an error message box (useful when the app has no console).
//
// Every Win32 call goes through the documented golang.org/x/sys/windows or
// github.com/go-ole/go-ole interfaces (static imports, COM via ole32) — no
// runtime LoadLibrary, no rundll32 launcher, no schtasks.exe spawn. The
// resulting binary's import table looks like a normal native Windows app.
package desktop

import (
	"errors"
	"os"
	"os/exec"
	"strings"
	"syscall"

	"github.com/go-ole/go-ole"
	"github.com/go-ole/go-ole/oleutil"
	"golang.org/x/sys/windows"
)

// AutoStartEnabled reports whether a scheduled task with this name exists
// for the current user.
func AutoStartEnabled(name string) bool {
	ok, _ := taskExists(name)
	return ok
}

// SetAutoStart enables or disables auto-start for the current executable.
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

// SetAutoStartPath enables or disables auto-start for a specific exe path.
// Used by the installer, which needs to register the eventual location of
// the binary rather than its own path.
func SetAutoStartPath(name, exePath string, enabled bool) error {
	if !enabled {
		return deleteTask(name)
	}
	return registerTask(name, exePath)
}

// RefreshAutoStart re-syncs the auto-start task to the current executable
// path if it is enabled. No-op if disabled. Lets the user move or rebuild
// the .exe transparently — the next launch updates the task definition so
// a stale path is healed without any manual action.
func RefreshAutoStart(name string) error {
	if !AutoStartEnabled(name) {
		return nil
	}
	return SetAutoStart(name, true)
}

// OpenBrowser opens a URL in the user's default browser via ShellExecute
// (the same API the Start menu uses to launch shortcuts).
func OpenBrowser(url string) {
	verb, _ := syscall.UTF16PtrFromString("open")
	file, _ := syscall.UTF16PtrFromString(url)
	_ = windows.ShellExecute(0, verb, file, nil, nil, windows.SW_SHOWNORMAL)
}

// OpenTerminal opens a PowerShell window that tails the log live. The script
// is passed inline via -Command (no .ps1 on disk, no ExecutionPolicy flag).
func OpenTerminal(logPath string) {
	p := strings.ReplaceAll(logPath, "'", "''")
	script := "$Host.UI.RawUI.WindowTitle = 'Calendarr - server log';" +
		"[Console]::OutputEncoding = [Text.Encoding]::UTF8;" +
		"$log = '" + p + "';" +
		"Write-Host '=== Calendarr log (live, Ctrl+C to close) ===' -ForegroundColor Cyan;" +
		"Write-Host $log -ForegroundColor DarkGray;" +
		"while (-not (Test-Path -LiteralPath $log)) { Write-Host 'Waiting for the log...' -ForegroundColor Yellow; Start-Sleep -Seconds 1 };" +
		"Get-Content -LiteralPath $log -Tail 500 -Wait -Encoding UTF8"
	cmd := exec.Command("powershell.exe", "-NoExit", "-NoLogo", "-NoProfile", "-Command", script)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: 0x00000010} // CREATE_NEW_CONSOLE
	_ = cmd.Start()
}

// MessageBox displays a native error dialog. user32!MessageBoxW is imported
// statically through golang.org/x/sys/windows — no runtime LoadLibrary.
func MessageBox(title, text string) {
	t, _ := syscall.UTF16PtrFromString(text)
	c, _ := syscall.UTF16PtrFromString(title)
	_, _ = windows.MessageBox(0, t, c, 0x10) // MB_ICONERROR
}

// ----------------------------------------------------------------------
// Task Scheduler via COM (Schedule.Service). Functionally identical to
// "schtasks /create /sc onlogon /rl limited /f" but without spawning the
// schtasks.exe LOLBin.

// Task Scheduler enum values (see ITaskService / IRegisteredTask docs).
const (
	taskActionExec            = 0 // TASK_ACTION_EXEC
	taskTriggerLogon          = 9 // TASK_TRIGGER_LOGON
	taskCreateOrUpdate        = 6 // TASK_CREATE_OR_UPDATE
	taskLogonInteractiveToken = 3 // TASK_LOGON_INTERACTIVE_TOKEN
)

// withTaskService runs fn against a connected Schedule.Service instance,
// handling COM init/uninit and IDispatch lifetimes.
func withTaskService(fn func(svc *ole.IDispatch) error) error {
	// CoInitializeEx may fail with RPC_E_CHANGED_MODE / S_FALSE if the
	// thread is already initialized — both are non-fatal for our purposes.
	_ = ole.CoInitializeEx(0, ole.COINIT_MULTITHREADED)
	defer ole.CoUninitialize()

	unknown, err := oleutil.CreateObject("Schedule.Service")
	if err != nil {
		return err
	}
	defer unknown.Release()

	svc, err := unknown.QueryInterface(ole.IID_IDispatch)
	if err != nil {
		return err
	}
	defer svc.Release()

	if _, err := oleutil.CallMethod(svc, "Connect"); err != nil {
		return err
	}
	return fn(svc)
}

// taskExists reports whether a task with the given name is registered in
// the root folder of the user's Task Scheduler store.
func taskExists(name string) (bool, error) {
	var found bool
	err := withTaskService(func(svc *ole.IDispatch) error {
		folderV, e := oleutil.CallMethod(svc, "GetFolder", `\`)
		if e != nil {
			return e
		}
		folder := folderV.ToIDispatch()
		defer folder.Release()

		taskV, e := oleutil.CallMethod(folder, "GetTask", name)
		if e != nil {
			return nil // GetTask returns an error if the task does not exist
		}
		if taskV != nil {
			if td := taskV.ToIDispatch(); td != nil {
				td.Release()
			}
		}
		found = true
		return nil
	})
	return found, err
}

// registerTask creates (or replaces) a logon-triggered scheduled task that
// runs exePath as the current user with standard privileges.
func registerTask(name, exePath string) error {
	return withTaskService(func(svc *ole.IDispatch) error {
		folderV, err := oleutil.CallMethod(svc, "GetFolder", `\`)
		if err != nil {
			return err
		}
		folder := folderV.ToIDispatch()
		defer folder.Release()

		defV, err := oleutil.CallMethod(svc, "NewTask", uint32(0))
		if err != nil {
			return err
		}
		def := defV.ToIDispatch()
		defer def.Release()

		// Description (purely cosmetic, visible in Task Scheduler UI).
		if regV, e := oleutil.GetProperty(def, "RegistrationInfo"); e == nil {
			reg := regV.ToIDispatch()
			_, _ = oleutil.PutProperty(reg, "Description", "Calendarr launches at user logon.")
			reg.Release()
		}

		// Settings: keep running on battery, allow on-demand start.
		if setV, e := oleutil.GetProperty(def, "Settings"); e == nil {
			set := setV.ToIDispatch()
			_, _ = oleutil.PutProperty(set, "DisallowStartIfOnBatteries", false)
			_, _ = oleutil.PutProperty(set, "StopIfGoingOnBatteries", false)
			_, _ = oleutil.PutProperty(set, "AllowDemandStart", true)
			set.Release()
		}

		// Logon trigger.
		trigsV, err := oleutil.GetProperty(def, "Triggers")
		if err != nil {
			return err
		}
		trigs := trigsV.ToIDispatch()
		trigV, err := oleutil.CallMethod(trigs, "Create", int32(taskTriggerLogon))
		trigs.Release()
		if err != nil {
			return err
		}
		trigV.ToIDispatch().Release()

		// Exec action.
		actsV, err := oleutil.GetProperty(def, "Actions")
		if err != nil {
			return err
		}
		acts := actsV.ToIDispatch()
		actV, err := oleutil.CallMethod(acts, "Create", int32(taskActionExec))
		acts.Release()
		if err != nil {
			return err
		}
		act := actV.ToIDispatch()
		_, _ = oleutil.PutProperty(act, "Path", exePath)
		act.Release()

		// Register (create or update).
		_, err = oleutil.CallMethod(folder, "RegisterTaskDefinition",
			name,
			def,
			int32(taskCreateOrUpdate),
			nil, // user (nil = current)
			nil, // password
			int32(taskLogonInteractiveToken),
			"", // sddl
		)
		return err
	})
}

// deleteTask removes a scheduled task by name. A missing task is not an
// error (the function is idempotent — call it freely when disabling).
func deleteTask(name string) error {
	return withTaskService(func(svc *ole.IDispatch) error {
		folderV, err := oleutil.CallMethod(svc, "GetFolder", `\`)
		if err != nil {
			return err
		}
		folder := folderV.ToIDispatch()
		defer folder.Release()

		_, err = oleutil.CallMethod(folder, "DeleteTask", name, int32(0))
		if err != nil {
			// "task not found" is fine when toggling off.
			var oleErr *ole.OleError
			if errors.As(err, &oleErr) {
				return nil
			}
			return nil // treat any delete error as non-fatal (idempotent)
		}
		return nil
	})
}
