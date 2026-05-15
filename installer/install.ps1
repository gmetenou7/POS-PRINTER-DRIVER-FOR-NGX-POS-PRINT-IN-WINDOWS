# Print Bridge — installeur Windows (Phase 1)
# - copie le binaire dans %ProgramFiles%\PrintBridge
# - enregistre le service Windows (démarrage automatique)
# - démarre le service
#
# Usage : clic droit > Exécuter avec PowerShell (en admin)
#         ou      : powershell -ExecutionPolicy Bypass -File install.ps1

[CmdletBinding()]
param(
    [switch]$Uninstall
)

function Assert-Admin {
    $id  = [Security.Principal.WindowsIdentity]::GetCurrent()
    $pri = New-Object Security.Principal.WindowsPrincipal($id)
    if (-not $pri.IsInRole([Security.Principal.WindowsBuiltInRole]::Administrator)) {
        Write-Host "Cet installeur doit être lancé en administrateur." -ForegroundColor Red
        exit 1
    }
}

$ServiceName = "PrintBridge"
$InstallDir  = Join-Path $env:ProgramFiles "PrintBridge"
$Exe         = Join-Path $InstallDir "print-bridge.exe"
$TrayExe     = Join-Path $InstallDir "print-bridge-tray.exe"
$SourceExe   = if (Test-Path (Join-Path $PSScriptRoot "..\bin\print-bridge.exe")) { Join-Path $PSScriptRoot "..\bin\print-bridge.exe" } else { Join-Path $PSScriptRoot "print-bridge.exe" }
$SourceTray  = if (Test-Path (Join-Path $PSScriptRoot "..\bin\print-bridge-tray.exe")) { Join-Path $PSScriptRoot "..\bin\print-bridge-tray.exe" } else { Join-Path $PSScriptRoot "print-bridge-tray.exe" }

$StartupDir  = Join-Path ([Environment]::GetFolderPath("CommonStartup")) ""
$ShortcutLnk = Join-Path $StartupDir "PrintBridgeTray.lnk"

Assert-Admin

if ($Uninstall) {
    Write-Host "Désinstallation de Print Bridge..." -ForegroundColor Cyan

    # Tuer la tray si elle tourne
    Get-Process print-bridge-tray -ErrorAction SilentlyContinue | Stop-Process -Force -ErrorAction SilentlyContinue

    # Supprimer le raccourci de démarrage
    if (Test-Path $ShortcutLnk) { Remove-Item -Force $ShortcutLnk }

    if (Get-Service $ServiceName -ErrorAction SilentlyContinue) {
        if ((Get-Service $ServiceName).Status -eq 'Running') {
            Stop-Service $ServiceName -Force
        }
        if (Test-Path $Exe) { & $Exe -cmd untrust-ca 2>$null | Out-Null }
        & $Exe -cmd uninstall 2>$null
        sc.exe delete $ServiceName | Out-Null
    }
    if (Test-Path $InstallDir) {
        Remove-Item -Recurse -Force $InstallDir
    }
    Remove-Item -Recurse -Force "$env:ProgramData\PrintBridge" -ErrorAction SilentlyContinue
    Write-Host "Désinstallation terminée." -ForegroundColor Green
    exit 0
}

if (-not (Test-Path $SourceExe)) {
    Write-Host "Binaire introuvable : $SourceExe" -ForegroundColor Red
    Write-Host "Compile d'abord avec :  go build -o bin\print-bridge.exe .\cmd\agent" -ForegroundColor Yellow
    exit 1
}

Write-Host "Installation de Print Bridge dans $InstallDir..." -ForegroundColor Cyan
New-Item -ItemType Directory -Path $InstallDir -Force | Out-Null
Copy-Item $SourceExe $Exe -Force
if (Test-Path $SourceTray) {
    Copy-Item $SourceTray $TrayExe -Force
}

Write-Host "Enregistrement du service Windows..." -ForegroundColor Cyan
if (Get-Service $ServiceName -ErrorAction SilentlyContinue) {
    Stop-Service $ServiceName -Force -ErrorAction SilentlyContinue
    & $Exe -cmd uninstall 2>$null | Out-Null
}
& $Exe -cmd install
if ($LASTEXITCODE -ne 0) {
    Write-Host "Échec d'enregistrement du service." -ForegroundColor Red
    exit 1
}

Write-Host "Génération et installation du certificat racine..." -ForegroundColor Cyan
& $Exe -cmd trust-ca
if ($LASTEXITCODE -ne 0) {
    Write-Host "Avertissement : impossible d'installer le CA dans le store racine." -ForegroundColor Yellow
    Write-Host "  → HTTPS sera disponible mais les navigateurs afficheront un avertissement." -ForegroundColor Yellow
} else {
    Write-Host "  → CA installée. Les navigateurs ne montreront pas d'avertissement TLS." -ForegroundColor Green
}

Write-Host ""
Write-Host "Démarrage du service..." -ForegroundColor Cyan
Start-Service $ServiceName

if (Test-Path $TrayExe) {
    Write-Host "Configuration du tray au démarrage..." -ForegroundColor Cyan
    $shell = New-Object -ComObject WScript.Shell
    $sc = $shell.CreateShortcut($ShortcutLnk)
    $sc.TargetPath = $TrayExe
    $sc.WorkingDirectory = $InstallDir
    $sc.IconLocation = $TrayExe
    $sc.Description = "Print Bridge — icône de notification"
    $sc.Save()

    # Démarrer la tray maintenant pour l'utilisateur courant
    Start-Process -FilePath $TrayExe -WindowStyle Hidden
}

$svc = Get-Service $ServiceName
Write-Host ""
Write-Host "==========================================" -ForegroundColor Green
Write-Host "  Print Bridge installé et démarré." -ForegroundColor Green
Write-Host "  Service   : $($svc.Status)" -ForegroundColor Green
Write-Host "  API HTTP  : http://127.0.0.1:19100/health" -ForegroundColor Green
Write-Host "  API HTTPS : https://localhost:19101/health" -ForegroundColor Green
if (Test-Path $TrayExe) {
    Write-Host "  Tray      : démarré (icône dans la zone de notification)" -ForegroundColor Green
}
Write-Host "==========================================" -ForegroundColor Green
