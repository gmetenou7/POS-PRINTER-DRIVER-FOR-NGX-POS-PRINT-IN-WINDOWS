# @print-bridge/sdk

> Client navigateur pour [Print Bridge](https://github.com/gmetenou7/POS-PRINTER-DRIVER-FOR-NGX-POS-PRINT-IN-WINDOWS) — impression thermique universelle depuis une app web, sans dialogue Windows, sans WebUSB.

## Installation

```bash
npm install @print-bridge/sdk
```

Et bien sûr, l'agent Print Bridge doit tourner sur le poste de l'utilisateur — voir le [README principal](https://github.com/gmetenou7/POS-PRINTER-DRIVER-FOR-NGX-POS-PRINT-IN-WINDOWS).

## Utilisation

### Auto-découverte (recommandé)

```ts
import { PrintBridge } from '@print-bridge/sdk';

const bridge = await PrintBridge.autodiscover();

// Liste de toutes les imprimantes détectées
const printers = await bridge.listPrinters();
console.log(printers);

// Impression rapide sur l'imprimante thermique par défaut
await bridge.printText('Bonjour le monde !\nLigne 2', { cut: true });

// Ouvrir le tiroir-caisse en même temps
await bridge.printText('Total : 12,50 €', { openDrawer: true });

// Cibler une imprimante précise
await bridge.printText('Cuisine', { printerId: 'winspool-abcd1234' });

// Envoyer du ESC/POS brut (pour intégrations existantes)
await bridge.printRaw(new Uint8Array([0x1B, 0x40, 0x48, 0x69, 0x0A]));
```

### Connexion explicite

```ts
const bridge = new PrintBridge('https://localhost:19101');
await bridge.health();
```

## API

### `PrintBridge.autodiscover(options?)`

Cherche l'agent sur HTTPS:19101 d'abord (pour pages HTTPS), puis HTTP:19100. Cache l'URL trouvée dans `sessionStorage` pour les appels suivants.

### `bridge.listPrinters()`

Retourne un tableau d'objets `Printer` :

```ts
{
  id: 'winspool-1c346a7a',
  name: 'POS-80C',
  channel: 'winspool' | 'network' | 'libusb' | 'serial' | 'bluetooth',
  port: 'USB001',
  isThermal: true,
  isDefault: true,
  status: 'ready' | 'offline' | 'printing' | 'error' | 'paused',
}
```

### `bridge.printText(text, options?)`

L'agent construit le flux ESC/POS et l'envoie au bon backend (spooler RAW, TCP 9100, etc.). Options :

| Champ | Type | Défaut | Description |
|---|---|---|---|
| `printerId` | `string` | défaut système | Imprimante cible |
| `copies` | `number` | `1` | Nombre d'exemplaires (max 10) |
| `cut` | `boolean` | `true` | Coupe en fin de ticket |
| `openDrawer` | `boolean` | `false` | Ouvre le tiroir-caisse (pin 2) |

### `bridge.printRaw(bytes, options?)`

Envoie des bytes ESC/POS bruts. Utile si tu utilises déjà une lib de construction (par exemple [escpos-buffer](https://www.npmjs.com/package/escpos-buffer)) ou si tu génères des images bitmap.

## HTTPS et certificats

Quand l'agent est installé via `install.ps1`, il crée une autorité racine privée « Print Bridge Local CA » et l'ajoute au store racine Windows. Les navigateurs font alors confiance à `https://localhost:19101` sans avertissement.

Si tu vois une erreur `ERR_CERT_AUTHORITY_INVALID` en console, lance en admin :
```powershell
print-bridge.exe -cmd trust-ca
```

## Licence

MIT
