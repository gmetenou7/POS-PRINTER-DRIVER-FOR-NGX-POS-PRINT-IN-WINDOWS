# POS Printer Driver Installer for Windows

> **Installe automatiquement le driver WinUSB pour les imprimantes thermiques POS, permettant l'accès WebUSB sur Windows.**

[![Version](https://img.shields.io/badge/version-4.0-blue.svg)]()
[![Windows](https://img.shields.io/badge/platform-Windows%2010%2F11-green.svg)]()
[![License](https://img.shields.io/badge/license-MIT-orange.svg)]()

## Le problème

Sur Windows, le driver système `usbprint.sys` prend le contrôle exclusif des imprimantes USB, ce qui **bloque l'accès WebUSB**. Les applications web comme celles utilisant [ngx-pos-print](https://www.npmjs.com/package/ngx-pos-print) ne peuvent pas communiquer directement avec l'imprimante.

## La solution

Cet installeur remplace automatiquement le driver Windows par **WinUSB**, permettant à WebUSB d'accéder à l'imprimante. Il utilise **libwdi** (la même technologie que [Zadig](https://zadig.akeo.ie/)), garantissant une installation fiable.

## Installation

### 1. Téléchargez l'installeur

Téléchargez `POS-Printer-Driver-Installer.exe` ou le dossier complet.

### 2. Branchez votre imprimante

Connectez votre imprimante POS en USB et allumez-la.

### 3. Lancez l'installeur

**Double-cliquez** sur `POS-Printer-Driver-Installer.exe`

> Alternative : vous pouvez aussi utiliser `Install-POS-Printer-Driver.bat`

### 4. Suivez les instructions

```
========================================
  POS Printer WinUSB Driver Installer
        for ngx-pos-print v4.0
       (Powered by libwdi/Zadig)
========================================

Running as Administrator.

Scanning USB devices...

Found 5 USB device(s):

  #  | VID:PID     | Class           | Name
  ---|-------------|-----------------|--------------------------------
  1  | 1FC9:2016   | USB             | Printer POS-80 (NEEDS WINUSB)
  2  | 046D:C52B   | HIDClass        | Logitech USB Receiver
  ...

Enter device number (or 'q' to quit): 1

Installing WinUSB driver (this may take a moment)...

========================================
       SUCCESS! WinUSB INSTALLED
========================================

Your printer is ready for WebUSB!
```

### 5. Testez

1. Débranchez et rebranchez l'imprimante
2. Ouvrez Chrome ou Edge
3. Testez votre application WebUSB

## Fonctionnalités

- ✅ **100% automatique** - Aucune configuration manuelle requise
- ✅ **Téléchargement automatique** - Récupère libwdi automatiquement
- ✅ **Compatible toutes imprimantes** - Fonctionne avec n'importe quelle imprimante USB
- ✅ **Sécurisé** - Utilise les mêmes outils que Zadig (libwdi)
- ✅ **Réversible** - Facile à annuler si besoin

## Imprimantes testées

| Marque | Modèles |
|--------|---------|
| **Epson** | TM-T20, TM-T88, TM-M30 |
| **Star Micronics** | TSP100, TSP650, mPOP |
| **Bixolon** | SRP-330, SRP-350 |
| **Citizen** | CT-S310, CT-S4000 |
| **XPrinter** | XP-58, XP-80 |
| **HPRT** | TP806, TP808 |
| **Generiques** | POS-58, POS-80, etc. |

## Désinstallation / Retour en arrière

Pour restaurer le driver Windows original :

1. Ouvrez le **Gestionnaire de périphériques** (`Win + X`)
2. Trouvez votre imprimante sous "Périphériques USB universels"
3. Clic droit → **Désinstaller l'appareil**
4. Cochez "Supprimer le pilote"
5. **Débranchez et rebranchez** l'imprimante

Windows réinstallera automatiquement le driver original.

## FAQ

### L'installeur demande des droits administrateur ?

Oui, l'installation de drivers Windows nécessite des privilèges administrateur. C'est normal et sécurisé.

### Une fenêtre de sécurité Windows apparaît ?

Cliquez sur **"Installer"**. C'est Windows qui demande confirmation pour l'installation du driver.

### L'imprimante n'apparaît pas dans la liste ?

- Vérifiez que l'imprimante est **allumée**
- Vérifiez le **câble USB**
- Essayez un **autre port USB**

### WebUSB dit toujours "Access Denied" ?

1. **Débranchez et rebranchez** l'imprimante
2. **Fermez et rouvrez** le navigateur
3. Vérifiez que vous êtes sur **HTTPS** ou **localhost**

### Ça affecte mes autres imprimantes ?

**Non.** Le driver WinUSB est installé uniquement pour le VID:PID spécifique de l'imprimante sélectionnée. Vos autres périphériques ne sont pas affectés.

## Structure des fichiers

```
pos-printer-driver-installer/
├── POS-Printer-Driver-Installer.exe  # ⭐ Double-cliquez ici !
├── Install-POS-Printer-Driver.bat    # Alternative (lance le .ps1)
├── install-driver.ps1                # Script PowerShell source
└── README.md                         # Ce fichier
```

> **Distribution** : Vous pouvez distribuer uniquement `POS-Printer-Driver-Installer.exe` (33 KB) - il contient tout le nécessaire.

## Compatibilité

| OS | Support |
|----|---------|
| Windows 11 (64-bit) | ✅ |
| Windows 10 (64-bit) | ✅ |
| Windows 10/11 ARM64 | ✅ |
| Windows 7/8 | ❌ |

## Technologies utilisées

- **[libwdi](https://github.com/pbatard/libwdi)** - Windows Driver Installer library
- **[WinUSB](https://docs.microsoft.com/en-us/windows-hardware/drivers/usbcon/winusb)** - Driver USB générique de Microsoft
- **PowerShell** - Script d'automatisation

## Lien avec ngx-pos-print

Cet outil est conçu pour fonctionner avec [ngx-pos-print](https://www.npmjs.com/package/ngx-pos-print), une bibliothèque Angular pour l'impression POS thermique via WebUSB/Bluetooth.

```bash
npm install ngx-pos-print
```

## Contributing

Contributions are welcome! See [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines.

- Report bugs or suggest features via [Issues](https://github.com/gmetenou7/POS-PRINTER-DRIVER-FOR-NGX-POS-PRINT-IN-WINDOWS/issues)
- Submit improvements via [Pull Requests](https://github.com/gmetenou7/POS-PRINTER-DRIVER-FOR-NGX-POS-PRINT-IN-WINDOWS/pulls)
- Add your tested printer models to the compatibility table

## Licence

MIT License - Libre d'utilisation et de modification.

## Crédits

- [libwdi](https://github.com/pbatard/libwdi) par Pete Batard
- [Zadig](https://zadig.akeo.ie/) pour l'inspiration
- [QMK](https://github.com/qmk/qmk_driver_installer) pour l'installeur

---

**Développé pour la communauté POS/retail par l'équipe ngx-pos-print**
