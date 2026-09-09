/**
 * ─────────────────────────────────────────────────────────────────────────────
 * EVERY piece of fabricated data in this app lives in this file.
 *
 * If a number appears in the UI and is not sourced from `lib/api`, it comes from
 * here and is labelled "Dummy data" on screen. Each export names the backend work
 * that retires it — when that endpoint lands, delete the export and the compiler
 * will point at every call site.
 * ─────────────────────────────────────────────────────────────────────────────
 */

/** Retired by: GET /markets/{symbol}/candles or a trades websocket. */
export const MOCK_CHART_SYMBOLS = ["BTC-USD", "ETH-USD", "SOL-USD"] as const;

export type PricePoint = { t: number; price: number };

/** Seeds a flat-ish price history so the chart has something to draw. */
export function seedSeries(): Record<string, PricePoint[]> {
  const now = Date.now();
  const start = (price: number) =>
    Array.from({ length: 12 }, (_, i) => ({
      t: now - (11 - i) * 60_000,
      price: Number((price * (1 + (Math.random() - 0.5) * 0.01)).toFixed(2))
    }));

  return {
    "BTC-USD": start(45000),
    "ETH-USD": start(3200),
    "SOL-USD": start(110)
  };
}

export function driftSeries(series: Record<string, PricePoint[]>): Record<string, PricePoint[]> {
  const next: Record<string, PricePoint[]> = {};
  Object.entries(series).forEach(([symbol, points]) => {
    const last = points[points.length - 1];
    const drift = (Math.random() - 0.5) * (last.price * 0.002);
    const price = Math.max(last.price + drift, 0.0001);
    next[symbol] = [...points.slice(-30), { t: Date.now(), price: Number(price.toFixed(2)) }];
  });
  return next;
}

export const MOCK_TICK_MS = 2500;
