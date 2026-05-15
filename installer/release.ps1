# Print Bridge — script de release.
# Compile les binaires, assemble le payload et produit deux livrables :
#   1. dist\print-bridge-X.Y.Z-windows-amd64.zip  (archive classique)
#   2. dist\PrintBridge-Setup-X.Y.Z.exe           (installeur single-EXE)
#
# Usage : .\installer\release.ps1 -Version 1.0.0

[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Version
)

$ErrorActionPreference = "Stop"

$Root        = Split-Path $PSScriptRoot -Parent
$BinDir      = Join-Path $Root "bin"
$DistDir     = Join-Path $Root "dist"
$Stage       = Join-Path $DistDir "print-bridge-$Version"
$ZipPath     = Join-Path $DistDir "print-bridge-$Version-windows-amd64.zip"
$SetupPath   = Join-Path $DistDir "PrintBridge-Setup-$Version.exe"
$PayloadDir  = Join-Path $Root "cmd\setup\payload"

# Clean previous build
foreach ($p in @($Stage, $ZipPath, $SetupPath)) {
    if (Test-Path $p) { Remove-Item -Recurse -Force $p }
}
New-Item -ItemType Directory -Path $Stage -Force | Out-Null

# --- 1) Compile the agent + tray ----------------------------------------

Write-Host "Compilation de l'agent (print-bridge.exe)..." -ForegroundColor Cyan
& go build -trimpath -ldflags "-s -w" -o (Join-Path $BinDir "print-bridge.exe") .\cmd\agent
if ($LASTEXITCODE -ne 0) { throw "Build agent échoué" }

Write-Host "Compilation du tray (print-bridge-tray.exe)..." -ForegroundColor Cyan
& go build -trimpath -ldflags "-s -w -H=windowsgui" -o (Join-Path $BinDir "print-bridge-tray.exe") .\cmd\tray
if ($LASTEXITCODE -ne 0) { throw "Build tray échoué" }

# --- 2) Build the ZIP package -------------------------------------------

Write-Host "Assemblage du ZIP..." -ForegroundColor Cyan
Copy-Item (Join-Path $BinDir "print-bridge.exe")      (Join-Path $Stage "print-bridge.exe")
Copy-Item (Join-Path $BinDir "print-bridge-tray.exe") (Join-Path $Stage "print-bridge-tray.exe")
Copy-Item (Join-Path $Root "installer\install.ps1")   (Join-Path $Stage "install.ps1")
Copy-Item (Join-Path $Root "installer\Install.cmd")   (Join-Path $Stage "Install.cmd")
Copy-Item (Join-Path $Root "installer\Uninstall.cmd") (Join-Path $Stage "Uninstall.cmd")
Copy-Item (Join-Path $Root "README.md")               (Join-Path $Stage "README.md")
Copy-Item (Join-Path $Root "LICENSE")                 (Join-Path $Stage "LICENSE")

# install.ps1 expects either a sibling bin/ folder or sibling EXEs. The
# zip staging puts EXEs alongside it, so the second branch already works
# without patching.

Compress-Archive -Path "$Stage\*" -DestinationPath $ZipPath -Force

# --- 3) Stage the payload for the single-EXE bootstrapper ---------------

Write-Host "Préparation du payload single-EXE..." -ForegroundColor Cyan
# Clear everything in payload except .gitkeep
Get-ChildItem $PayloadDir -Exclude ".gitkeep" -Force | Remove-Item -Recurse -Force
Copy-Item (Join-Path $BinDir "print-bridge.exe")      (Join-Path $PayloadDir "print-bridge.exe")
Copy-Item (Join-Path $BinDir "print-bridge-tray.exe") (Join-Path $PayloadDir "print-bridge-tray.exe")
Copy-Item (Join-Path $Root "installer\install.ps1")   (Join-Path $PayloadDir "install.ps1")
Copy-Item (Join-Path $Root "installer\Uninstall.cmd") (Join-Path $PayloadDir "Uninstall.cmd")
Copy-Item (Join-Path $Root "LICENSE")                 (Join-Path $PayloadDir "LICENSE")

# --- 4) Build the single-EXE installer ----------------------------------

Write-Host "Compilation de l'installeur single-EXE (PrintBridge-Setup-$Version.exe)..." -ForegroundColor Cyan
& go build -trimpath -ldflags "-s -w" -o $SetupPath .\cmd\setup
if ($LASTEXITCODE -ne 0) { throw "Build setup échoué" }

# Clean payload after embedding (the data is now inside the EXE)
Get-ChildItem $PayloadDir -Exclude ".gitkeep" -Force | Remove-Item -Recurse -Force

# Clean staging directory (already zipped)
if (Test-Path $Stage) { Remove-Item -Recurse -Force $Stage }

# --- 5) Summary ---------------------------------------------------------

$zipSize   = [math]::Round((Get-Item $ZipPath).Length / 1MB, 2)
$setupSize = [math]::Round((Get-Item $SetupPath).Length / 1MB, 2)

Write-Host ""
Write-Host "=========================================" -ForegroundColor Green
Write-Host "  Release v$Version prête." -ForegroundColor Green
Write-Host ""
Write-Host "  ZIP   : $ZipPath" -ForegroundColor Green
Write-Host "          $zipSize MB" -ForegroundColor Green
Write-Host ""
Write-Host "  Setup : $SetupPath" -ForegroundColor Green
Write-Host "          $setupSize MB  (double-clic pour installer)" -ForegroundColor Green
Write-Host "=========================================" -ForegroundColor Green
