/**
 * Print Bridge — browser SDK.
 *
 * Talks to the local Print Bridge agent (HTTPS:19101 preferred, HTTP:19100 fallback)
 * to print on any thermal receipt printer the agent has detected.
 *
 * @example
 * ```ts
 * import { PrintBridge } from '@print-bridge/sdk';
 *
 * const bridge = await PrintBridge.autodiscover();
 * await bridge.printText('Hello\nWorld', { cut: true });
 * ```
 */

export type PrinterChannel =
  | 'winspool'
  | 'libusb'
  | 'network'
  | 'serial'
  | 'bluetooth';

export type PrinterStatus =
  | 'ready'
  | 'printing'
  | 'offline'
  | 'error'
  | 'paused'
  | 'unknown';

export interface Printer {
  id: string;
  name: string;
  channel: PrinterChannel;
  port?: string;
  driver?: string;
  vendor?: string;
  model?: string;
  vid?: string;
  pid?: string;
  isThermal: boolean;
  isDefault: boolean;
  status: PrinterStatus;
  detectedAt: string;
}

export interface PrintResult {
  ok: boolean;
  jobId?: string;
  bytes?: number;
  durationMs?: number;
  error?: string;
}

export interface PrintTextOptions {
  printerId?: string;
  copies?: number;
  cut?: boolean;
  openDrawer?: boolean;
}

export interface PrintRawOptions {
  printerId?: string;
  copies?: number;
}

export type QRECC = 'L' | 'M' | 'Q' | 'H';

export interface PrintQROptions {
  printerId?: string;
  /** Dot size, 1..16. Default 6. */
  module?: number;
  /** Error correction level. Default 'M'. */
  ecc?: QRECC;
  cut?: boolean;
}

export type BarcodeType =
  | 'EAN13'
  | 'EAN8'
  | 'UPCA'
  | 'UPCE'
  | 'CODE39'
  | 'CODE128'
  | 'ITF'
  | 'CODE93'
  | 'CODABAR';

export type BarcodeHRI = 'none' | 'above' | 'below' | 'both';

export interface PrintBarcodeOptions {
  printerId?: string;
  type?: BarcodeType;
  /** Bar height in dots. Default 100. */
  height?: number;
  /** Module width multiplier 2..6. Default 3. */
  widthMul?: number;
  /** Where to render the human-readable text. Default 'below'. */
  hri?: BarcodeHRI;
  cut?: boolean;
}

export interface PrintImageOptions {
  printerId?: string;
  /** Max width in printer dots. 384 = 58mm, 576 = 80mm. Default 576. */
  maxWidthDots?: number;
  cut?: boolean;
}

export interface AutodiscoverOptions {
  /** Max delay per probe attempt, in ms. Default 600. */
  timeoutMs?: number;
  /** Reuse the last-known base URL from sessionStorage. Default true. */
  useCache?: boolean;
}

export declare class PrintBridge {
  readonly base: string;

  constructor(base?: string);

  /**
   * Probes the local agent on HTTPS first, then HTTP, across the
   * conventional port range (19100–19105). Caches the winning URL
   * in sessionStorage for subsequent calls.
   */
  static autodiscover(options?: AutodiscoverOptions): Promise<PrintBridge>;

  /** GET /health */
  health(): Promise<{ ok: boolean; ts: number }>;

  /** GET /printers */
  listPrinters(): Promise<Printer[]>;

  /** Returns the best default (prefers thermal + system-default). */
  pickDefault(): Promise<Printer | null>;

  /** Print plain text. The agent wraps it in ESC/POS and cuts by default. */
  printText(text: string, options?: PrintTextOptions): Promise<PrintResult>;

  /** Send raw ESC/POS bytes (Uint8Array or number[]). */
  printRaw(
    bytes: Uint8Array | number[],
    options?: PrintRawOptions,
  ): Promise<PrintResult>;

  /** Print a QR code. */
  printQR(data: string, options?: PrintQROptions): Promise<PrintResult>;

  /** Print a 1-D barcode. */
  printBarcode(
    data: string,
    options?: PrintBarcodeOptions,
  ): Promise<PrintResult>;

  /** Print a PNG or JPEG image. */
  printImage(
    source: Blob | ArrayBuffer | Uint8Array | string,
    options?: PrintImageOptions,
  ): Promise<PrintResult>;
}

export default PrintBridge;
