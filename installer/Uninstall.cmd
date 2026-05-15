@echo off
REM Print Bridge — désinstalleur double-cliquable

setlocal

net session >nul 2>&1
if %errorlevel% neq 0 (
    echo Demande des droits administrateur...
    powershell -Command "Start-Process -FilePath '%~f0' -Verb RunAs"
    exit /b
)

echo.
echo ===========================================
echo   Print Bridge — Désinstallation
echo ===========================================
echo.

powershell -ExecutionPolicy Bypass -NoProfile -File "%~dp0install.ps1" -Uninstall

echo.
pause
