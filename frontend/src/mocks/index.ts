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

import type { MarketTicker } from "../types";

/**
 * Retired by: GET /markets/{symbol}/ticker (does not exist yet).
 *
 * NOT retired by GET /markets, which is live — that lists which pairs exist but
 * carries no prices. The symbols below are invented alongside the numbers; the
 * real market list comes from lib/reference.
 */
export const MOCK_MARKETS: MarketTicker[] = [
  { symbol: "BTC-USD", price: 45210, change: 2.1, volume: 2150 },
  { symbol: "ETH-USD", price: 3230, change: -0.8, volume: 8891 },
  { symbol: "SOL-USD", price: 112, change: 1.6, volume: 12540 },
  { symbol: "DOGE-USD", price: 0.088, change: 5.2, volume: 102001 }
];

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

/** Random walk used to fake ticker movement. Pure noise — not a price feed. */
export function driftMarkets(markets: MarketTicker[]): MarketTicker[] {
  return markets.map((m) => {
    const drift = (Math.random() - 0.5) * (m.price * 0.0015);
    const nextPrice = Math.max(m.price + drift, 0.0001);
    const nextChange = ((nextPrice - m.price) / m.price) * 100 + m.change;
    return { ...m, price: Number(nextPrice.toFixed(2)), change: Number(nextChange.toFixed(2)) };
  });
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
