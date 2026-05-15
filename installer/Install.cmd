@echo off
REM Print Bridge — installeur double-cliquable
REM Auto-élève en administrateur puis lance install.ps1

setlocal

REM Vérifier les droits admin
net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Demande des droits administrateur...
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

REM Une fois élevé, lancer le script PowerShell
echo.
echo ===========================================
echo   Print Bridge — Installation
echo ===========================================
echo.

powershell -ExecutionPolicy Bypass -NoProfile -File "%~dp0install.ps1"

echo.
pause
