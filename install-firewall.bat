@echo off
REM ============================================================
REM  Calendarr - regles pare-feu (a faire UNE fois par machine)
REM  Double-clique ce fichier, clique "Oui" au prompt admin.
REM  Apres ca : plus aucun prompt pare-feu pour calendarr.
REM  C'est exactement ce que fera l'installeur final.
REM ============================================================

REM --- auto-elevation admin (1 seul UAC) ---
net session >nul 2>&1
if %errorlevel% neq 0 (
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

set "DIR=%~dp0"

REM Regle serveur (calendrier HTTP + decouverte). profile=any : marche meme si
REM Windows a classe le reseau maison en "Public" (frequent). Acces LAN seulement
REM (le routeur/NAT bloque deja l'exterieur).
netsh advfirewall firewall delete rule name="Calendarr Local" >nul 2>&1
netsh advfirewall firewall add rule name="Calendarr Local" dir=in action=allow program="%DIR%server.exe" enable=yes profile=any

REM Regle client (reception du phare de decouverte, UDP 8786).
netsh advfirewall firewall delete rule name="Calendarr Client" >nul 2>&1
netsh advfirewall firewall add rule name="Calendarr Client" dir=in action=allow program="%DIR%client.exe" enable=yes profile=any

echo.
echo  Regles pare-feu Calendarr ajoutees. Plus aucun prompt pour calendarr.
echo.
pause
