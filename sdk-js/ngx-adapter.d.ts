/**
 * PrintDriverAdapter implementation for ngx-pos-print
 * (https://www.npmjs.com/package/ngx-pos-print).
 *
 * Bridges any ngx-pos-print-based POS app to the local Print Bridge agent.
 *
 * @example
 * ```typescript
 * import { providePosPrint } from 'ngx-pos-print';
 * import { PrintBridgeAdapter } from '@print-bridge/sdk/ngx-adapter';
 *
 * bootstrapApplication(AppComponent, {
 *   providers: [
 *     providePosPrint(
 *       { driver: 'custom', paperSize: 80 },
 *       [new PrintBridgeAdapter()]
 *     ),
 *   ],
 * });
 * ```
 */

export interface PrintBridgeAdapterOptions {
  /** Fallback-chain priority (lower = tried first). Default 0. */
  priority?: number;
  /** Pin to a specific printer ID returned by the agent. Default: agent picks. */
  printerId?: string;
  /** Override the agent base URL. When set, autodiscover() is skipped. */
  baseUrl?: string;
}

/** ngx-pos-print PrintResult (re-declared to avoid the peer-dep import). */
export interface NgxPrintResult {
  success: boolean;
  driver: 'custom';
  error?: string;
  timestamp: number;
}

/** ngx-pos-print DetectedPrinter (re-declared to avoid the peer-dep import). */
export interface NgxDetectedPrinter {
  driver: 'custom';
  name?: string;
  connected: boolean;
}

export declare class PrintBridgeAdapter {
  readonly name: string;
  readonly priority: number;

  constructor(options?: PrintBridgeAdapterOptions);

  isAvailable(): Promise<boolean>;
  isConnected(): Promise<boolean>;
  connect(): Promise<void>;
  print(data: Uint8Array): Promise<NgxPrintResult>;
  detect(): Promise<NgxDetectedPrinter[]>;
  disconnect(): Promise<void>;
}

export default PrintBridgeAdapter;
