# build.ps1 — build the release binary: bin\calendarr.exe
#
# Produces a self-contained executable:
#   - CGO + external linker (mingw-w64), with the mingw runtime linked
#     statically => no external DLL dependency beyond the system Universal CRT
#     (present on every Windows 10/11).
#   - GOAMD64=v3 => optimized for x86-64-v3 CPUs (Intel Haswell 2013+ /
#     AMD Excavator 2015+). Older CPUs get a clear runtime message and exit,
#     rather than crashing.
#   - -trimpath -buildvcs=true => reproducible, no local paths, embeds the
#     Git revision.
#   - -H=windowsgui => runs silently in the system tray (no console window).
#
# Build dependency (one-time install):
#   winget install BrechtSanders.WinLibs.POSIX.UCRT

$ErrorActionPreference = "Stop"

# Locate the mingw-w64 gcc installed via winget (path contains a version hash,
# so resolve it dynamically rather than hard-coding it).
$gcc = Get-ChildItem "$env:LOCALAPPDATA\Microsoft\WinGet\Packages\BrechtSanders.WinLibs*\mingw64\bin\gcc.exe" -ErrorAction SilentlyContinue | Select-Object -First 1
if (-not $gcc) {
    Write-Error "mingw-w64 gcc not found. Install it with: winget install BrechtSanders.WinLibs.POSIX.UCRT"
    exit 1
}

$mingwBin = Split-Path $gcc.FullName
$env:PATH = "$mingwBin;$env:PATH"
$env:CC = $gcc.FullName
$env:CGO_ENABLED = "1"
$env:GOAMD64 = "v3"

Write-Host "Building bin\calendarr.exe (CGO, static, x86-64-v3)..." -ForegroundColor Cyan
go build -trimpath -buildvcs=true `
    -ldflags "-H=windowsgui -s -w -linkmode=external -extldflags=-static" `
    -o bin\calendarr.exe .\cmd\server

if ($LASTEXITCODE -eq 0) {
    $exe = Get-Item bin\calendarr.exe
    Write-Host ("OK: bin\calendarr.exe ({0:N2} MB)" -f ($exe.Length / 1MB)) -ForegroundColor Green
} else {
    Write-Error "build failed"
    exit 1
}
