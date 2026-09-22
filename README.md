# Print Bridge

> **Agent local d'impression pour applications web.**
> Permet à n'importe quelle application web d'imprimer sur n'importe quelle imprimante, thermique
> par flux ESC/POS ou de bureau par pages rendues, **sans driver à modifier, sans dialogue
> système, sans configuration**.

[![Status](https://img.shields.io/badge/status-v1.1-blue.svg)]()
[![Windows](https://img.shields.io/badge/platform-Windows%2010%2F11-green.svg)]()
[![Linux](https://img.shields.io/badge/platform-Linux%20%7C%20macOS%20(CUPS)-green.svg)]()
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

## État actuel, v1.0

Toutes les phases sont livrées. L'agent supporte cinq canaux de communication en parallèle, identifie automatiquement les imprimantes thermiques, et expose une API stable côté navigateur en HTTP et HTTPS.

| Capacité | Statut |
|---|---|
| Détection des imprimantes installées dans Windows (spooler) | ✅ |
| Identification automatique des imprimantes thermiques (DB VID:PID + heuristiques) | ✅ |
| Impression RAW ESC/POS via `WritePrinter` (zéro dialogue Windows) | ✅ |
| API HTTP locale (`/printers`, `/print`, `/print/text`, `/health`) | ✅ |
| Documents de page A4 sans dialogue Windows (`/print-document`) | ✅ |
| Lecture des capacités du pilote (`/printers/{id}/capabilities`) | ✅ |
| Service Windows (install / uninstall / start / stop) | ✅ |
| Builder ESC/POS (texte, alignement, gras, cut, tiroir-caisse) | ✅ |
| Builder ESC/POS riche, QR code, code-barres 1D, image bitmap | ✅ |
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
| Impression de documents sous Linux et macOS via CUPS (`lp`, `lpstat`, `lpoptions`) | ✅ |
| Lecture des capacités du pilote sous CUPS (formats, bacs, recto-verso, couleur) | ✅ |
| Chaque appel au système borné dans le temps (6 s en lecture, 60 s à l'impression) | ✅ |

## Installation

### Pré-requis

- **Windows 10 ou 11** (64-bit ou ARM64), pour le service, l'installeur et l'icône de zone de
  notification. C'est le parcours complet, et celui que décrit le reste de cette page.
- **Linux ou macOS** conviennent aussi, avec CUPS, présent par défaut sur les deux. L'agent s'y
  lance à la main ou en service du système ; il n'y a ni installeur ni icône de zone de
  notification, l'API et le comportement sont les mêmes.
- Au moins une imprimante accessible par l'un des canaux supportés : USB (avec ou sans driver),
  réseau Ethernet/Wi-Fi, port série, ou Bluetooth appairé. Une imprimante de bureau installée
  dans le système convient également, pour les documents de page.

### Pour les utilisateurs finaux

**Option A, Installeur single-EXE (le plus simple)**

1. Télécharger `PrintBridge-Setup-X.Y.Z.exe` depuis les [releases](https://github.com/gmetenou7/POS-PRINTER-DRIVER-FOR-NGX-POS-PRINT-IN-WINDOWS/releases)
2. **Double-cliquer** dessus → UAC apparaît → accepter
3. Suivre la fenêtre de progression (~5 secondes)

**Option B, Archive ZIP**

1. Télécharger `print-bridge-X.Y.Z-windows-amd64.zip`
2. Extraire l'archive
3. Double-cliquer sur `Install.cmd`, il demande les droits admin automatiquement

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
- **HTTP** : `http://127.0.0.1:19100`, pour les apps web servies en HTTP/localhost
- **HTTPS** : `https://localhost:19101`, pour les apps web servies en HTTPS (Mixed Content)

| Méthode | Endpoint | Description |
|---|---|---|
| `GET` | `/health` | Sonde de vie |
| `GET` | `/printers` | Liste de toutes les imprimantes détectées |
| `GET` | `/printers/{id}` | Détail d'une imprimante |
| `GET` | `/printers/{id}/capabilities` | Ce que le pilote sait faire : formats, bacs, recto-verso, couleur, copies |
| `POST` | `/print` | Soumettre un job (corps JSON `text` ou `raw` base64) |
| `POST` | `/print/text?printerId=…` | Soumettre du texte brut (corps `text/plain`) |
| `POST` | `/print-document` | Imprimer un document de page (A4 et assimilés), pages déjà rendues |

### Documents de page : remplacer la fenêtre d'impression

Les deux routes ci-dessus existent pour une raison précise : permettre à une application web de
**se passer entièrement du dialogue d'impression du système**, y compris pour une facture A4.

`GET /printers/{id}/capabilities` interroge le pilote installé sur la machine, le spooler sous
Windows et CUPS ailleurs, et rend ce qu'il déclare savoir faire.

```json
{
  "ok": true,
  "capabilities": {
    "papers": [{ "id": 9, "name": "A4" }, { "id": 1, "name": "Letter" }],
    "bins":   [{ "id": 1, "name": "Bac 1" }, { "id": 4, "name": "Bac manuel" }],
    "duplex": true, "color": true, "maxCopies": 99,
    "dpi": 600, "widthPx": 4958, "heightPx": 7016
  }
}
```

C'est la même source que la fenêtre de réglages du pilote. Une application peut donc n'offrir
que des options réelles, au lieu d'en proposer que la machine remplacera sans rien dire. Une
imprimante sans pilote hôte, une thermique en USB brut par exemple, rend `"driverless": true`
et aucune capacité : elle n'a pas d'options à offrir, et c'est une réponse, pas une panne.

**`papers` et `bins` sont toujours des listes, jamais `null`.** Une imprimante qui n'annonce
aucun format rend `[]`. La distinction n'est pas cosmétique : un `null` traversait l'API sous
forme de liste absente, et le code appelant qui comptait ses éléments plantait au milieu de son
rendu, laissant une fenêtre de sélection vide sans le moindre message.

`POST /print-document` imprime, sans qu'aucune fenêtre ne s'ouvre.

```json
{
  "printerId": "…",
  "jobName": "FAC-2026-000123",
  "pages": ["data:image/png;base64,…", "…"],
  "options": { "copies": 2, "color": false, "duplex": "long", "bin": 1, "paper": 9 }
}
```

**Les pages arrivent déjà rendues, une image chacune.** Celui qui imprime affiche presque
toujours un aperçu avant, donc ce rendu existe déjà chez lui. Le refaire ici obligerait à
embarquer un moteur PDF dans l'agent, donc du C, donc la fin du binaire unique qui s'installe
sans rien d'autre. La contrepartie est assumée : ce qui sort est une image de la page, pas du
texte vectoriel. Sur du papier, à 200 points par pouce, la différence ne se voit pas.

Les options partent dans le `DEVMODE` du pilote, via `CreateDC` puis `StartDoc` : le chemin
qu'emprunte n'importe quelle application qui imprime, à ceci près qu'aucune fenêtre n'est
ouverte. Le pilote relit et corrige ce qui n'a pas de sens pour lui, un recto-verso sur un
modèle qui n'en a pas par exemple.

Une imprimante qui n'est pas pilotée par l'hôte (`channel` autre que `winspool`) refuse cette
route avec un 422 : elle attend son flux d'octets, pas une page rendue. C'est `/print` qui la
sert.

### Linux et macOS : le même contrat, par CUPS

Le dépôt porte « IN WINDOWS » dans son nom parce que c'est le système où l'absence de solution
faisait le plus mal. L'agent tourne aussi sous Linux et macOS, et la même application web y
retrouve la même API, sans rien changer chez elle.

Là où Windows passe par le spooler, ces systèmes passent par **CUPS**, qui est déjà installé
partout :

| Ce qu'il faut | Commande appelée |
|---|---|
| Lister les imprimantes | `lpstat -p`, puis `lpstat -v` et `lpstat -d` pour le canal et celle par défaut |
| Lire les capacités | `lpoptions -p <file> -l` |
| Imprimer une page rendue | `lp` avec `-o fit-to-page` |
| Imprimer un flux ESC/POS | `lp -o raw` |

Les options voyagent traduites, parce que CUPS raisonne en mots-clés là où Windows raisonne en
numéros : la couleur devient `print-color-mode=color` ou `monochrome`, le recto-verso devient
`sides=two-sided-long-edge` ou `short-edge`, le format devient `media=A4` et le bac
`InputSlot=<mot-clé>`.

**Ce que la machine ne sait pas faire n'est pas proposé.** Une HP DeskJet 2800 en Wi-Fi rend
vingt-deux formats, aucun recto-verso et aucune couleur, ce que confirme `ipptool` avec
`sides-supported = one-sided` et `color-supported = false`. L'application n'affiche donc ni
case couleur ni case recto-verso pour cette imprimante, au lieu d'offrir un réglage que le
pilote jetterait en silence.

**Chaque appel est borné.** Six secondes pour une lecture, soixante pour une impression, et les
options d'une file sont gardées cinq minutes. Sans ces bornes, une file déclarée dont
l'imprimante est débranchée laisse `lpoptions` attendre, et l'agent attend avec lui : côté
navigateur, cela se voit comme une recherche d'imprimantes qui tourne sans fin.

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

MIT, Libre d'utilisation et de modification.
