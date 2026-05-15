# Print Bridge

> **Agent local universel d'impression thermique pour applications web.**
> Permet à n'importe quelle application web d'imprimer sur n'importe quelle imprimante thermique, **sans driver à modifier, sans dialogue Windows, sans configuration**.

[![Status](https://img.shields.io/badge/status-v1.0-blue.svg)]()
[![Windows](https://img.shields.io/badge/platform-Windows%2010%2F11-green.svg)]()
[![License](https://img.shields.io/badge/license-MIT-orange.svg)]()

## Pourquoi ?

Le problème avec WebUSB et la boîte de dialogue Windows :

- **WebUSB** ne fonctionne pas de manière fiable sur toutes les imprimantes thermiques. Il faut remplacer le driver (WinUSB) au préalable, ce qui échoue sur certains modèles. Le navigateur impose aussi un consentement utilisateur à chaque session.
- **La boîte de dialogue d'impression Windows** ouverte par le navigateur est lente, intrusive, et rend impossible un flux d'impression silencieux (caisse, ticket, étiquette).
- **L'identification déterministe** de l'imprimante connectée est impossible quand plusieurs périphériques sont sur les ports USB.

Print Bridge résout ces trois problèmes en s'intercalant entre le navigateur et l'imprimante.

## Architecture

```
┌──────────────────────────────────────────────────┐
│   Logiciel de vente (navigateur Chrome/Edge)     │
│   fetch('https://localhost:19101/print', …)      │
└──────────────────────┬───────────────────────────┘
                       │ HTTP + HTTPS + CORS *
                       ▼
┌──────────────────────────────────────────────────┐
│       Print Bridge Agent (service Windows)       │
│  - détection auto multi-canaux (poll + mDNS)     │
│  - API REST locale + double serveur HTTP/HTTPS   │
│  - cert racine privé installé dans store Windows │
│  - envoi RAW ESC/POS sans dialogue Windows       │
│  - builder ESC/POS riche (QR, barcode, image)    │
└──┬──────────┬───────────┬──────────┬────────────┘
   │          │           │          │
 winspool   WinUSB     TCP 9100    Serial / BT
   │          │           │          │
   ▼          ▼           ▼          ▼
            Imprimantes thermiques
```

## État actuel — v1.0

Toutes les phases sont livrées. L'agent supporte cinq canaux de communication en parallèle, identifie automatiquement les imprimantes thermiques, et expose une API stable côté navigateur en HTTP et HTTPS.

| Capacité | Statut |
|---|---|
| Détection des imprimantes installées dans Windows (spooler) | ✅ |
| Identification automatique des imprimantes thermiques (DB VID:PID + heuristiques) | ✅ |
| Impression RAW ESC/POS via `WritePrinter` (zéro dialogue Windows) | ✅ |
| API HTTP locale (`/printers`, `/print`, `/print/text`, `/health`) | ✅ |
| Service Windows (install / uninstall / start / stop) | ✅ |
| Builder ESC/POS (texte, alignement, gras, cut, tiroir-caisse) | ✅ |
| Builder ESC/POS riche — QR code, code-barres 1D, image bitmap | ✅ |
| Backend réseau TCP 9100 + scan auto du /24 local | ✅ |
| Découverte mDNS / Bonjour (`_pdl-datastream`, `_printer`, `_ipp`) | ✅ |
| Backend série COM + Bluetooth SPP (via port COM virtuel) | ✅ |
| Backend USB direct via Win32 WinUSB (pur Go, sans CGO) | ✅ |
| Routage multi-canal automatique + déduplication | ✅ |
| HTTPS avec certificat racine auto-généré et auto-installé Windows | ✅ |
| Appel depuis sites HTTPS sans Mixed-Content | ✅ |
| App tray Windows (status, test print, accès logs) | ✅ |
| Driver natif `'bridge'` dans [ngx-pos-print](https://www.npmjs.com/package/ngx-pos-print) v1.1.1+ | ✅ |
| Client JS standalone (non-publié) + page HTML de démo dans `sdk-js/` | ✅ |
| Installeur double-cliquable (`Install.cmd` auto-élève en admin) | ✅ |
| Script de release (ZIP autonome, ~5.9 MB) | ✅ |

## Installation

### Pré-requis

- Windows 10 ou 11 (64-bit ou ARM64)
- Au moins une imprimante thermique accessible par l'un des canaux supportés : USB (avec ou sans driver Windows), réseau Ethernet/Wi-Fi, port série, ou Bluetooth appairé

### Pour les utilisateurs finaux

**Option A — Installeur single-EXE (le plus simple)**

1. Télécharger `PrintBridge-Setup-X.Y.Z.exe` depuis les [releases](https://github.com/gmetenou7/POS-PRINTER-DRIVER-FOR-NGX-POS-PRINT-IN-WINDOWS/releases)
2. **Double-cliquer** dessus → UAC apparaît → accepter
3. Suivre la fenêtre de progression (~5 secondes)

**Option B — Archive ZIP**

1. Télécharger `print-bridge-X.Y.Z-windows-amd64.zip`
2. Extraire l'archive
3. Double-cliquer sur `Install.cmd` — il demande les droits admin automatiquement

Dans les deux cas, l'installeur :
- Copie les binaires dans `C:\Program Files\PrintBridge\`
- Enregistre le service Windows (démarrage automatique)
- Génère un certificat racine privé et l'ajoute au store Windows (pour HTTPS sans avertissement)
- Démarre le service et lance l'icône tray
- Configure le tray pour démarrer à chaque login

Pour désinstaller : double-cliquer sur `Uninstall.cmd` (présent dans `C:\Program Files\PrintBridge\` après installation, ou dans l'archive ZIP).

### Pour les développeurs

```powershell
# Compiler les deux binaires
go build -o bin\print-bridge.exe .\cmd\agent
go build -ldflags "-H=windowsgui" -o bin\print-bridge-tray.exe .\cmd\tray

# Tester sans installer (console)
.\bin\print-bridge.exe

# Produire un ZIP + setup.exe de release stripped
.\installer\release.ps1 -Version 1.0.3
```

## Utilisation depuis un navigateur

### Avec ngx-pos-print (recommandé pour Angular)

```bash
npm install ngx-pos-print  # version 1.1.1 ou plus récente
```

```ts
import { providePosPrint } from 'ngx-pos-print';

bootstrapApplication(AppComponent, {
  providers: [
    providePosPrint({ driver: 'bridge', paperSize: 80 }),
  ],
});
```

Puis dans ton composant :

```ts
posPrint.printLines([
  { type: 'text', content: 'MAGASIN', align: 'center', bold: true },
  { type: 'separator' },
  { type: 'text', content: 'Article 1 …………  5,00 €' },
  { type: 'text', content: 'Total …………… 12,50 €', bold: true },
  { type: 'cut' },
]);
```

ngx-pos-print détecte automatiquement l'agent Print Bridge et route l'impression. Si l'agent n'est pas installé, fallback automatique vers les autres drivers (USB/BT/Network/Window).

### Vanilla JavaScript (sans framework)

Le dossier [`sdk-js/`](sdk-js/) contient un client autonome non-publié pour les apps non-Angular :

```html
<script type="module">
  import { PrintBridge } from './sdk-js/index.js';

  const bridge = await PrintBridge.autodiscover();
  await bridge.printText('Bonjour !', { cut: true });
</script>
```

Ouvre [`sdk-js/example.html`](sdk-js/example.html) dans Chrome pour une démo interactive.

> **HTTPS sans avertissement** : à l'installation, `install.ps1` enregistre une autorité racine privée « Print Bridge Local CA » dans le store racine Windows. Les navigateurs font ensuite confiance à `https://localhost:19101` sans rien afficher. Si tu utilises le binaire sans installeur, exécute `print-bridge.exe -cmd trust-ca` en admin.

### Appel HTTP direct (fetch / axios / curl)

```http
POST http://127.0.0.1:19100/print
Content-Type: application/json

{
  "text": "Hello\nWorld",
  "cut": true,
  "openDrawer": false,
  "copies": 1
}
```

Réponse :
```json
{ "ok": true, "bytes": 42, "durationMs": 73 }
```

## API HTTP

L'agent écoute sur deux ports :
- **HTTP** : `http://127.0.0.1:19100` — pour les apps web servies en HTTP/localhost
- **HTTPS** : `https://localhost:19101` — pour les apps web servies en HTTPS (Mixed Content)

| Méthode | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Sonde de vie |
| `GET` | `/printers` | Liste de toutes les imprimantes détectées |
| `GET` | `/printers/{id}` | Détail d'une imprimante |
| `POST` | `/print` | Soumettre un job (corps JSON `text` ou `raw` base64) |
| `POST` | `/print/text?printerId=…` | Soumettre du texte brut (corps `text/plain`) |

## Comment Print Bridge contourne le dialogue Windows

Le dialogue Windows apparaît quand on imprime via **GDI** ou via `ShellExecute "print"`. Print Bridge n'appelle **jamais** ces APIs. À la place, il ouvre directement le spooler avec le type de données `RAW` :

```c
OpenPrinter(name, &h, NULL);
StartDocPrinter(h, 1, &(DOC_INFO_1){ .pDatatype = "RAW" });
StartPagePrinter(h);
WritePrinter(h, escposBytes, len, &written);
EndPagePrinter(h);
EndDocPrinter(h);
ClosePrinter(h);
```

Le spooler transmet les octets bruts à l'imprimante sans aucun rendu graphique ni interaction utilisateur. C'est la même technique qu'utilisent les SDK des fabricants (Epson EPS, Star CloudPRNT bridge, Bixolon Web Print SDK).

## Imprimantes thermiques reconnues automatiquement

La base interne reconnaît les VID USB et les noms de modèle des fabricants courants : **Epson, Star Micronics, Bixolon, Citizen, XPrinter, HPRT, SNBC, Custom, Zebra**, plus toute imprimante dont le nom contient `POS`, `Thermal`, `Receipt`, `Ticket`, `80mm`, `58mm`, etc.

Si ta marque n'est pas reconnue, ajoute son VID dans `internal/printers/thermal_db.go` et ouvre une PR.

## Désinstallation

```powershell
.\installer\install.ps1 -Uninstall
```

ou double-clic sur `Uninstall.cmd` depuis l'archive de release.

## Licence

MIT — Libre d'utilisation et de modification.
