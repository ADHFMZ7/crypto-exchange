import { formatMinorUnits, toAmount } from "./decimal";
import type { ReferenceData } from "./reference";

export type { Currency, Market, ReferenceData } from "./reference";

// Side is the wire vocabulary, so it is defined once in types and re-exported
// here for the callers that reason about markets rather than payloads.
export type { Side } from "../types";

/**
 * The decimal places a currency holds — minor units per whole unit is 10^this.
 *
 * The backend is the only source — lib/reference deliberately keeps no fallback
 * table, because two copies of a currency's precision silently disagreeing is a
 * factor-of-10^n error in every amount the app shows and sends.
 *
 * An unknown currency falls back to 0, which treats a value as already being in
 * minor units rather than inventing a scale factor for something we have no
 * exponent for.
 */
export function effectiveExponent(ref: ReferenceData, code: string): number {
  return ref.currencies[code]?.exponent ?? 0;
}

/**
 * The everyday name for a currency's smallest unit, for a hint under an input.
 */
export function minorUnitName(code: string, exponent: number): string {
  if (exponent === 0) return `whole ${code}`;
  if (code === "USD") return "cents";
  if (code === "BTC") return "satoshis";
  return `${code} minor units`;
}

/** Formats an amount already in minor units. */
export function formatAmount(ref: ReferenceData, minor: bigint, code: string): string {
  return `${formatMinorUnits(minor, effectiveExponent(ref, code))} ${code}`;
}

/** Formats a raw balance from GET /wallets/me. */
export function formatBalance(
  ref: ReferenceData,
  available: string | number,
  code: string
): string {
  return formatAmount(ref, toAmount(available), code);
}

/**
 * Renders a submitted order's two amounts back into money.
 *
 * An order records what actually went on the wire — `shares` in base minor
 * units, `price` in quote minor units per whole base — so reading it back needs
 * the same two exponents that produced it, and they come from different
 * currencies. Getting that pairing backwards is silent: both are plausible
 * integers, and the only symptom is a price off by a factor of 10^6.
 *
 * A market with no reference entry has no exponent to apply, so the raw integer
 * is shown rather than a guess at one.
 */
export function formatOrderLegs(
  ref: ReferenceData,
  order: { market: string; quantity: number; filled_quantity?: number; price_each: number }
): { quantity: string; filled: string; price: string } {
  const market = ref.markets.find((m) => m.symbol === order.market);

  const render = (value: number | undefined, code: string | undefined): string => {
    if (value === undefined) return "—";
    // Defensive against a malformed payload: a non-integer must not throw
    // inside BigInt() and take the page down with it.
    if (!Number.isInteger(value)) return String(value);
    if (!code) return value.toLocaleString();
    return formatAmount(ref, BigInt(value), code);
  };

  return {
    quantity: render(order.quantity, market?.base),
    filled: render(order.filled_quantity, market?.base),
    price: render(order.price_each, market?.quote)
  };
}

/**
 * What fraction of an order has filled, 0..1.
 *
 * Settlement advances `filled_quantity` in the same transaction that records
 * the trade, so this tracks real executions rather than an estimate. It moves
 * asynchronously — an order can fill seconds after the 202 that accepted it.
 */
export function fillFraction(order: { quantity: number; filled_quantity: number }): number {
  if (!order.quantity) return 0;
  return Math.min(Math.max(order.filled_quantity / order.quantity, 0), 1);
}

/**
 * Formats a price for a market — quote minor units per one WHOLE base unit, so
 * the QUOTE exponent applies.
 *
 * Pairing a price with the base exponent is the silent failure this exists to
 * prevent: both readings produce a plausible integer, and the only symptom is a
 * number wrong by a factor of 10^(base - quote).
 */
export function formatPrice(ref: ReferenceData, symbol: string, priceMinor: number): string {
  const market = ref.markets.find((m) => m.symbol === symbol);
  if (!market || !Number.isInteger(priceMinor)) return String(priceMinor);
  return formatAmount(ref, BigInt(priceMinor), market.quote);
}

/** Formats a quantity for a market — base minor units, so the BASE exponent applies. */
export function formatQuantity(ref: ReferenceData, symbol: string, quantityMinor: number): string {
  const market = ref.markets.find((m) => m.symbol === symbol);
  if (!market || !Number.isInteger(quantityMinor)) return String(quantityMinor);
  return formatAmount(ref, BigInt(quantityMinor), market.base);
}

/**
 * The ticker's change as a percentage of its opening price.
 *
 * The server sends the change in absolute minor units and the open beside it,
 * deliberately: a percentage is a ratio of two integers, and forming it here
 * keeps the rounding at the point of display instead of baking a precision into
 * the wire format.
 *
 * Undefined when there is no open to divide by — a market whose first trade is
 * inside the window has no prior price, and "infinite gain" is not a number to
 * render.
 */
export function percentChange(ticker: { change: number; open_price: number }): number | undefined {
  if (!ticker.open_price) return undefined;
  return (ticker.change / ticker.open_price) * 100;
}
