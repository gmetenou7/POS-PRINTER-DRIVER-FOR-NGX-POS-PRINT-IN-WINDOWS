# Legacy — WinUSB Driver Installer

> Cette approche est **dépréciée**. Utilise plutôt [Print Bridge](../README.md), l'agent local universel qui fonctionne sur toutes les imprimantes thermiques sans modifier le driver Windows.

## Pourquoi déprécié ?

L'installation d'un driver WinUSB pour activer WebUSB ne fonctionnait pas de manière fiable sur tous les modèles d'imprimantes. De plus, WebUSB ne permet pas une identification déterministe de l'imprimante quand plusieurs périphériques USB sont connectés, et le navigateur impose un consentement utilisateur à chaque session.

Le nouveau Print Bridge contourne complètement ces problèmes :
- Pas de driver à remplacer
- Pas de dialogue Windows
- API locale universelle (HTTP/HTTPS/WebSocket)
- Détection automatique multi-canaux (USB driver, USB direct, réseau, série, Bluetooth)

## Quand utiliser quand même cet installeur ?

Uniquement si tu as besoin d'une intégration **WebUSB pure** sans installer d'agent local. Dans ce cas, l'installeur ci-dessous reste fonctionnel.

## Contenu

- `Install-POS-Printer-Driver.bat` — Lance le script PowerShell avec élévation
- `install-driver.ps1` — Script PowerShell qui télécharge libwdi et installe WinUSB
- `POS-Printer-Driver-Installer.exe` — Version compilée du `.bat` (pour distribution)

Voir l'historique git pour la documentation complète de cette version.
