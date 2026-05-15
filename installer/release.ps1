# Print Bridge — script de release.
# Compile les deux binaires, vérifie la version, et zippe le tout pour distribution.
#
# Usage : .\installer\release.ps1 -Version 0.4.0

[CmdletBinding()]
param(
    [Parameter(Mandatory)][string]$Version
)

$ErrorActionPreference = "Stop"

$Root      = Split-Path $PSScriptRoot -Parent
$BinDir    = Join-Path $Root "bin"
$DistDir   = Join-Path $Root "dist"
$Stage     = Join-Path $DistDir "print-bridge-$Version"
$ZipPath   = Join-Path $DistDir "print-bridge-$Version-windows-amd64.zip"

if (Test-Path $Stage)   { Remove-Item -Recurse -Force $Stage }
if (Test-Path $ZipPath) { Remove-Item -Force $ZipPath }
New-Item -ItemType Directory -Path $Stage -Force | Out-Null

Write-Host "Compilation de l'agent (print-bridge.exe)..." -ForegroundColor Cyan
& go build -trimpath -ldflags "-s -w" -o (Join-Path $BinDir "print-bridge.exe") .\cmd\agent
if ($LASTEXITCODE -ne 0) { throw "Build agent échoué" }

Write-Host "Compilation du tray (print-bridge-tray.exe)..." -ForegroundColor Cyan
& go build -trimpath -ldflags "-s -w -H=windowsgui" -o (Join-Path $BinDir "print-bridge-tray.exe") .\cmd\tray
if ($LASTEXITCODE -ne 0) { throw "Build tray échoué" }

Write-Host "Assemblage du package..." -ForegroundColor Cyan
Copy-Item (Join-Path $BinDir "print-bridge.exe")      (Join-Path $Stage "print-bridge.exe")
Copy-Item (Join-Path $BinDir "print-bridge-tray.exe") (Join-Path $Stage "print-bridge-tray.exe")
Copy-Item (Join-Path $Root "installer\install.ps1")   (Join-Path $Stage "install.ps1")
Copy-Item (Join-Path $Root "installer\Install.cmd")   (Join-Path $Stage "Install.cmd")
Copy-Item (Join-Path $Root "installer\Uninstall.cmd") (Join-Path $Stage "Uninstall.cmd")
Copy-Item (Join-Path $Root "README.md")               (Join-Path $Stage "README.md")
Copy-Item (Join-Path $Root "LICENSE")                 (Join-Path $Stage "LICENSE")

# Patch install.ps1 staged copy so it looks for binaries next to itself (no ..\bin)
$staged = Join-Path $Stage "install.ps1"
$content = Get-Content $staged -Raw
$content = $content -replace '\$PSScriptRoot \\\.\.\\bin\\', '$PSScriptRoot\'
Set-Content -Path $staged -Value $content -Encoding UTF8

Write-Host "Création de l'archive ZIP..." -ForegroundColor Cyan
Compress-Archive -Path "$Stage\*" -DestinationPath $ZipPath -Force

$size = [math]::Round((Get-Item $ZipPath).Length / 1MB, 2)
Write-Host ""
Write-Host "=========================================" -ForegroundColor Green
Write-Host "  Release prête : $ZipPath" -ForegroundColor Green
Write-Host "  Taille : $size MB" -ForegroundColor Green
Write-Host "=========================================" -ForegroundColor Green
