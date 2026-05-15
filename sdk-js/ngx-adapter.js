// Print Bridge — ngx-pos-print PrintDriverAdapter implementation.
//
// Lets a POS app written against ngx-pos-print (https://www.npmjs.com/package/ngx-pos-print)
// route its prints through the local Print Bridge agent without changing
// the app's code at all. Register it via `providePosPrint(config, [new PrintBridgeAdapter()])`
// and set `driver: 'custom'` to use it as the active driver.
//
// All Print Bridge advantages flow through transparently:
//   - multi-channel printer detection (winspool, libusb, network, serial)
//   - HTTPS support (no Mixed-Content from HTTPS-hosted POS apps)
//   - no Windows print dialog
//   - works on any thermal printer the agent can reach

import { PrintBridge } from './index.js';

export class PrintBridgeAdapter {
  /**
   * @param {object} [options]
   * @param {number} [options.priority]
   * @param {string} [options.printerId]    Specific printer ID to target (else default).
   * @param {string} [options.baseUrl]      Override agent base URL (skips auto-discovery).
   */
  constructor(options = {}) {
    this.name = 'print-bridge';
    this.priority = options.priority ?? 0;
    this._printerId = options.printerId;
    this._baseUrl = options.baseUrl;
    /** @type {PrintBridge | null} */
    this._bridge = null;
  }

  async isAvailable() {
    try {
      await this._ensureBridge();
      return true;
    } catch (_) {
      return false;
    }
  }

  async isConnected() {
    try {
      const bridge = await this._ensureBridge();
      const printers = await bridge.listPrinters();
      if (this._printerId) {
        return printers.some(p => p.id === this._printerId && p.status === 'ready');
      }
      return printers.some(p => p.isThermal && p.status === 'ready');
    } catch (_) {
      return false;
    }
  }

  async connect() {
    await this._ensureBridge();
  }

  async print(data) {
    const t0 = Date.now();
    try {
      const bridge = await this._ensureBridge();
      await bridge.printRaw(data, { printerId: this._printerId });
      return {
        success: true,
        driver: 'custom',
        timestamp: t0,
      };
    } catch (err) {
      return {
        success: false,
        driver: 'custom',
        error: String(err?.message ?? err),
        timestamp: t0,
      };
    }
  }

  async detect() {
    try {
      const bridge = await this._ensureBridge();
      const printers = await bridge.listPrinters();
      return printers
        .filter(p => p.isThermal)
        .map(p => ({
          driver: 'custom',
          name: `${p.name} [${p.channel}]${p.isDefault ? ' ★' : ''}`,
          connected: p.status === 'ready',
        }));
    } catch (_) {
      return [];
    }
  }

  async disconnect() {
    this._bridge = null;
  }

  // --- internals -----------------------------------------------------------

  async _ensureBridge() {
    if (this._bridge) return this._bridge;
    this._bridge = this._baseUrl
      ? new PrintBridge(this._baseUrl)
      : await PrintBridge.autodiscover();
    return this._bridge;
  }
}

export default PrintBridgeAdapter;
