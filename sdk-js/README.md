# sdk-js — client navigateur pour Print Bridge

> **Non publié sur npm.** Pour les apps Angular, utilise plutôt [ngx-pos-print](https://www.npmjs.com/package/ngx-pos-print) avec `driver: 'bridge'`.

Ce dossier contient un client JavaScript autonome pour parler à l'agent Print Bridge en HTTP/HTTPS depuis un navigateur. Il est conservé ici pour :

- **Tester l'agent** sans installer ngx-pos-print (voir `example.html`)
- **Servir de référence** pour intégrer Print Bridge dans une stack non-Angular (React, Vue, vanilla)
- **Reproduire des bugs** rapidement avec un client minimal

## Utilisation rapide

```html
<script type="module">
  import { PrintBridge } from './index.js';

  const bridge = await PrintBridge.autodiscover();
  await bridge.printText('Hello', { cut: true });
</script>
```

ou ouvrir directement `example.html` dans Chrome avec l'agent qui tourne sur la même machine.

## API

### `PrintBridge.autodiscover(options?)`
Sonde `https://localhost:19101` puis `http://127.0.0.1:19100`. Cache l'URL trouvée dans `sessionStorage`.

### `bridge.listPrinters()`
Retourne la liste des imprimantes détectées par l'agent.

### `bridge.printText(text, options?)`
Construit ESC/POS + cut + (optionnel) `openDrawer`.

### `bridge.printRaw(bytes, options?)`
Envoie des bytes ESC/POS bruts (Uint8Array ou array de nombres).

### `bridge.printQR(data, options?)`
QR code, options : `module` (1-16), `ecc` (`L`/`M`/`Q`/`H`).

### `bridge.printBarcode(data, options?)`
Code-barres 1D, options : `type` (`EAN13`/`CODE128`/...), `height`, `widthMul`, `hri`.

### `bridge.printImage(source, options?)`
PNG/JPEG en bitmap, accepte Blob, ArrayBuffer, Uint8Array ou base64.

## HTTPS et certificats

L'agent crée une autorité racine privée à l'installation et l'ajoute au store racine Windows. Les navigateurs font ensuite confiance à `https://localhost:19101` sans avertissement.

Si tu vois `ERR_CERT_AUTHORITY_INVALID`, lance en admin :
```powershell
print-bridge.exe -cmd trust-ca
```

## Pour les apps Angular

Tu n'as **pas besoin** de ce fichier — ngx-pos-print 1.1.0+ inclut déjà un driver `bridge` qui parle à l'agent :

```ts
providePosPrint({ driver: 'bridge' })
```
