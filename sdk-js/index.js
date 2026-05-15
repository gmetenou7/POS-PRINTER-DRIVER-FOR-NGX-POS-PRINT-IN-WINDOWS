// Print Bridge — minimal browser client.
// Works against the local agent (default http://127.0.0.1:19100).
// No build step required — drop this file into your web app and import it.

const DEFAULT_BASE = 'https://localhost:19101';
const PROBE_TARGETS = [
  // HTTPS first — required when the calling page is itself HTTPS.
  { scheme: 'https', host: 'localhost',   ports: [19101, 19103, 19105] },
  { scheme: 'http',  host: '127.0.0.1',   ports: [19100, 19102, 19104] },
];

const CACHE_KEY = 'printBridge.base';

export class PrintBridge {
  constructor(base = DEFAULT_BASE) {
    this.base = base.replace(/\/+$/, '');
  }

  /**
   * Find the agent. Probes HTTPS targets first (so HTTPS-hosted apps work
   * without Mixed-Content errors), then HTTP. Caches the winning URL in
   * sessionStorage to avoid re-probing on every page.
   */
  static async autodiscover({ timeoutMs = 600, useCache = true } = {}) {
    if (useCache && typeof sessionStorage !== 'undefined') {
      const cached = sessionStorage.getItem(CACHE_KEY);
      if (cached) {
        const ok = await PrintBridge._ping(cached, timeoutMs);
        if (ok) return new PrintBridge(cached);
        sessionStorage.removeItem(CACHE_KEY);
      }
    }
    for (const tgt of PROBE_TARGETS) {
      for (const port of tgt.ports) {
        const base = `${tgt.scheme}://${tgt.host}:${port}`;
        if (await PrintBridge._ping(base, timeoutMs)) {
          if (typeof sessionStorage !== 'undefined') {
            try { sessionStorage.setItem(CACHE_KEY, base); } catch (_) {}
          }
          return new PrintBridge(base);
        }
      }
    }
    throw new Error('Agent Print Bridge introuvable. Vérifie qu\'il est installé et démarré.');
  }

  static async _ping(base, timeoutMs) {
    try {
      const r = await fetch(`${base}/health`, { signal: AbortSignal.timeout(timeoutMs) });
      return r.ok;
    } catch (_) {
      return false;
    }
  }

  async health() {
    const r = await fetch(`${this.base}/health`);
    if (!r.ok) throw new Error(`health ${r.status}`);
    return r.json();
  }

  /** Returns the array of detected printers. */
  async listPrinters() {
    const r = await fetch(`${this.base}/printers`);
    if (!r.ok) throw new Error(`printers ${r.status}`);
    const body = await r.json();
    return body.printers || [];
  }

  /** Returns the best default printer (prefers thermal + system default). */
  async pickDefault() {
    const list = await this.listPrinters();
    return (
      list.find(p => p.isDefault && p.isThermal) ||
      list.find(p => p.isThermal) ||
      list.find(p => p.isDefault) ||
      list[0] ||
      null
    );
  }

  /** Quick way to print plain text. The agent wraps it in ESC/POS and cuts. */
  async printText(text, { printerId, copies = 1, openDrawer = false, cut = true } = {}) {
    return this._post('/print', {
      printerId,
      text,
      copies,
      openDrawer,
      cut,
    });
  }

  /** Send raw ESC/POS bytes (Uint8Array or array of numbers). */
  async printRaw(bytes, { printerId, copies = 1 } = {}) {
    const u8 = bytes instanceof Uint8Array ? bytes : Uint8Array.from(bytes);
    const b64 = btoa(String.fromCharCode(...u8));
    return this._post('/print', { printerId, raw: b64, copies });
  }

  /** Print a QR code. `data` is any string (URL, text, etc.). */
  async printQR(data, { printerId, module = 6, ecc = 'M', cut = true } = {}) {
    return this._post('/print', { printerId, qr: { data, module, ecc }, cut });
  }

  /**
   * Print a 1-D barcode.
   * type: 'EAN13' | 'EAN8' | 'UPCA' | 'UPCE' | 'CODE39' | 'CODE128' | 'ITF' | 'CODE93' | 'CODABAR'
   */
  async printBarcode(data, { printerId, type = 'CODE128', height = 100, widthMul = 3, hri = 'below', cut = true } = {}) {
    return this._post('/print', {
      printerId,
      barcode: { data, type, height, widthMul, hri },
      cut,
    });
  }

  /**
   * Print a bitmap image (PNG or JPEG). `source` accepts a Blob, an ArrayBuffer,
   * a Uint8Array, or a base64 string.
   */
  async printImage(source, { printerId, maxWidthDots = 576, cut = true } = {}) {
    let b64;
    if (typeof source === 'string') {
      b64 = source.replace(/^data:[^;]+;base64,/, '');
    } else {
      const buf = source instanceof Blob ? new Uint8Array(await source.arrayBuffer())
              : source instanceof ArrayBuffer ? new Uint8Array(source)
              : source instanceof Uint8Array ? source
              : null;
      if (!buf) throw new Error('source doit être Blob, ArrayBuffer, Uint8Array ou base64');
      b64 = btoa(String.fromCharCode(...buf));
    }
    return this._post('/print', {
      printerId,
      image: { base64: b64, maxWidthDots },
      cut,
    });
  }

  async _post(path, body) {
    const r = await fetch(`${this.base}${path}`, {
      method: 'POST',
      headers: { 'Content-Type': 'application/json' },
      body: JSON.stringify(body),
    });
    const data = await r.json().catch(() => ({}));
    if (!r.ok || data.ok === false) {
      throw new Error(data.error || `print ${r.status}`);
    }
    return data;
  }
}

export default PrintBridge;
